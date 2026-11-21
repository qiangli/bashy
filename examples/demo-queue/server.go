package main

import (
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed web/*
var web embed.FS

type room struct {
	mu sync.Mutex
	*session
	owner, display, peer, service string
}

type action struct {
	Name, ID, Action string
	Limit, Warning   int
	Turn, Generation uint64
}

func response(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func fail(w http.ResponseWriter, status int, message string) {
	response(w, status, map[string]string{"error": message})
}

func readAction(w http.ResponseWriter, r *http.Request) (action, bool) {
	var a action
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&a); err != nil {
		fail(w, 400, "Invalid request.")
		return a, false
	}
	return a, true
}

func equal(a, b string) bool {
	return a != "" && subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func loopback(addr string, next http.Handler) http.Handler {
	origin := "http://" + addr
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; media-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'")
		if r.Host != addr || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(w, 403, "Open the local URL printed by Demo Queue.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func page(w http.ResponseWriter, r *http.Request) {
	name := "web" + r.URL.Path
	switch r.URL.Path {
	case "/control", "/projector", "/present":
		name = "web/index.html"
	case "/app.js", "/style.css":
	default:
		http.NotFound(w, r)
		return
	}
	b, err := web.ReadFile(name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch name {
	case "web/index.html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case "web/app.js":
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case "web/style.css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	_, _ = w.Write(b)
}

func (s *room) local(addr string) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /", page)
	m.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		if !equal(r.URL.Query().Get("key"), s.owner) && !equal(r.URL.Query().Get("key"), s.display) {
			fail(w, 401, "Open the private session URL.")
			return
		}
		s.events(w, r, nil, "")
	})
	m.HandleFunc("POST /api/control", s.control)
	m.HandleFunc("POST /api/viewer", s.connectViewer)
	m.HandleFunc("POST /api/rendered", s.rendered)
	m.HandleFunc("GET /api/chunk", s.chunk)
	return loopback(addr, m)
}

func (s *room) mesh() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("POST /api/join", func(w http.ResponseWriter, r *http.Request) {
		a, ok := readAction(w, r)
		if !ok {
			return
		}
		name := strings.TrimSpace(a.Name)
		if name == "" || len(name) > 80 {
			fail(w, 400, "Enter a display name of up to 80 bytes.")
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		p := s.invites[bearer(r)]
		if p == nil || s.ended {
			fail(w, 403, "This invite is invalid or the session ended.")
			return
		}
		lease := s.join(p, name, time.Now())
		response(w, 200, map[string]string{"lease": lease})
	})
	m.HandleFunc("GET /api/events", s.member(func(w http.ResponseWriter, r *http.Request, p *participant) {
		s.events(w, r, p, r.Header.Get("X-Demo-Lease"))
	}))
	m.HandleFunc("POST /api/prepare", s.member(s.prepare))
	m.HandleFunc("POST /api/member", s.member(s.memberAction))
	m.HandleFunc("POST /api/upload", s.member(s.upload))
	return m
}

func bearer(r *http.Request) string {
	token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token
}

func (s *room) member(next func(http.ResponseWriter, *http.Request, *participant)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		p := s.invites[bearer(r)]
		valid := p != nil && equal(r.Header.Get("X-Demo-Lease"), p.lease)
		s.mu.Unlock()
		if !valid {
			fail(w, 401, "Join this session again.")
			return
		}
		next(w, r, p)
	}
}

func (s *room) authorized(r *http.Request, p *participant) bool {
	return !s.ended && equal(r.Header.Get("X-Demo-Lease"), p.lease)
}

func (s *room) prepare(w http.ResponseWriter, r *http.Request, p *participant) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, probeSize))
	if err != nil || len(body) != probeSize {
		fail(w, 400, "Connection check did not complete.")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorized(r, p) {
		fail(w, 409, "The session is no longer available.")
		return
	}
	p.probed = time.Now()
	w.WriteHeader(204)
}

func (s *room) memberAction(w http.ResponseWriter, r *http.Request, p *participant) {
	a, ok := readAction(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.authorized(r, p) {
		fail(w, 409, "The session is no longer available.")
		return
	}
	now := time.Now()
	s.tick(now)
	switch a.Action {
	case "ready":
		if err := s.ready(p, now); err != nil {
			fail(w, 409, err.Error())
			return
		}
	case "withdraw":
		s.withdraw(p, "Joined", now)
	case "finish":
		if t := s.current; t != nil && t.member == p && t.ID == a.Turn {
			s.finish("Finished", "finished early", now)
		}
	default:
		fail(w, 400, "Unknown presenter action.")
		return
	}
	w.WriteHeader(204)
}

func (s *room) control(w http.ResponseWriter, r *http.Request) {
	if !equal(bearer(r), s.owner) {
		fail(w, 403, "Organizer access is required.")
		return
	}
	a, ok := readAction(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.tick(now)
	if s.ended {
		fail(w, 409, "The session already ended.")
		return
	}
	switch a.Action {
	case "invite":
		invite, _ := json.Marshal([]string{s.peer, s.service, s.issue()})
		response(w, 200, map[string]string{"invite": base64.RawURLEncoding.EncodeToString(invite)})
		return
	case "start":
		if s.started || a.Limit < 1 || a.Limit > 3600 || a.Warning < 0 || a.Warning >= a.Limit {
			fail(w, 400, "Set a turn from 1 to 3600 seconds and a shorter warning.")
			return
		}
		if s.viewer == "" || now.Sub(s.viewerSeen) >= offline {
			fail(w, 409, "Open the projector view before starting.")
			return
		}
		s.limit, s.warning, s.started = a.Limit, a.Warning, true
		s.advance(now)
	case "skip":
		if s.current != nil && s.current.ID == a.Turn {
			s.finish("Skipped", "skipped by organizer", now)
		}
	case "assign":
		for _, p := range s.roster {
			if p.ID == a.ID && p.Status != "Offline" && (s.current == nil || s.current.member != p) {
				s.assigned = p
				s.advance(now)
				w.WriteHeader(204)
				return
			}
		}
		fail(w, 409, "Choose a connected presenter who is waiting.")
		return
	case "end":
		s.ended, s.assigned, s.current, s.queue = true, nil, nil, nil
	default:
		fail(w, 400, "Unknown organizer action.")
		return
	}
	w.WriteHeader(204)
}

func (s *room) snapshot(p *participant, now time.Time) map[string]any {
	state := map[string]any{
		"started": s.started, "ended": s.ended, "warning": s.warning,
		"notice": s.notice, "now": now.UnixMilli(),
		"projector": s.viewer != "" && now.Sub(s.viewerSeen) < offline,
	}
	if s.current != nil {
		state["turn"] = *s.current
	}
	if len(s.queue) > 0 {
		state["next"] = s.queue[0].Name
	}
	if p != nil {
		me := *p
		for i, queued := range s.queue {
			if queued == p {
				me.Position = i + 1
			}
		}
		state["me"] = me
		state["assigned"] = s.assigned == p
		if s.current == nil || s.current.member != p {
			delete(state, "turn")
		}
		return state
	}
	roster := make([]participant, 0, len(s.roster))
	for _, person := range s.roster {
		copy := *person
		for i, queued := range s.queue {
			if queued == person {
				copy.Position = i + 1
			}
		}
		roster = append(roster, copy)
	}
	state["roster"] = roster
	if s.assigned != nil {
		state["assigned"] = s.assigned.Name
	}
	return state
}

func (s *room) events(w http.ResponseWriter, r *http.Request, p *participant, lease string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	controller := http.NewResponseController(w)
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	first := true
	viewer := r.URL.Query().Get("viewer")
	isProjector := equal(r.URL.Query().Get("key"), s.display) && viewer != ""
	if isProjector {
		defer func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if equal(viewer, s.viewer) {
				s.viewerSeen = time.Time{}
			}
		}()
	}
	for {
		s.mu.Lock()
		now := time.Now()
		if p != nil && !equal(lease, p.lease) {
			s.mu.Unlock()
			return
		}
		if p != nil {
			if first && p.Status == "Offline" {
				p.Status = "Joined"
			}
			p.seen = now
		}
		if isProjector {
			if !equal(viewer, s.viewer) {
				s.mu.Unlock()
				return
			}
			if first && s.viewerSeen.IsZero() && s.current != nil {
				s.current.resetMedia()
			}
			s.viewerSeen = now
			s.advance(now)
		}
		first = false
		s.tick(now)
		data, _ := json.Marshal(s.snapshot(p, now))
		ended := s.ended
		s.mu.Unlock()
		_ = controller.SetWriteDeadline(now.Add(2 * time.Second))
		if _, err := w.Write(append(append([]byte("data: "), data...), '\n', '\n')); err != nil || controller.Flush() != nil || ended {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *room) connectViewer(w http.ResponseWriter, r *http.Request) {
	if !equal(bearer(r), s.display) {
		fail(w, 403, "Open the private projector URL.")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewer, s.viewerSeen = secret(), time.Now()
	if t := s.current; t != nil {
		t.resetMedia()
	}
	s.advance(s.viewerSeen)
	response(w, 200, map[string]string{"viewer": s.viewer})
}

func (s *room) validViewer(r *http.Request) bool {
	return equal(bearer(r), s.display) && equal(r.Header.Get("X-Demo-Viewer"), s.viewer)
}

func (s *room) rendered(w http.ResponseWriter, r *http.Request) {
	a, ok := readAction(w, r)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.validViewer(r) {
		fail(w, 403, "Projector access is required.")
		return
	}
	now := time.Now()
	s.tick(now)
	t := s.current
	if t == nil || t.ID != a.Turn || t.Generation != a.Generation || t.mime == "" {
		fail(w, 409, "The turn has changed.")
		return
	}
	if t.Deadline.IsZero() {
		t.Deadline = now.Add(time.Duration(s.limit) * time.Second)
	}
	t.Live = true
	t.member.Status = "Presenting"
	w.WriteHeader(204)
}

func numbers(r *http.Request) (turn, generation, seq uint64, err error) {
	q := r.URL.Query()
	turn, a := strconv.ParseUint(q.Get("turn"), 10, 64)
	generation, b := strconv.ParseUint(q.Get("generation"), 10, 64)
	seq, c := strconv.ParseUint(q.Get("seq"), 10, 64)
	return turn, generation, seq, errors.Join(a, b, c)
}

func (s *room) upload(w http.ResponseWriter, r *http.Request, p *participant) {
	turn, gen, seq, err := numbers(r)
	mime := r.Header.Get("Content-Type")
	if err != nil || (mime != "video/webm;codecs=vp8,opus" && mime != "video/webm;codecs=vp8") {
		fail(w, 400, "Invalid media format or sequence.")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, chunkLimit))
	if err != nil || len(body) == 0 {
		fail(w, 413, "The media chunk is empty or too large.")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	s.tick(now)
	t := s.current
	if !s.authorized(r, p) || t == nil || t.member != p || t.ID != turn || t.Generation != gen || now.Sub(s.viewerSeen) >= offline {
		fail(w, 409, "The turn has changed.")
		return
	}
	if err := t.put(seq, mime, body); err != nil {
		s.finish("Missed", "playback fell behind", now)
		fail(w, 409, err.Error())
		return
	}
	w.WriteHeader(204)
}

func (s *room) chunk(w http.ResponseWriter, r *http.Request) {
	turn, gen, seq, err := numbers(r)
	if err != nil {
		fail(w, 400, "Invalid media sequence.")
		return
	}
	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(time.Second)
	defer timeout.Stop()
	for {
		s.mu.Lock()
		t := s.current
		if !s.validViewer(r) || t == nil || t.ID != turn || t.Generation != gen {
			s.mu.Unlock()
			fail(w, 409, "The turn has changed.")
			return
		}
		if seq < t.offset || seq > t.offset+uint64(len(t.chunks)) {
			s.mu.Unlock()
			fail(w, 409, "The media sequence has changed.")
			return
		}
		t.ack(seq)
		if len(t.chunks) > 0 {
			body, mime := t.chunks[0], t.mime
			s.mu.Unlock()
			w.Header().Set("Content-Type", mime)
			_, _ = w.Write(body)
			return
		}
		s.mu.Unlock()
		select {
		case <-ticker.C:
		case <-timeout.C:
			w.WriteHeader(204)
			return
		case <-r.Context().Done():
			return
		}
	}
}

func (s *room) clock(stop <-chan struct{}) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case now := <-tick.C:
			s.mu.Lock()
			s.tick(now)
			s.mu.Unlock()
		case <-stop:
			return
		}
	}
}

func report(err error) {
	_, _ = os.Stderr.WriteString("demo-queue: " + err.Error() + "\n")
}
