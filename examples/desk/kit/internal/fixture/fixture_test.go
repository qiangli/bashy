// Fixture tests: the shared contract test goes green against the clean
// fixture server, red against each -break variant, and the CLI door hides
// content on open exactly like the HTTP door (duality).
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"desk-kit/contracttest"
)

func startServer(t *testing.T, breakM string) string {
	t.Helper()
	old := breakMode
	breakMode = breakM
	t.Cleanup(func() { breakMode = old })
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "items"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{dir: dir, items: map[string]*item{}}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	a.baseURL = srv.URL
	return srv.URL
}

func TestContractGreen(t *testing.T) {
	url := startServer(t, "")
	contracttest.Run(t, contracttest.Config{BaseURL: url, Tool: "memo"})
}

func TestContractRedOnBreak(t *testing.T) {
	for _, breakM := range []string{"open-emits-content", "no-csp", "allow-secret", "hex-color", "font-family", "no-shell", "no-schema-version", "accept-old-verb"} {
		url := startServer(t, breakM)
		fails := contracttest.Check(contracttest.Config{BaseURL: url, Tool: "memo"})
		if len(fails) == 0 {
			t.Errorf("break %q: contract test stayed green, want red", breakM)
			continue
		}
		if strings.HasPrefix(breakM, "hex") || strings.HasPrefix(breakM, "font") || breakM == "no-shell" {
			if !strings.Contains(strings.Join(fails, "\n"), "kit conformance") {
				t.Errorf("break %q: want a kit conformance failure, got %v", breakM, fails)
			}
		}
		t.Logf("break %q correctly fails %d contract point(s), e.g.: %s", breakM, len(fails), fails[0])
	}
}

func TestContractRedOnMissingCommonOption(t *testing.T) {
	url := startServer(t, "")
	fails := contracttest.Check(contracttest.Config{BaseURL: url, Tool: "memo", CLI: func(args ...string) (int, string) {
		return 2, "unsupported common option"
	}})
	if !strings.Contains(strings.Join(fails, "\n"), "common option") {
		t.Fatalf("missing common option stayed green: %v", fails)
	}
}

func TestContractRedOnNonLoopbackURL(t *testing.T) {
	// Pure origin rule, no network: any non-loopback BaseURL is a violation.
	for _, base := range []string{"http://10.9.9.9:18097", "http://example.test/", "http://0.0.0.0:18097/"} {
		if v := contracttest.LoopbackViolation(base); !strings.Contains(v, "loopback") {
			t.Errorf("BaseURL %q: want a loopback violation, got %q", base, v)
		}
	}
	if v := contracttest.LoopbackViolation(startServer(t, "")); v != "" {
		t.Errorf("loopback server URL: want no violation, got %q", v)
	}
}

// The CLI door mirrors the HTTP door: open re-surfaces id+url+status only,
// get returns the content.
func TestCLIOpenHidesContent(t *testing.T) {
	srvURL := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer
	if code := runCLI([]string{"-addr", addr, "--json", "--id", "cli-probe", "--title", "T", "--body", "cli-secret-body", "add"}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}
	var created map[string]any
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("new: not JSON: %s", out.String())
	}
	key, _ := created["ownerKey"].(string)
	if key == "" {
		t.Fatalf("new: no ownerKey in %s", out.String())
	}
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "open", "cli-probe"}, &out); code != 0 {
		t.Fatalf("open: exit %d: %s", code, out.String())
	}
	if strings.Contains(out.String(), "cli-secret-body") {
		t.Errorf("CLI open re-emits content: %s", out.String())
	}
	if !strings.Contains(out.String(), "cli-probe") {
		t.Errorf("CLI open lacks the id: %s", out.String())
	}
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "--key", key, "show", "cli-probe"}, &out); code != 0 {
		t.Fatalf("get: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "cli-secret-body") {
		t.Errorf("CLI get must return content: %s", out.String())
	}
}

// A failed save must never leave the in-memory item mutated (share/join
// used to shallow-copy the Invites/Members maps).
func TestFailedSaveDoesNotMutateItem(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "items"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{dir: dir, items: map[string]*item{}}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	a.baseURL = srv.URL
	post := func(key, body string) (int, map[string]any) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/action", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Facilitator-Key", key)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		var v map[string]any
		_ = json.NewDecoder(r.Body).Decode(&v)
		return r.StatusCode, v
	}
	code, created := post("", `{"action":"add","id":"fs","title":"t"}`)
	if code != 200 {
		t.Fatalf("new: %d", code)
	}
	key := created["ownerKey"].(string)
	code, shared := post(key, `{"action":"share","id":"fs"}`)
	if code != 200 {
		t.Fatalf("share: %d", code)
	}
	invite := shared["invite"].(string)
	// Break storage: items becomes a regular file so every save fails.
	if err := os.RemoveAll(filepath.Join(dir, "items")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "items"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	invitesBefore, membersBefore := len(a.items["fs"].Invites), len(a.items["fs"].Members)
	if code, _ := post(key, `{"action":"share","id":"fs"}`); code != 500 {
		t.Fatalf("share on broken storage: want 500, got %d", code)
	}
	if got := len(a.items["fs"].Invites); got != invitesBefore {
		t.Errorf("failed share leaked an invite into memory: %d -> %d", invitesBefore, got)
	}
	if code, _ := post("", `{"action":"join","id":"fs","invite":"`+invite+`"}`); code != 500 {
		t.Fatalf("join on broken storage: want 500, got %d", code)
	}
	if !a.items["fs"].Invites[invite] || len(a.items["fs"].Members) != membersBefore {
		t.Errorf("failed join mutated memory: invites=%v members=%v", a.items["fs"].Invites, a.items["fs"].Members)
	}
}
