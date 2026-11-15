package agentos

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestShapePageCommands(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		cookie, err := r.Cookie("bashy_console")
		if err != nil || cookie.Value != "test-session" {
			t.Errorf("missing session")
		}
		if r.URL.Path != "/prefix/api/page/console/invoke" || r.Method != "POST" {
			t.Errorf("route %s %s", r.Method, r.URL.Path)
		}
		var body struct {
			PageID string         `json:"page_id"`
			Action string         `json:"action"`
			Input  map[string]any `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.PageID != "p1" || body.Action != "presentation.shape-only" || body.Input["enabled"] != false {
			t.Errorf("body %#v", body)
		}
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"id":"c1","status":"queued"}`))
	}))
	defer server.Close()
	cookie := filepath.Join(t.TempDir(), "cookie")
	if err := os.WriteFile(cookie, []byte("test-session\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := bashyShapeCmd()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetArgs([]string{"--remote", server.URL + "/prefix", "--cookie-file", cookie, "page", "invoke", "console", "p1", "presentation.shape-only", `{"enabled":false}`})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if calls != 1 || !bytes.Contains(out.Bytes(), []byte(`"id":"c1"`)) {
		t.Fatalf("calls=%d result=%s", calls, out)
	}
}

func TestShapePageNoRetryOrRedirect(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Redirect(w, r, "/login", 303) }))
	defer server.Close()
	cmd := bashyShapeCmd()
	cmd.SetArgs([]string{"--console", server.URL, "page", "invoke", "console", "p1", "shape.submit"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("redirect accepted")
	}
	if calls != 1 {
		t.Fatalf("requests %d", calls)
	}
}
