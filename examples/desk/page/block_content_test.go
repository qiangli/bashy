package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

func TestEmbedFramesKitAppsAndLinksOutEngines(t *testing.T) {
	_, a := startServer(t, "")
	key := addPageBody(t, a, "p", "# C\n\n```block embed:/notepad/?id=n1 id=notes\n```\n\n"+
		"```block embed:/whiteboard/?room=r1 id=board\n```\n\n```block embed:/apps/klatch/ id=chat\n```\n\n"+
		"```block embed:/chart/?id=c1 id=linked mode=link\n```\n")
	page := getPage(t, a, "p", key)
	for id, framed := range map[string]bool{"notes": true, "board": false, "chat": false, "linked": false} {
		start := strings.Index(page, `data-block="`+id+`"`)
		if start < 0 {
			t.Fatalf("block %s missing:\n%s", id, page)
		}
		section := page[start:]
		section = section[:strings.Index(section, "</section>")]
		if got := strings.Contains(section, "<iframe"); got != framed {
			t.Errorf("block %s: iframe=%v, want %v:\n%s", id, got, framed, section)
		}
		if !strings.Contains(section, `target="_blank" rel="noopener noreferrer"`) {
			t.Errorf("block %s lacks its link-out card:\n%s", id, section)
		}
	}
	w := do(a, "GET", "/?id=p", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key})
	if csp := w.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'self'") {
		t.Errorf("CSP must allow same-origin framing only, got %q", csp)
	}
}

func TestTimerControlsAndSSE(t *testing.T) {
	_, a := startServer(t, "")
	key := addPageBody(t, a, "p", "# C\n\n```block timer:break duration=2s\n```\n")
	owner := map[string]string{"X-Facilitator-Key": key}
	start := `{"action":"timer","id":"p","kind":"start","block":"timer-1"}`
	w := do(a, "GET", "/api/block?id=p&block=timer-1", "", "127.0.0.1:5000", nil)
	if body := w.Body.String(); !strings.Contains(body, "0:02") || strings.Contains(body, "data-timer-action") {
		t.Errorf("participant timer: want the clock without controls: %s", body)
	}
	if w := do(a, "POST", "/api/action", start, "127.0.0.1:5000", nil); w.Code != http.StatusForbidden {
		t.Errorf("participant timer start: want 403, got %d %s", w.Code, w.Body)
	}
	if w := do(a, "POST", "/api/action?view=participant", start, "127.0.0.1:5000", owner); w.Code != http.StatusForbidden {
		t.Errorf("organizer previewing as participant: want 403, got %d %s", w.Code, w.Body)
	}
	if w := do(a, "POST", "/api/action", start, "127.0.0.1:5000", owner); w.Code != 200 {
		t.Fatalf("organizer start: %d %s", w.Code, w.Body)
	}
	frag := do(a, "GET", "/api/block?id=p&block=timer-1", "", "127.0.0.1:5000", owner).Body.String()
	if !strings.Contains(frag, "Running") || !strings.Contains(frag, `data-timer-action="stop"`) {
		t.Errorf("organizer running timer fragment: %s", frag)
	}
	// The ticker publishes block frames until zero, then records it finished.
	deadline := time.Now().Add(5 * time.Second)
	for {
		a.mu.Lock()
		ts := *a.items["p"].Timers["timer-1"]
		a.mu.Unlock()
		if ts.Finished && !ts.Running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timer never finished: %+v", ts)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if md := do(a, "GET", "/api/export?id=p&to=md", "", "127.0.0.1:5000", owner).Body.String(); !strings.Contains(md, "break: 0:00 (finished)") {
		t.Errorf("export should carry the timer state:\n%s", md)
	}
	if w := do(a, "POST", "/api/action", `{"action":"timer","id":"p","kind":"reset","block":"timer-1"}`, "127.0.0.1:5000", owner); w.Code != 200 {
		t.Fatalf("reset: %d %s", w.Code, w.Body)
	}
	a.mu.Lock()
	ts := *a.items["p"].Timers["timer-1"]
	a.mu.Unlock()
	if ts.Running || ts.Finished || ts.Remaining != 2 {
		t.Errorf("after reset timer = %+v", ts)
	}
}

func TestTimerDisplayOnlyWhenProjected(t *testing.T) {
	_, a := startServer(t, "")
	key := addPageBody(t, a, "p", "# C\n\n```block timer:break duration=90s\n```\n")
	display := func() string {
		return do(a, "GET", "/?id=p&view=display", "", "127.0.0.1:5000", nil).Body.String()
	}
	if strings.Contains(display(), "1:30") {
		t.Error("an unprojected timer must stay off the display")
	}
	owner := map[string]string{"X-Facilitator-Key": key}
	if w := do(a, "POST", "/api/action", `{"action":"set","id":"p","faces":{"timer-1":{"project":true}}}`, "127.0.0.1:5000", owner); w.Code != 200 {
		t.Fatalf("project: %d %s", w.Code, w.Body)
	}
	page := display()
	if !strings.Contains(page, "1:30") || strings.Contains(page, `data-timer-action="`) {
		t.Errorf("projected timer: want the clock without controls:\n%s", page)
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
