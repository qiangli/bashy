package kit

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
