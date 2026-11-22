package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestReadyOrderDeadlineAndRecall(t *testing.T) {
	s := newSession()
	now := time.Now()
	alex, sam := s.invites[s.issue()], s.invites[s.issue()]
	s.join(alex, "Alex", now)
	s.join(sam, "Sam", now)
	if err := s.ready(alex, now); err == nil {
		t.Fatal("ready without a probe")
	}
	sam.probed, alex.probed = now, now
	_ = s.ready(sam, now)
	_ = s.ready(alex, now)
	s.started, s.viewer, s.viewerSeen = true, "viewer", now
	s.advance(now)
	if s.current.member != sam || !s.current.Deadline.IsZero() {
		t.Fatal("prepare order or initial deadline is wrong")
	}
	s.assigned = alex
	s.advance(now)
	if s.current.member != alex || len(s.queue) != 1 || s.queue[0] != sam {
		t.Fatal("override did not requeue interrupted presenter")
	}
	s.current.Deadline = now.Add(time.Second)
	s.tick(now.Add(time.Second))
	if s.current.member != sam || alex.Status != "Finished" {
		t.Fatal("deadline did not hand off")
	}
	s.tick(now.Add(offline + time.Second))
	if s.current != nil || sam.Status != "Offline" {
		t.Fatal("disconnected presenter kept the turn")
	}
}

func TestMediaBoundaryAndProjectorReconnect(t *testing.T) {
	s := &room{session: newSession(), owner: secret(), display: secret()}
	local := s.local("local.test")
	mesh := s.mesh()
	hit := func(h http.Handler, method, path, token, lease, typ string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, "http://local.test"+path, bytes.NewReader(body))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("X-Demo-Lease", lease)
		r.Header.Set("X-Demo-Viewer", s.viewer)
		r.Header.Set("Content-Type", typ)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, code int) {
		t.Helper()
		if w.Code != code {
			t.Fatalf("got %d want %d: %s", w.Code, code, w.Body)
		}
	}
	expect(hit(mesh, "POST", "/api/join", "bad", "", "", []byte(`{"name":"Alex"}`)), 403)
	expect(hit(mesh, "POST", "/api/control", s.owner, "", "", []byte(`{"action":"start"}`)), 404)
	key := s.issue()
	joinBody := []byte(`{"name":"Alex"}`)
	join := func() string {
		w := hit(mesh, "POST", "/api/join", key, "", "", joinBody)
		expect(w, 200)
		var data map[string]string
		_ = json.Unmarshal(w.Body.Bytes(), &data)
		return data["lease"]
	}
	old := join()
	lease := join()
	if len(s.roster) != 1 {
		t.Fatal("duplicate roster entry")
	}
	expect(hit(mesh, "POST", "/api/prepare", key, old, "", make([]byte, probeSize)), 401)
	expect(hit(mesh, "POST", "/api/prepare", key, lease, "", nil), 400)
	expect(hit(mesh, "POST", "/api/prepare", key, lease, "", make([]byte, probeSize)), 204)
	expect(hit(mesh, "POST", "/api/member", key, lease, "", []byte(`{"action":"ready"}`)), 204)
	path := "?turn=1&generation=1&seq=0"
	mime := "video/webm;codecs=vp8,opus"
	expect(hit(mesh, "POST", "/api/upload"+path, key, lease, mime, []byte("chunk")), 409)
	expect(hit(local, "POST", "/api/viewer", s.display, "", "", nil), 200)
	expect(hit(local, "POST", "/api/control", s.display, "", "", []byte(`{"action":"start"}`)), 403)
	expect(hit(local, "POST", "/api/control", s.owner, "", "", []byte(`{"action":"start","limit":2,"warning":1}`)), 204)
	expect(hit(mesh, "POST", "/api/upload"+path, key, lease, mime, make([]byte, chunkLimit+1)), 413)
	expect(hit(mesh, "POST", "/api/upload"+path, key, lease, mime, []byte("chunk")), 204)
	if !s.current.Deadline.IsZero() {
		t.Fatal("upload began the countdown")
	}
	w := hit(local, "GET", "/api/chunk"+path, s.display, "", "", nil)
	expect(w, 200)
	if w.Body.String() != "chunk" || w.Header().Get("Content-Type") != mime {
		t.Fatal("incorrect projector media")
	}
	expect(hit(local, "POST", "/api/rendered", s.display, "", "", []byte(`{"turn":1,"generation":1}`)), 204)
	deadline := s.current.Deadline
	expect(hit(local, "POST", "/api/viewer", s.display, "", "", nil), 200)
	if s.current.Generation != 2 || !s.current.Deadline.Equal(deadline) || len(s.current.chunks) != 0 {
		t.Fatal("reconnect did not isolate media and preserve deadline")
	}
	expect(hit(mesh, "POST", "/api/upload"+path, key, lease, mime, []byte("stale")), 409)
	expect(hit(local, "POST", "/api/control", s.owner, "", "", []byte(`{"action":"end"}`)), 204)
	if s.current != nil || !s.ended {
		t.Fatal("end did not release media")
	}
}

func TestEventsKeepViewerAlive(t *testing.T) {
	s := &room{session: newSession(), display: secret()}
	s.viewer = "viewer"
	s.viewerSeen = time.Now().Add(-offline)
	ln, err := openLocal()
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	server, _ := serve(ln, s.local(addr))
	defer server.Close()
	res, err := http.Get("http://" + addr + "/api/events?viewer=viewer&key=" + s.display)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	first := make([]byte, 200)
	n, err := io.ReadAtLeast(res.Body, first, 25)
	if err != nil || !strings.Contains(string(first[:n]), `"projector":true`) {
		t.Fatalf("failed to renew projector presence: %q, %v", first[:n], err)
	}
}

func TestPendingAssignmentAndStartupTimeout(t *testing.T) {
	s := newSession()
	now := time.Now()
	alex, sam := s.invites[s.issue()], s.invites[s.issue()]
	s.join(alex, "Alex", now)
	s.join(sam, "Sam", now)
	s.viewer, s.viewerSeen, s.started = "viewer", now, true
	alex.probed = now
	if err := s.ready(alex, now); err != nil {
		t.Fatal(err)
	}
	s.assigned = sam
	s.advance(now)
	if s.current.member != alex || s.assigned != sam {
		t.Fatal("waiting for preparation interrupted the current turn")
	}
	step := now.Add(time.Second)
	sam.probed = step
	if err := s.ready(sam, step); err != nil {
		t.Fatal(err)
	}
	if s.current.member != sam || s.assigned != nil {
		t.Fatal("prepared assignment did not get priority")
	}
	later := step.Add(startup)
	alex.seen, sam.seen, s.viewerSeen = later, later, later
	s.tick(later)
	if s.current.member != alex || sam.Status != "Missed" {
		t.Fatal("no-show did not hand off to the interrupted presenter")
	}
}

func TestShapedRelayAndDryRun(t *testing.T) {
	s := &room{session: newSession(), owner: secret(), display: secret(), peer: "peer", service: "test"}
	native := httptest.NewUnstartedServer(nil)
	native.Config.Handler = s.local(native.Listener.Addr().String())
	native.Start()
	defer native.Close()

	shaped := httptest.NewServer(shapedHandler())
	defer shaped.Close()
	host, err := fromLink(native.URL + "/control#" + s.owner)
	if err != nil {
		t.Fatal(err)
	}
	host.via = strings.TrimPrefix(shaped.URL, "http://")
	ctx := t.Context()
	if err := host.validate(ctx); err != nil {
		t.Fatal(err)
	}
	res, err := host.post(ctx, "control", action{Action: "invite"}, true)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(s.invites) != 0 {
		t.Fatal("dry-run issued an invite")
	}
	res, err = host.post(ctx, "control", action{Action: "invite"}, false)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if len(s.invites) != 1 {
		t.Fatal("relay did not mutate native room")
	}

	guest := host
	guest.role, guest.key = "projector", s.display
	if _, err := guest.post(ctx, "control", action{Action: "end"}, true); err == nil {
		t.Fatal("projector reached organizer action")
	}
	if s.ended {
		t.Fatal("denied request mutated state")
	}
	guest.addr = "outside.example:443"
	if err := guest.validate(ctx); err == nil {
		t.Fatal("relay accepted non-loopback destination")
	}
}
