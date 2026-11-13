// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// fakeRunner records each invocation; when the argv names an output file
// after "-o"/the last .gif/.mp4-looking path, it creates it so the worker
// sees the artifact appear. FailWith makes matching tool calls fail.
type fakeRunner struct {
	mu    sync.Mutex
	calls [][]string
	fail  string
}

func (f *fakeRunner) Run(_ context.Context, tool string, args ...string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	call := append([]string{tool}, args...)
	f.calls = append(f.calls, call)
	if f.fail == tool {
		return tool + " log", errors.New(tool + " exited 3")
	}
	// Create the output file: the last argument that ends in an artifact
	// extension, or the value after -o.
	out := ""
	for _, a := range args {
		if strings.HasSuffix(a, ".mp4") || strings.HasSuffix(a, ".gif") {
			out = a
		}
	}
	if out != "" {
		if err := os.WriteFile(out, []byte(tool+" output"), 0o600); err != nil {
			return "", err
		}
	}
	return tool + " log", nil
}

func (f *fakeRunner) args(n int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[n]
}

func mustQueue(t *testing.T, s *Store, j *Job) *Job {
	t.Helper()
	q, err := s.Queue(j)
	if err != nil {
		t.Fatalf("queue %s: %v", j.Kind, err)
	}
	return q
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func touchVideo(t *testing.T, dir, name string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("fake video"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// The store round-trips jobs, lists them newest first and refuses ids it
// did not mint.
func TestStoreRoundTripAndOrder(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Queue(&Job{Kind: KindScreen, Seconds: 5}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Queue(&Job{Kind: KindScreen, Seconds: 6}); err != nil {
		t.Fatal(err)
	}
	jobs, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Fatalf("listed %d jobs, want 2", len(jobs))
	}
	if jobs[0].Created < jobs[1].Created {
		t.Fatal("list is not newest first")
	}
	if _, err := s.Load("../evil"); err == nil {
		t.Fatal("load accepted a path traversal id")
	}
	if _, err := s.Load("c1234"); err == nil {
		t.Fatal("load accepted a non-hex id")
	}
	if _, err := s.Load("c12345678"); !strings.Contains(err.Error(), "no job") {
		t.Fatalf("missing job error = %v", err)
	}
}

// Queue rejects bad specs loudly: unknown kind, missing or non-file
// source, wrong extensions, backwards spans, oversized spans, bad names.
func TestQueueValidation(t *testing.T) {
	dir := t.TempDir()
	vid := touchVideo(t, dir, "take.mp4")
	scene := touchVideo(t, dir, "scene.cast")
	cases := []struct {
		name string
		job  Job
		want string
	}{
		{"unknown kind", Job{Kind: "rip"}, "kind is trim"},
		{"relative source", Job{Kind: KindTrim, Source: "take.mp4", Start: 1, End: 2}, "absolute path"},
		{"missing source", Job{Kind: KindTrim, Source: filepath.Join(dir, "nope.mp4"), Start: 1, End: 2}, "source"},
		{"bad ext", Job{Kind: KindTrim, Source: touchVideo(t, dir, "x.txt"), Start: 1, End: 2}, "trim needs a video"},
		{"cast ext", Job{Kind: KindCast, Source: vid, Start: 0, End: 1}, "asciicast"},
		{"backwards span", Job{Kind: KindTrim, Source: vid, Start: 9, End: 2}, "greater than"},
		{"oversize span", Job{Kind: KindTrim, Source: vid, Start: 0, End: MaxSpanSeconds + 1}, "at most"},
		{"bad name", Job{Kind: KindScreen, Seconds: 5, Name: "a/b"}, "name may use"},
		{"long name", Job{Kind: KindScreen, Seconds: 5, Name: strings.Repeat("x", 81)}, "80 characters"},
		{"screen seconds", Job{Kind: KindScreen, Seconds: 0}, "--seconds"},
		{"screen size", Job{Kind: KindScreen, Seconds: 5, Size: "big"}, "--size"},
		{"bad format", Job{Kind: KindCast, Source: scene, Start: 0, End: 1, Format: "avi"}, "--format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestStore(t)
			_, err := s.Queue(&tc.job)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want it to mention %q", err, tc.want)
			}
		})
	}
	// The valid forms pass, and cast defaults to gif.
	s := newTestStore(t)
	j := mustQueue(t, s, &Job{Kind: KindCast, Source: scene, Start: 0, End: 2})
	if j.Format != "gif" || j.Status != StatusQueued || !ValidID(j.ID) {
		t.Fatalf("queued cast job = %+v", j)
	}
}

// The worker renders a queued trim job with ffmpeg, marks it done and
// records the artifact; a failed tool marks the job failed with the reason.
func TestWorkTrimDoneAndFailed(t *testing.T) {
	s := newTestStore(t)
	vid := touchVideo(t, t.TempDir(), "take.mp4")
	j := mustQueue(t, s, &Job{Kind: KindTrim, Source: vid, Start: 2.5, End: 7})
	f := &fakeRunner{}
	n, err := Work(context.Background(), s, f, false)
	if err != nil || n != 1 {
		t.Fatalf("work: n=%d err=%v", n, err)
	}
	done, err := s.Load(j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if done.Status != StatusDone || done.Artifact != "clips/"+j.ID+".mp4" {
		t.Fatalf("job after work = %+v", done)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "clips", j.ID+".mp4")); err != nil {
		t.Fatalf("artifact missing: %v", err)
	}
	argv := f.args(0)
	joined := strings.Join(argv, " ")
	for _, want := range []string{"ffmpeg", "-ss 2.5", "-to 7", vid} {
		if !strings.Contains(joined, want) {
			t.Fatalf("ffmpeg argv %q lacks %q", joined, want)
		}
	}

	// A failing tool fails only its own job.
	j2 := mustQueue(t, s, &Job{Kind: KindTrim, Source: vid, Start: 0, End: 1})
	f2 := &fakeRunner{fail: "ffmpeg"}
	if _, err := Work(context.Background(), s, f2, true); err != nil {
		t.Fatal(err)
	}
	failed, _ := s.Load(j2.ID)
	if failed.Status != StatusFailed || !strings.Contains(failed.Error, "exited 3") {
		t.Fatalf("failed job = %+v", failed)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "clips", j2.ID+".mp4")); !os.IsNotExist(err) {
		t.Fatal("failed trim left an artifact behind")
	}
}

// Cast scenes render with agg to a gif, or to mp4 through both tools when
// asked; the intermediate gif does not stay behind.
func TestWorkCastFormats(t *testing.T) {
	s := newTestStore(t)
	scene := touchVideo(t, t.TempDir(), "scene.cast")
	gif := mustQueue(t, s, &Job{Kind: KindCast, Source: scene, Start: 0, End: 1})
	mp4 := mustQueue(t, s, &Job{Kind: KindCast, Source: scene, Start: 0, End: 1, Format: "mp4"})
	f := &fakeRunner{}
	if _, err := Work(context.Background(), s, f, false); err != nil {
		t.Fatal(err)
	}
	g, _ := s.Load(gif.ID)
	m, _ := s.Load(mp4.ID)
	if g.Status != StatusDone || !strings.HasSuffix(g.Artifact, ".gif") {
		t.Fatalf("gif job = %+v", g)
	}
	if m.Status != StatusDone || !strings.HasSuffix(m.Artifact, ".mp4") {
		t.Fatalf("mp4 job = %+v", m)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "clips", mp4.ID+".gif")); !os.IsNotExist(err) {
		t.Fatal("the intermediate gif survived an mp4 render")
	}
	if f.args(0)[0] != "agg" || f.args(1)[0] != "agg" || f.args(2)[0] != "ffmpeg" {
		t.Fatalf("tools called in the wrong order: %v %v %v", f.args(0), f.args(1), f.args(2))
	}
}

// Screen jobs are an X11 job: on Linux the worker calls ffmpeg with
// x11grab; elsewhere the job fails with the platform reason instead of
// hanging.
func TestWorkScreen(t *testing.T) {
	s := newTestStore(t)
	j := mustQueue(t, s, &Job{Kind: KindScreen, Seconds: 4, Display: ":1.0", Size: "800x600"})
	f := &fakeRunner{}
	if _, err := Work(context.Background(), s, f, true); err != nil {
		t.Fatal(err)
	}
	got, _ := s.Load(j.ID)
	var flat []string
	for _, call := range f.callsIfAny() {
		flat = append(flat, strings.Join(call, " "))
	}
	joined := strings.Join(flat, " ")
	if runtime.GOOS == "linux" {
		if got.Status != StatusDone {
			t.Fatalf("screen job on linux = %+v", got)
		}
		for _, want := range []string{"x11grab", "800x600", ":1.0", "-t 4"} {
			if !strings.Contains(joined, want) {
				t.Fatalf("x11grab argv %q lacks %q", joined, want)
			}
		}
	} else {
		if got.Status != StatusFailed || !strings.Contains(got.Error, "Linux") {
			t.Fatalf("screen job off-linux = %+v", got)
		}
		if joined != "" {
			t.Fatalf("a tool ran off-linux: %q", joined)
		}
	}
}

// A job left running by a dead worker returns to the queue instead of
// blocking it forever.
func TestWorkRequeuesStaleRunning(t *testing.T) {
	s := newTestStore(t)
	j := mustQueue(t, s, &Job{Kind: KindScreen, Seconds: 2})
	j.Status = StatusRunning
	if err := s.Save(j); err != nil {
		t.Fatal(err)
	}
	// Not stale yet: nothing is processed.
	if n, err := Work(context.Background(), s, &fakeRunner{}, false); err != nil || n != 0 {
		t.Fatalf("fresh running job was touched: n=%d err=%v", n, err)
	}
	// Age it past the stale window by writing the record directly: Save
	// always stamps Updated, so a stale timestamp cannot go through it.
	old := *j
	old.Status = StatusRunning
	old.Updated = "2020-01-01T00:00:00.000000000Z"
	raw, err := json.Marshal(&old)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.jobPathForTest(j.ID), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if n, err := Work(context.Background(), s, &fakeRunner{}, true); err != nil || n != 1 {
		t.Fatalf("stale job not requeued: n=%d err=%v", n, err)
	}
	got, _ := s.Load(j.ID)
	if runtime.GOOS == "linux" && got.Status != StatusDone {
		t.Fatalf("requeued job = %+v", got)
	}
}

// The page serves health, the library table, the queue API and finished
// clips, and refuses paths that leave the clips directory.
func TestServePage(t *testing.T) {
	s := newTestStore(t)
	vid := touchVideo(t, t.TempDir(), "take.mp4")
	done := mustQueue(t, s, &Job{Kind: KindTrim, Source: vid, Start: 1, End: 2, Name: "Demo walk"})
	if err := os.MkdirAll(filepath.Join(s.Dir(), "clips"), 0o700); err != nil {
		t.Fatal(err)
	}
	clip := filepath.Join(s.Dir(), "clips", done.ID+".mp4")
	if err := os.WriteFile(clip, []byte("clip bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	done.Status, done.Artifact = StatusDone, "clips/"+done.ID+".mp4"
	if err := s.Save(done); err != nil {
		t.Fatal(err)
	}
	h := Serve(s)

	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)
	get := func(path string) *http.Response {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Result()
	}
	follow := func(path string) *http.Response {
		t.Helper()
		resp, err := ts.Client().Get(ts.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		return resp
	}

	if r := get("/healthz"); r.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", r.StatusCode)
	}
	page := get("/")
	if page.StatusCode != http.StatusOK {
		t.Fatalf("page = %d", page.StatusCode)
	}
	body := pageText(t, page)
	for _, want := range []string{"Clips", "Demo walk", done.ID, "done", "/media/" + done.ID + ".mp4"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page lacks %q", want)
		}
	}

	// The media route serves the artifact and refuses traversal: the mux
	// may clean a dirty path to a redirect, but no byte of the store is
	// ever served that way.
	if r := get("/media/" + done.ID + ".mp4"); r.StatusCode != http.StatusOK {
		t.Fatalf("media = %d", r.StatusCode)
	}
	if r := get("/media/nope.mp4"); r.StatusCode != http.StatusNotFound {
		t.Fatalf("missing media = %d, want 404", r.StatusCode)
	}
	// Traversal is refused: following whatever clean redirect the standard
	// mux issues still ends outside the clips directory, at 404.
	for _, bad := range []string{"/media/../jobs/" + done.ID + ".json", "/media/..%2fjobs"} {
		resp := follow(bad)
		body := pageText(t, resp)
		if resp.StatusCode != http.StatusNotFound && resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("media %q = %d: traversal must be refused", bad, resp.StatusCode)
		}
		if strings.Contains(body, done.ID) {
			t.Fatalf("media %q leaked the store record", bad)
		}
	}

	// Queueing over the API writes the same job file the CLI would.
	rec := httptest.NewRecorder()
	body2 := fmt.Sprintf(`{"kind":"screen","seconds":3,"name":"Page queued"}`)
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(body2)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/jobs = %d: %s", rec.Code, rec.Body.String())
	}
	posted := &Job{}
	if err := json.Unmarshal(rec.Body.Bytes(), posted); err != nil {
		t.Fatal(err)
	}
	if posted.Status != StatusQueued || posted.Name != "Page queued" {
		t.Fatalf("posted job = %+v", posted)
	}
	if _, err := s.Load(posted.ID); err != nil {
		t.Fatalf("posted job did not land in the store: %v", err)
	}

	// Invalid queue requests are refused, never queued.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(`{"kind":"trim","source":"/nonexistent/x.mp4","start":1,"end":2}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid POST = %d, want 400", rec.Code)
	}
}

func pageText(t *testing.T, r *http.Response) string {
	t.Helper()
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (f *fakeRunner) callsIfAny() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// jobPathForTest reaches the store's layout for fixtures Save cannot
// express (a stale Updated stamp).
func (s *Store) jobPathForTest(id string) string {
	return filepath.Join(s.Dir(), "jobs", id+".json")
}

// The console builds the server with a fixed command (GOWORK=off,
// CGO_ENABLED=0, go build ./server in the app root) and starts it with the
// manifest's argv; shaped.json must keep matching that contract.
func TestShapedManifestMatchesServer(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "shaped.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Server *struct {
			Main string   `json:"main"`
			Args []string `json:"args"`
		} `json:"server"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m.Server == nil {
		t.Fatal("shaped.json declares no server")
	}
	if m.Server.Main != "server" {
		t.Fatalf("server.main = %q, want server", m.Server.Main)
	}
	joined := strings.Join(m.Server.Args, " ")
	if !strings.Contains(joined, "$ADDR") {
		t.Fatal("server.args must pass $ADDR")
	}
	if !strings.Contains(joined, "$DATA") {
		t.Fatal("server.args must pass $DATA")
	}
	// The argv the manifest names must be argv run() understands.
	if !strings.HasPrefix(joined, "serve -addr") {
		t.Fatalf("server.args = %q", joined)
	}
}

// The listener the serve verb opens answers the health probe: the same
// contract the console's launcher waits for.
func TestServeListenHealthz(t *testing.T) {
	s := newTestStore(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	srv := &http.Server{Handler: Serve(s)}
	ln, err = net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() { _ = srv.Close() })
	resp, err := http.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}
}
