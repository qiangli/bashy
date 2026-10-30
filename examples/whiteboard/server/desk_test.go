package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http/httptest"
	"strings"
	"testing"

	"desk-kit/contracttest"
)

func deskServer(t *testing.T) (*httptest.Server, *app) {
	t.Helper()
	a := fresh(t)
	a.mu.Lock()
	_, err := a.createLocked("main")
	a.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	return srv, a
}

// The shared desk contract runs against the real board server. The test
// server binds loopback; the shipped default stays LAN (see
// TestDefaultBindStaysLAN). StateHasContent: /api/state is the page's full
// board state, so it is exempt from the contentless-state probe.
func TestDeskContract(t *testing.T) {
	srv, _ := deskServer(t)
	contracttest.Run(t, contracttest.Config{BaseURL: srv.URL, Tool: "whiteboard", CLI: deskCLICallback(srv.URL), ItemParam: "board", StateHasContent: true})
}

func TestDefaultBindStaysLAN(t *testing.T) {
	host, _, err := net.SplitHostPort(defaultAddr)
	if err != nil || host != "0.0.0.0" {
		t.Fatalf("whiteboard keeps its LAN default bind (recorded exception); got %q", defaultAddr)
	}
}

func TestDeskPageCarriesKit(t *testing.T) {
	for _, m := range []string{"data-desk-copy", "desk-badge", "desk-item-header", "desk-field", "Copied", "prefers-color-scheme"} {
		if !bytes.Contains(page, []byte(m)) {
			t.Errorf("page lacks kit marker %q", m)
		}
	}
}

func TestDeskDoorMapsOntoBoards(t *testing.T) {
	_, a := deskServer(t)
	w := request(a, token(), "", "/api/action", map[string]any{"action": "add", "id": "desk1", "body": "hello-note"})
	good(t, w)
	var created map[string]any
	json.Unmarshal(w.Body.Bytes(), &created)
	key, _ := created["ownerKey"].(string)
	if !validToken(key) || created["facilitatorKey"] != key || created["id"] != "desk1" || created["status"] != "pending" {
		t.Fatalf("new: %v", created)
	}
	if strings.Contains(w.Body.String(), "hello-note") {
		t.Fatal("new re-emitted content")
	}
	// The existing board API sees the same board and the note.
	st := request(a, token(), key, "/api/state?board=desk1", nil)
	good(t, st)
	if !strings.Contains(st.Body.String(), "hello-note") {
		t.Fatal("note from new is not on the board")
	}
	// A stranger cannot open or get a private board, nor see it in list.
	for _, act := range []string{"open", "show"} {
		if c := request(a, token(), "", "/api/action", map[string]any{"action": act, "id": "desk1"}).Code; c != 403 {
			t.Errorf("%s by stranger: want 403, got %d", act, c)
		}
	}
	l := request(a, token(), "", "/api/action", map[string]any{"action": "list"})
	good(t, l)
	if strings.Contains(l.Body.String(), "desk1") {
		t.Errorf("list leaked a private board to a stranger: %s", l.Body.String())
	}
	// Only the facilitator can share/close; share on main is refused.
	if c := request(a, token(), "", "/api/action", map[string]any{"action": "close", "id": "desk1"}).Code; c != 403 {
		t.Errorf("close by stranger: want 403, got %d", c)
	}
	if c := request(a, token(), "", "/api/action", map[string]any{"action": "share", "id": "main"}).Code; c != 403 {
		t.Errorf("share main without key: want 403, got %d", c)
	}
	// close == lock; the page's lock state shows through.
	good(t, request(a, token(), key, "/api/action", map[string]any{"action": "close", "id": "desk1"}))
	if !strings.Contains(request(a, token(), key, "/api/state?board=desk1", nil).Body.String(), `"locked":true`) {
		t.Error("close did not lock the board")
	}
	if c := request(a, token(), key, "/api/action", map[string]any{"action": "set", "id": "desk1", "body": "x"}).Code; c != 409 {
		t.Errorf("put on closed board: want 409, got %d", c)
	}
}

func TestDeskTrailingDataRejected(t *testing.T) {
	srv, _ := deskServer(t)
	for _, body := range []string{`{"action":"list"}{"action":"list"}`, `{"action":"list"}{}`} {
		resp, err := srv.Client().Post(srv.URL+"/api/action", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("%s: want 400, got %d", body, resp.StatusCode)
		}
	}
}

// CLI door == HTTP door: open prints id/URL/status only.
func TestCLIOpenHidesContent(t *testing.T) {
	srv, _ := deskServer(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	var out bytes.Buffer
	if c := runCLI([]string{"-addr", addr, "--json", "add", "--id", "cli1", "--body", "cli-secret-note"}, &out); c != 0 {
		t.Fatalf("new: %d %s", c, out.String())
	}
	var created map[string]any
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("new --json: %s", out.String())
	}
	key := created["ownerKey"].(string)
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "open", "cli1", "--key", key}, &out); c != 0 {
		t.Fatalf("open: %d %s", c, out.String())
	}
	got := strings.Fields(out.String())
	if len(got) != 3 || got[0] != "cli1" || got[2] != "pending" || !strings.Contains(got[1], "board=cli1") {
		t.Errorf("open must print exactly id url status, got %q", out.String())
	}
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "show", "cli1", "--key", key}, &out); c != 0 || !strings.Contains(out.String(), "cli-secret-note") {
		t.Errorf("get must return content: %d %s", c, out.String())
	}
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "list", "--key", key}, &out); c != 0 || !strings.Contains(out.String(), "cli1 pending") || strings.Contains(out.String(), "cli-secret-note") {
		t.Errorf("list: %d %s", c, out.String())
	}
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "bogus"}, &out); c != 2 {
		t.Errorf("unknown verb: want 2, got %d", c)
	}
}

func deskCLICallback(base string) func(...string) (int, string) {
	return func(args ...string) (int, string) {
		var out bytes.Buffer
		argv := append([]string{"--addr", strings.TrimPrefix(base, "http://")}, args...)
		code := runCLI(argv, &out)
		return code, out.String()
	}
}
