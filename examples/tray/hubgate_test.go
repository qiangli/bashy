package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// The tray reads and flips the hub's gate; it never runs a process.
func TestHubGateItemReadsAndFlipsTheGate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	_ = os.MkdirAll(filepath.Join(home, "browser"), 0o700)
	_ = os.WriteFile(filepath.Join(home, "browser", "hub-control.token"), []byte("tok123\n"), 0o600)

	paused := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/gate" {
			http.NotFound(w, r)
			return
		}
		if r.Method == http.MethodPost {
			if r.Header.Get("X-Bashy-Gate-Token") != "tok123" {
				http.Error(w, "bad token", http.StatusForbidden)
				return
			}
			var req struct {
				Paused bool   `json:"paused"`
				By     string `json:"by"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.By != "tray" {
				t.Errorf("by = %q, want tray", req.By)
			}
			paused = req.Paused
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"paused": paused})
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	t.Setenv("BASHY_BROWSER_HUB_PORT", port)

	if s := readHubGate(); s != hubOn || s.title() != "Browser hub: on" {
		t.Fatalf("open gate read as %v %q", s, s.title())
	}
	if err := setHubGate(true); err != nil || readHubGate() != hubOff {
		t.Fatalf("pause: %v, state %v", err, readHubGate())
	}
	if err := setHubGate(false); err != nil || readHubGate() != hubOn {
		t.Fatalf("resume: %v", err)
	}
	t.Setenv("BASHY_BROWSER_HUB_PORT", "1")
	if s := readHubGate(); s != hubDown {
		t.Fatalf("no hub read as %v", s)
	}
}

// One tray item switches every running hub (Chrome and Edge) together.
func TestOneItemSwitchesEveryRunningHub(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	_ = os.MkdirAll(filepath.Join(home, "browser"), 0o700)
	_ = os.WriteFile(filepath.Join(home, "browser", "hub-control.token"), []byte("tok123\n"), 0o600)
	fake := func() (*httptest.Server, *bool) {
		paused := new(bool)
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				var req struct{ Paused bool }
				_ = json.NewDecoder(r.Body).Decode(&req)
				*paused = req.Paused
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"paused": *paused})
		}))
		return srv, paused
	}
	chrome, cp := fake()
	defer chrome.Close()
	edge, ep := fake()
	defer edge.Close()
	_, cport, _ := net.SplitHostPort(chrome.Listener.Addr().String())
	_, eport, _ := net.SplitHostPort(edge.Listener.Addr().String())
	t.Setenv("BASHY_BROWSER_HUB_PORT", cport)
	t.Setenv("BASHY_BROWSER_EDGE_HUB_PORT", eport)

	if s := readAllHubs(); s != hubOn {
		t.Fatalf("both open read as %v", s)
	}
	*ep = true // Edge paused from the CLI
	if s := readAllHubs(); s != hubOff {
		t.Fatalf("one paused read as %v, want off", s)
	}
	if err := setAllHubs(false); err != nil || *cp || *ep {
		t.Fatalf("resume all: %v chrome=%v edge=%v", err, *cp, *ep)
	}
	if err := setAllHubs(true); err != nil || !*cp || !*ep {
		t.Fatalf("pause all: %v chrome=%v edge=%v", err, *cp, *ep)
	}
	t.Setenv("BASHY_BROWSER_EDGE_HUB_PORT", "1") // Edge hub not running
	if s := readAllHubs(); s != hubOff {
		t.Fatalf("chrome paused, edge absent read as %v", s)
	}
}
