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
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	return srv, a
}

// The shared desk contract runs against the real session server. The test
// server binds loopback; the shipped default stays LAN (see
// TestDefaultBindStaysLAN). StateHasContent: /api/state is the page's full
// session state, so it is exempt from the contentless-state probe.
func TestDeskContract(t *testing.T) {
	srv, _ := deskServer(t)
	contracttest.Run(t, contracttest.Config{BaseURL: srv.URL, Tool: "brainstorm", ItemParam: "session", StateHasContent: true})
}

func TestDefaultBindStaysLAN(t *testing.T) {
	host, _, err := net.SplitHostPort(defaultAddr)
	if err != nil || host != "0.0.0.0" {
		t.Fatalf("brainstorm keeps its LAN default bind (recorded exception); got %q", defaultAddr)
	}
}

func TestDeskPageCarriesKit(t *testing.T) {
	for _, m := range []string{"data-desk-copy", "desk-badge", "desk-item-header", "Copied", "prefers-color-scheme"} {
		if !bytes.Contains(page, []byte(m)) {
			t.Errorf("page lacks kit marker %q", m)
		}
	}
}

func TestDeskDoorMapsOntoSessions(t *testing.T) {
	_, a := deskServer(t)
	act := func(who, key string, body map[string]any) *httptest.ResponseRecorder {
		return request(a, who, key, "main", "/api/action", body)
	}
	w := act(token(), "", map[string]any{"action": "new", "id": "desk1", "body": "hello-idea"})
	good(t, w)
	var created map[string]any
	json.Unmarshal(w.Body.Bytes(), &created)
	key, _ := created["ownerKey"].(string)
	if !validToken(key) || created["facilitatorKey"] != key || created["id"] != "desk1" || created["status"] != "pending" {
		t.Fatalf("new: %v", created)
	}
	if strings.Contains(w.Body.String(), "hello-idea") {
		t.Fatal("new re-emitted content")
	}
	// Strangers cannot open/get a private session, nor see it in list.
	for _, name := range []string{"open", "get"} {
		if c := act(token(), "", map[string]any{"action": name, "id": "desk1"}).Code; c != 404 {
			t.Errorf("%s by stranger: want 404, got %d", name, c)
		}
	}
	l := act(token(), "", map[string]any{"action": "list"})
	good(t, l)
	if strings.Contains(l.Body.String(), "desk1") {
		t.Errorf("list leaked a private session: %s", l.Body.String())
	}
	// share needs the facilitator key; the invite admits one person via the page's join.
	if c := act(token(), "", map[string]any{"action": "share", "id": "desk1"}).Code; c != 404 {
		t.Errorf("share by stranger: want 404, got %d", c)
	}
	sh := act(token(), key, map[string]any{"action": "share", "id": "desk1"})
	good(t, sh)
	var shared map[string]any
	json.Unmarshal(sh.Body.Bytes(), &shared)
	invite, _ := shared["invite"].(string)
	if invite == "" || !strings.HasSuffix(shared["inviteURL"].(string), "#invite="+invite) {
		t.Fatalf("share: %v", shared)
	}
	guest := token()
	good(t, request(a, guest, "", "desk1", "/api/action", map[string]any{"action": "join", "invite": invite}))
	good(t, act(guest, "", map[string]any{"action": "open", "id": "desk1"}))
	// The existing page API sees the same session and card; put adds another.
	good(t, act(guest, "", map[string]any{"action": "put", "id": "desk1", "body": "guest-idea"}))
	good(t, request(a, token(), key, "desk1", "/api/action", map[string]any{"action": "phase", "phase": "discuss"}))
	g := act(guest, "", map[string]any{"action": "get", "id": "desk1"})
	good(t, g)
	if !strings.Contains(g.Body.String(), "hello-idea") || !strings.Contains(g.Body.String(), "guest-idea") {
		t.Fatalf("get after reveal lacks cards: %s", g.Body.String())
	}
	if c := act(guest, "", map[string]any{"action": "put", "id": "desk1", "body": "late"}).Code; c != 409 {
		t.Errorf("put after writing ended: want 409, got %d", c)
	}
	if c := act(guest, "", map[string]any{"action": "close", "id": "desk1"}).Code; c != 403 {
		t.Errorf("close by guest: want 403, got %d", c)
	}
	cl := act(token(), key, map[string]any{"action": "close", "id": "desk1"})
	good(t, cl)
	if !strings.Contains(cl.Body.String(), `"status":"closed"`) {
		t.Errorf("close: %s", cl.Body.String())
	}
	if !strings.Contains(request(a, token(), key, "desk1", "/api/state", nil).Body.String(), `"phase":"closed"`) {
		t.Error("close did not close the session on the page API")
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

func TestCLIOpenHidesContent(t *testing.T) {
	srv, _ := deskServer(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	var out bytes.Buffer
	if c := runCLI([]string{"-addr", addr, "--json", "new", "--id", "cli1", "--body", "cli-secret-idea"}, &out); c != 0 {
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
	if len(got) != 3 || got[0] != "cli1" || got[2] != "pending" || !strings.Contains(got[1], "session=cli1") {
		t.Errorf("open must print exactly id url status, got %q", out.String())
	}
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "list", "--key", key}, &out); c != 0 || !strings.Contains(out.String(), "cli1 pending") || strings.Contains(out.String(), "cli-secret-idea") {
		t.Errorf("list: %d %s", c, out.String())
	}
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "bogus"}, &out); c != 2 {
		t.Errorf("unknown verb: want 2, got %d", c)
	}
}
