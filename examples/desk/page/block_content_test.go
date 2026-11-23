package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// fakeSibling serves the viewer/notepad show contract.
func fakeSibling(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action, ID, Page string
		}
		json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch req.ID {
		case "sprint-537":
			json.NewEncoder(w).Encode(map[string]any{"title": "Sprint 537", "page": map[string]any{"title": "Report", "markdown": "# Hello\n\nBook *content*."}})
		case "note-1":
			json.NewEncoder(w).Encode(map[string]any{"title": "Agenda", "body": "Note **body** here."})
		default:
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"error": "not found"})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBookAndNoteBlocksInline(t *testing.T) {
	srv := fakeSibling(t)
	t.Setenv("VIEWER_ADDR", srv.URL)
	t.Setenv("NOTEPAD_ADDR", srv.URL)
	_, a := startServer(t, "")
	key := addPageBody(t, a, "p", "# Class\n\n```block book:sprint-537/p1\n```\n\n```block note:note-1\n```\n")
	page := getPage(t, a, "p", key)
	for _, want := range []string{"Hello", "<em>content</em>", "Report", "Note", "<strong>body</strong>", `data-kind="book"`, `data-kind="note"`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q:\n%s", want, page)
		}
	}
	if strings.Contains(page, "**body**") {
		t.Error("note markdown was not rendered")
	}
	// Missing sibling item degrades to a placeholder, not an error page.
	key2 := addPageBody(t, a, "q", "# C\n\n```block book:missing/p1\n```\n")
	page2 := getPage(t, a, "q", key2)
	if !strings.Contains(page2, "not available") {
		t.Errorf("missing book should degrade:\n%s", page2)
	}
}

func TestEmbedBlockAndCSP(t *testing.T) {
	_, a := startServer(t, "")
	key := addPageBody(t, a, "p", "# C\n\n```block embed:/whiteboard/?room=r1\n```\n\n```block embed:https://example.test/projector\n```\n")
	page := getPage(t, a, "p", key)
	if !strings.Contains(page, `<iframe`) || !strings.Contains(page, `src="/whiteboard/?room=r1"`) || !strings.Contains(page, `src="https://example.test/projector"`) {
		t.Errorf("embed iframe missing:\n%s", page)
	}
	w := do(a, "GET", "/?id=p", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key})
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Errorf("CSP must allow same-origin framing, got %q", csp)
	}
	for _, bad := range []string{"```block embed:javascript:alert(1)\n```\n", "```block embed://evil.example/x\n```\n"} {
		if _, err := parseBody(bad); err == nil {
			t.Errorf("parseBody(%q): want error", bad)
		}
	}
}

func TestTimerControlsAndSSE(t *testing.T) {
	_, a := startServer(t, "")
	key := addPageBody(t, a, "p", "# C\n\n```block timer:break duration=2s\n```\n")
	owner := map[string]string{"X-Facilitator-Key": key}
	// Participant view has the clock but no controls.
	w := do(a, "GET", "/api/block?id=p&block=timer-1", "", "127.0.0.1:5000", nil)
	if !strings.Contains(w.Body.String(), "2:00") && !strings.Contains(w.Body.String(), "0:02") {
		t.Errorf("timer fragment should show duration: %s", w.Body)
	}
	// Non-organizer cannot control.
	if w := do(a, "POST", "/api/action", `{"action":"timer","id":"p","kind":"start","block":"timer-1"}`, "127.0.0.1:5000", nil); w.Code != http.StatusForbidden {
		t.Errorf("participant timer start: want 403, got %d %s", w.Code, w.Body)
	}
	if w := do(a, "POST", "/api/action", `{"action":"timer","id":"p","kind":"start","block":"timer-1"}`, "127.0.0.1:5000", owner); w.Code != 200 {
		t.Fatalf("organizer start: %d %s", w.Code, w.Body)
	}
	a.mu.Lock()
	ts := a.items["p"].Timers["timer-1"]
	running := ts != nil && ts.Running
	a.mu.Unlock()
	if !running {
		t.Fatal("timer should be running after start")
	}
	// A tick frame for the timer block is published on the live stream.
	if id := a.events.Publish("p", "timer-1", a.items["p"].Revision); id == "" {
		t.Error("Publish should return an event id for the timer tick")
	}
	frag := do(a, "GET", "/api/block?id=p&block=timer-1", "", "127.0.0.1:5000", owner)
	if !strings.Contains(frag.Body.String(), "Running") {
		t.Errorf("running timer fragment: %s", frag.Body)
	}
	// Stop and reset restore the full duration.
	if w := do(a, "POST", "/api/action", `{"action":"timer","id":"p","kind":"reset","block":"timer-1"}`, "127.0.0.1:5000", owner); w.Code != 200 {
		t.Fatalf("reset: %d %s", w.Code, w.Body)
	}
	a.mu.Lock()
	ts = a.items["p"].Timers["timer-1"]
	reset := ts != nil && !ts.Running && ts.Remaining == 2
	a.mu.Unlock()
	if !reset {
		t.Errorf("after reset timer = %+v", ts)
	}
	// Export inlines the timer's final state, participant page has controls only for organizer.
	page := getPage(t, a, "p", key)
	if !strings.Contains(page, `data-timer-action="start"`) {
		t.Error("organizer page should carry timer controls")
	}
}

func addPageBody(t *testing.T, a *app, id, body string) string {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"action": "add", "id": id, "body": body, "audience": "host"})
	w := do(a, "POST", "/api/action", string(raw), "127.0.0.1:5000", nil)
	var v struct{ OwnerKey string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || v.OwnerKey == "" {
		t.Fatalf("add %s: %d %s", id, w.Code, w.Body)
	}
	return v.OwnerKey
}

func getPage(t *testing.T, a *app, id, key string) string {
	t.Helper()
	w := do(a, "GET", "/?id="+id, "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key})
	if w.Code != 200 {
		t.Fatalf("page %s: %d %s", id, w.Code, w.Body)
	}
	return w.Body.String()
}
