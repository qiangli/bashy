package contracttest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// sseWait bounds every wait for one live frame.
const sseWait = 3 * time.Second

type sseEvent struct {
	ID    string
	Data  string
	Frame struct {
		Item     string `json:"item"`
		Block    string `json:"block"`
		Revision int    `json:"revision"`
		Resync   bool   `json:"resync"`
	}
}

type sseStream struct {
	header http.Header
	events chan sseEvent
	errs   chan error
	cancel context.CancelFunc
	mu     sync.Mutex
	raw    strings.Builder
}

// text is everything read from the stream so far.
func (s *sseStream) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.raw.String()
}

// openStream opens GET /api/events?<param>=id, optionally resuming from
// lastID, and reads frames in the background.
func (t *check) openStream(id, lastID string, headers map[string]string) (*sseStream, int, error) {
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", t.cfgBase()+"/api/events?"+t.cfg.param()+"="+url.QueryEscape(id), nil)
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	r, err := (&http.Client{}).Do(req)
	if err != nil {
		cancel()
		return nil, 0, err
	}
	if r.StatusCode != http.StatusOK {
		r.Body.Close()
		cancel()
		return nil, r.StatusCode, fmt.Errorf("status %d", r.StatusCode)
	}
	s := &sseStream{header: r.Header, events: make(chan sseEvent, 64), errs: make(chan error, 1), cancel: cancel}
	lines := make(chan string, 256)
	go func() {
		defer r.Body.Close()
		defer close(lines)
		sc := bufio.NewScanner(r.Body)
		for sc.Scan() {
			lines <- sc.Text()
		}
	}()
	go func() {
		var ev sseEvent
		var data []string
		for line := range lines {
			s.mu.Lock()
			s.raw.WriteString(line + "\n")
			s.mu.Unlock()
			switch {
			case line == "":
				if len(data) == 0 {
					continue // retry:, comments and keepalives
				}
				ev.Data = strings.Join(data, "\n")
				if err := json.Unmarshal([]byte(ev.Data), &ev.Frame); err != nil {
					s.errs <- fmt.Errorf("frame data is not a JSON frame: %q", trunc(ev.Data, 80))
					cancel()
					return
				}
				select {
				case s.events <- ev:
				case <-ctx.Done():
					return
				}
				ev, data = sseEvent{}, nil
			case strings.HasPrefix(line, "id:"):
				ev.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			case strings.HasPrefix(line, "data:"):
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		s.errs <- errors.New("stream ended")
	}()
	return s, http.StatusOK, nil
}

func (s *sseStream) next() (sseEvent, error) {
	select {
	case ev := <-s.events:
		return ev, nil
	case err := <-s.errs:
		return sseEvent{}, err
	case <-time.After(sseWait):
		return sseEvent{}, fmt.Errorf("no frame within %s", sseWait)
	}
}

func (s *sseStream) close() { s.cancel() }

// The live layer (Config.Events): one stream per item whose frames say
// which item and block changed at what revision, never the content; each
// frame has an id so Last-Event-ID resumes with no gap and no replay.
func (t *check) events() {
	p := t.cfg.param()
	for _, c := range []struct {
		query string
		want  int
	}{{"", http.StatusBadRequest}, {"?" + p + "=bad!!name", http.StatusBadRequest}, {"?" + p + "=no-such-item", http.StatusNotFound}} {
		if code, _, _ := t.get("/api/events"+c.query, nil); code != c.want {
			t.fail("events: GET /api/events%s: want %d, got %d", c.query, c.want, code)
		}
	}
	canary, update := "events-canary-content", "events-canary-update"
	code, raw, _ := t.post("/api/action", fmt.Sprintf(`{"action":"add","title":"events probe","body":%q}`, canary), nil)
	if code != http.StatusOK {
		t.fail("events: add: want 200, got %d", code)
		return
	}
	created := decode(t, raw)
	if created == nil {
		return
	}
	id, _ := created["id"].(string)
	auth := map[string]string{}
	if key, _ := created["ownerKey"].(string); key != "" {
		auth["X-Facilitator-Key"] = key
	}
	set := func(n int) bool {
		body := fmt.Sprintf(`{"action":"set","id":%q,"body":"%s-%d"}`, id, update, n)
		if code, _, _ := t.post("/api/action", body, auth); code != http.StatusOK {
			t.fail("events: set: want 200, got %d", code)
			return false
		}
		return true
	}
	var raws []string
	defer func() {
		for _, r := range raws {
			if strings.Contains(r, canary) || strings.Contains(r, update) {
				t.fail("events: a frame carries item content; frames are {item, block, revision} only")
				return
			}
		}
	}()
	frame := func(where string, ev sseEvent) bool {
		if ev.ID == "" {
			t.fail("events: %s: frame has no id, so Last-Event-ID cannot resume", where)
			return false
		}
		if ev.Frame.Item != id {
			t.fail("events: %s: frame item %q, want %q", where, ev.Frame.Item, id)
			return false
		}
		return true
	}

	s, code, err := t.openStream(id, "", auth)
	if err != nil {
		t.fail("events: GET /api/events?%s=ID: want a 200 text/event-stream, got %d (%v)", p, code, err)
		return
	}
	if ct := s.header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.fail("events: Content-Type %q, want text/event-stream", ct)
	}
	if s.header.Get("Cache-Control") != "no-store" || s.header.Get("X-Content-Type-Options") != "nosniff" ||
		FramingViolation(s.header.Get("Content-Security-Policy"), t.cfg.Embeddable) != "" {
		t.fail("events: the stream must carry the desk security headers")
	}
	first, err := s.next()
	if err != nil {
		s.close()
		t.fail("events: a fresh stream must open with the item's current frame: %v", err)
		return
	}
	if !frame("fresh stream", first) || first.Frame.Block != "" || first.Frame.Resync {
		s.close()
		t.fail("events: a fresh stream must open with a whole-item frame (block \"\") at the current revision, got %s", first.Data)
		return
	}
	if !set(1) {
		s.close()
		return
	}
	live, err := s.next()
	s.close()
	raws = append(raws, s.text())
	if err != nil {
		t.fail("events: set must publish a frame to open streams: %v", err)
		return
	}
	if !frame("live", live) {
		return
	}
	if live.Frame.Revision <= first.Frame.Revision || live.Frame.Resync || live.ID == first.ID {
		t.fail("events: a change must arrive as a new frame with a higher revision (had %s, got %s)", first.Data, live.Data)
		return
	}

	// Resume: two changes while away, then reconnect from the last id.
	if !set(2) || !set(3) {
		return
	}
	r, code, err := t.openStream(id, live.ID, auth)
	if err != nil {
		t.fail("events: resume from Last-Event-ID: want 200, got %d (%v)", code, err)
		return
	}
	prev := live
	for i := 1; i <= 2; i++ {
		ev, err := r.next()
		if err != nil {
			t.fail("events: resume from Last-Event-ID must replay each missed frame (got %d of 2): %v", i-1, err)
			break
		}
		if !frame("resume", ev) {
			break
		}
		if ev.Frame.Resync || ev.Frame.Revision <= prev.Frame.Revision || ev.ID == prev.ID {
			t.fail("events: resume from Last-Event-ID must continue after %s with no replay and no resync, got %s", prev.Data, ev.Data)
			break
		}
		prev = ev
	}
	r.close()
	raws = append(raws, r.text())

	// An id this server never issued cannot be continued: resync.
	u, code, err := t.openStream(id, "no-such-event-id", auth)
	if err != nil {
		t.fail("events: unknown Last-Event-ID: want 200 and a resync frame, got %d (%v)", code, err)
		return
	}
	ev, err := u.next()
	u.close()
	raws = append(raws, u.text())
	if err != nil {
		t.fail("events: unknown Last-Event-ID must be answered with a resync frame: %v", err)
		return
	}
	if frame("resync", ev) && (!ev.Frame.Resync || ev.Frame.Block != "" || ev.Frame.Revision < prev.Frame.Revision) {
		t.fail("events: unknown Last-Event-ID must be answered with a whole-item resync frame at the current revision, got %s", ev.Data)
	}
}

// The LAN guard (Config.BindGuard): loopback by default; any other bind
// only through the explicit --lan escape hatch, and that override is
// logged on start. A malformed address is refused either way.
func (t *check) bindGuard() {
	if _, err := t.cfg.BindGuard("127.0.0.1:0", false); err != nil {
		t.fail("LAN guard: loopback bind refused: %v", err)
	}
	for _, addr := range []string{"0.0.0.0:18097", ":18097", "192.0.2.10:18097", "[::]:18097"} {
		if _, err := t.cfg.BindGuard(addr, false); err == nil {
			t.fail("LAN guard: %s accepted without --lan", addr)
		}
		logged, err := t.cfg.BindGuard(addr, true)
		if err != nil {
			t.fail("LAN guard: %s refused with the explicit --lan override: %v", addr, err)
			continue
		}
		if !strings.Contains(logged, "--lan") || !strings.Contains(logged, addr) {
			t.fail("LAN guard: the --lan override for %s must be logged on start naming the flag and the address, got %q", addr, trunc(logged, 120))
		}
	}
	if _, err := t.cfg.BindGuard("not-an-address", true); err == nil {
		t.fail("LAN guard: malformed address accepted with --lan")
	}
}
