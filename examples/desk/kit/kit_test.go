package kit

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var errRenderBoom = errors.New("boom")

func TestDecodeActionRejectsTrailingData(t *testing.T) {
	type req struct {
		Action string `json:"action"`
	}
	cases := []struct {
		body string
		ok   bool
	}{
		{`{"action":"x"}`, true},
		{"{\"action\":\"x\"}\n", true},
		{`{"action":"x"}{"action":"y"}`, false},
		{`{"action":"x"}{}`, false},
		{`{"action":"x"} {"action":"y"}`, false},
		{`{"action":"x"} 1`, false},
		{`{"action":"x"} trailing`, false},
		{`{"action":"x","bogus":1}`, false},
		{``, false},
	}
	for _, c := range cases {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/action", strings.NewReader(c.body))
		var v req
		got := DecodeAction(w, r, 1024, &v)
		if got != c.ok {
			t.Errorf("body %q: DecodeAction = %v, want %v", c.body, got, c.ok)
		}
		if !c.ok && w.Code != http.StatusBadRequest {
			t.Errorf("body %q: status %d, want 400", c.body, w.Code)
		}
	}
}

func TestServeRender(t *testing.T) {
	render := func(src []byte) ([]byte, error) { return append([]byte("<div>"), src...), nil }
	post := func(body string) (int, string) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/api/render", strings.NewReader(body))
		ServeRender(w, r, render)
		return w.Code, w.Body.String()
	}
	code, body := post(`{"text":"# Hi"}`)
	if code != http.StatusOK {
		t.Fatalf("status %d, want 200", code)
	}
	// encoding/json escapes < > & in strings; decode before asserting.
	var got RenderResponse
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("decode: %v (%q)", err, body)
	}
	if got.HTML != "<div># Hi" {
		t.Errorf("unexpected html %q", got.HTML)
	}
	// Malformed bodies are 400: unknown fields, trailing data, empty.
	for _, bad := range []string{`{"text":"x","mode":"y"}`, `{"text":"x"} {}`, ``} {
		if code, _ := post(bad); code != http.StatusBadRequest {
			t.Errorf("body %q: status %d, want 400", bad, code)
		}
	}
	// Renderer failures are 500 without leaking internals.
	w := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/render", strings.NewReader(`{"text":"x"}`))
	ServeRender(w, r, func([]byte) ([]byte, error) { return nil, errRenderBoom })
	if w.Code != http.StatusInternalServerError {
		t.Errorf("render error: status %d, want 500", w.Code)
	}
	if strings.Contains(w.Body.String(), "boom") {
		t.Errorf("error leaked internals: %q", w.Body.String())
	}
	// Only POST is served.
	w = httptest.NewRecorder()
	r = httptest.NewRequest("GET", "/api/render", nil)
	ServeRender(w, r, render)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: status %d, want 405", w.Code)
	}
}

func TestRequestOrigin(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "127.0.0.1:1"
	r.RemoteAddr = "127.0.0.1:5555"
	if got := RequestOrigin(r); got != "http://127.0.0.1:1" {
		t.Errorf("direct: %q", got)
	}
	r.Header.Set("X-Forwarded-Host", "pub.example")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := RequestOrigin(r); got != "https://pub.example" {
		t.Errorf("loopback proxy: %q", got)
	}
	r.RemoteAddr = "10.1.1.1:5555"
	if got := RequestOrigin(r); got != "http://127.0.0.1:1" {
		t.Errorf("forwarded headers from a non-loopback client must be ignored: %q", got)
	}
}

// Behind the app shell (the console's shaped-app proxy, or outpost) a tool
// is reached on loopback with its mount stripped; X-Forwarded-Prefix names
// the mount. Item URLs must name the mount, not the tool's loopback address.
func TestPublicBaseBehindAppShell(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Host = "127.0.0.1:40123"
	r.RemoteAddr = "127.0.0.1:5555"
	if got := PublicBase(r, "http://127.0.0.1:18095"); got != "http://127.0.0.1:18095" {
		t.Errorf("direct: %q", got)
	}
	if got := PublicBase(nil, "http://127.0.0.1:18095/p"); got != "http://127.0.0.1:18095/p" {
		t.Errorf("no request: %q", got)
	}
	r.Header.Set("X-Forwarded-Prefix", "/notepad/")
	if got := PublicBase(r, "http://127.0.0.1:18095"); got != "http://127.0.0.1:40123/notepad" {
		t.Errorf("console mount: %q", got)
	}
	if got := PublicPrefix(r, ""); got != "/notepad" {
		t.Errorf("console mount prefix: %q", got)
	}
	r.Header.Set("X-Forwarded-Host", "pub.example")
	r.Header.Set("X-Forwarded-Proto", "https")
	if got := PublicBase(r, "http://127.0.0.1:18095/sub"); got != "https://pub.example/notepad/sub" {
		t.Errorf("mount + own prefix: %q", got)
	}
	if got := PublicPrefix(r, "/sub"); got != "/notepad/sub" {
		t.Errorf("mount + own prefix path: %q", got)
	}
	for _, bad := range []string{"notepad", "//evil.example", "/a\"b", "/a b"} {
		r.Header.Set("X-Forwarded-Prefix", bad)
		if got := PublicPrefix(r, "/sub"); got != "/sub" {
			t.Errorf("malformed prefix %q must be ignored: %q", bad, got)
		}
	}
	r.Header.Set("X-Forwarded-Prefix", "/notepad")
	r.RemoteAddr = "10.1.1.1:5555"
	if got := PublicBase(r, "http://127.0.0.1:18095"); got != "http://127.0.0.1:18095" {
		t.Errorf("forwarded prefix from a non-loopback client must be ignored: %q", got)
	}
}
