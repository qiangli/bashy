package kit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type testFrame struct {
	id    string
	frame Frame
}

// readFrames reads n frames from an SSE body, failing after wait.
func readFrames(t *testing.T, sc *bufio.Scanner, n int, wait time.Duration) []testFrame {
	t.Helper()
	got := make(chan []testFrame, 1)
	go func() {
		var out []testFrame
		var cur testFrame
		for len(out) < n && sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "id: "):
				cur.id = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "data: "):
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &cur.frame); err != nil {
					break
				}
			case line == "" && cur.id != "":
				out = append(out, cur)
				cur = testFrame{}
			}
		}
		got <- out
	}()
	select {
	case out := <-got:
		if len(out) != n {
			t.Fatalf("want %d frames, got %d: %+v", n, len(out), out)
		}
		return out
	case <-time.After(wait):
		t.Fatalf("no %d frames within %s", n, wait)
		return nil
	}
}

func openEvents(t *testing.T, url, lastID string) (*bufio.Scanner, *http.Response, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	if lastID != "" {
		req.Header.Set("Last-Event-ID", lastID)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); r.Body.Close() })
	return bufio.NewScanner(r.Body), r, cancel
}

func eventsServer(t *testing.T, e *Events, current func() int) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		e.Serve(w, r, r.URL.Query().Get("id"), current())
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEventsFreshLiveAndResume(t *testing.T) {
	e := &Events{}
	srv := eventsServer(t, e, func() int { return 7 })
	sc, r, cancel := openEvents(t, srv.URL+"?id=a", "")
	if ct := r.Header.Get("Content-Type"); ct != "text/event-stream" || r.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("headers: %v", r.Header)
	}
	base := readFrames(t, sc, 1, 2*time.Second)[0]
	if base.frame != (Frame{Item: "a", Revision: 7}) {
		t.Fatalf("fresh stream must open with a whole-item frame at the current revision, got %+v", base.frame)
	}
	e.Publish("b", "x", 1) // another item: never on a's stream
	e.Publish("a", "poll", 8)
	live := readFrames(t, sc, 1, 2*time.Second)[0]
	if live.frame != (Frame{Item: "a", Block: "poll", Revision: 8}) || live.id == base.id {
		t.Fatalf("live frame: %+v", live)
	}
	cancel()
	e.Publish("a", "timer", 9)
	e.Publish("a", "", 10)
	sc, _, _ = openEvents(t, srv.URL+"?id=a", live.id)
	resumed := readFrames(t, sc, 2, 2*time.Second)
	if resumed[0].frame != (Frame{Item: "a", Block: "timer", Revision: 9}) || resumed[1].frame != (Frame{Item: "a", Revision: 10}) {
		t.Fatalf("resume must deliver exactly the missed frames in order, got %+v", resumed)
	}
	// Resuming from the newest id replays nothing; the next publish is next.
	sc, _, _ = openEvents(t, srv.URL+"?id=a", resumed[1].id)
	e.Publish("a", "hands", 11)
	if f := readFrames(t, sc, 1, 2*time.Second)[0]; f.frame.Block != "hands" || f.frame.Resync {
		t.Fatalf("resume at head must not replay: %+v", f)
	}
}

func TestEventsResync(t *testing.T) {
	e := &Events{Keep: 2}
	srv := eventsServer(t, e, func() int { return 0 })
	first := e.Publish("a", "x", 1)
	for i := 2; i <= 5; i++ {
		e.Publish("a", "x", i)
	}
	other := &Events{}
	foreign := other.Publish("a", "x", 1) // same shape, another epoch (a restarted server)
	for name, lastID := range map[string]string{
		"behind the retained history": first,
		"another epoch":               foreign,
		"garbage":                     "nope",
		"ahead of the log":            strings.Split(first, "-")[0] + "-99",
	} {
		sc, _, _ := openEvents(t, srv.URL+"?id=a", lastID)
		f := readFrames(t, sc, 1, 2*time.Second)[0]
		if f.frame != (Frame{Item: "a", Revision: 5, Resync: true}) {
			t.Errorf("%s: want one whole-item resync frame at revision 5, got %+v", name, f.frame)
		}
	}
}

func TestEventsRefusals(t *testing.T) {
	e := &Events{MaxStreams: 1}
	srv := eventsServer(t, e, func() int { return 0 })
	for _, c := range []struct {
		method, query string
		want          int
	}{{"POST", "?id=a", 405}, {"GET", "?id=bad!!name", 400}, {"GET", "", 400}} {
		req, _ := http.NewRequest(c.method, srv.URL+c.query, nil)
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		r.Body.Close()
		if r.StatusCode != c.want {
			t.Errorf("%s %s: want %d, got %d", c.method, c.query, c.want, r.StatusCode)
		}
	}
	sc, _, cancel := openEvents(t, srv.URL+"?id=a", "")
	readFrames(t, sc, 1, 2*time.Second)
	r, err := http.Get(srv.URL + "?id=a")
	if err != nil {
		t.Fatal(err)
	}
	r.Body.Close()
	if r.StatusCode != 503 || r.Header.Get("Retry-After") == "" {
		t.Fatalf("over MaxStreams: want 503 with Retry-After, got %d", r.StatusCode)
	}
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for {
		e.mu.Lock()
		n := e.streams
		e.mu.Unlock()
		if n == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("a closed stream must release its slot, %d still open", n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A quiet stream outlives the server's WriteTimeout (desk servers set one)
// and keeps proxies awake with comments.
func TestEventsOutliveWriteTimeout(t *testing.T) {
	e := &Events{KeepAlive: 50 * time.Millisecond}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{WriteTimeout: 200 * time.Millisecond, ReadTimeout: 200 * time.Millisecond,
		Handler: ContractHandler("probe", WithSecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			e.Serve(w, r, r.URL.Query().Get("id"), 1)
		})))}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	sc, r, _ := openEvents(t, "http://"+ln.Addr().String()+"/api/events?id=a", "")
	if r.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("stream lost the security headers through ContractHandler: %v", r.Header)
	}
	readFrames(t, sc, 1, 2*time.Second)
	time.Sleep(500 * time.Millisecond)
	e.Publish("a", "late", 2)
	if f := readFrames(t, sc, 1, 2*time.Second)[0]; f.frame.Block != "late" {
		t.Fatalf("frame after the write timeout: %+v", f)
	}
}

// ContractHandler streams an event-stream answer but still envelopes the
// refusals of the same route, and buffers everything else as before.
func TestContractHandlerEventStream(t *testing.T) {
	e := &Events{}
	h := ContractHandler("probe", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("id") == "missing" {
			Fail(w, 404, "Item not found")
			return
		}
		e.Serve(w, r, r.URL.Query().Get("id"), 3)
	}))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	r, err := http.Get(srv.URL + "/api/events?id=missing")
	if err != nil {
		t.Fatal(err)
	}
	var v map[string]any
	_ = json.NewDecoder(r.Body).Decode(&v)
	r.Body.Close()
	errObj, _ := v["error"].(map[string]any)
	if r.StatusCode != 404 || v["schema_version"] != "bashy-probe-v1" || errObj["code"] != "not_found" {
		t.Fatalf("refusal not enveloped: %d %v", r.StatusCode, v)
	}
	sc, _, _ := openEvents(t, srv.URL+"/api/events?id=a", "")
	if f := readFrames(t, sc, 1, 2*time.Second)[0]; f.frame != (Frame{Item: "a", Revision: 3}) {
		t.Fatalf("stream through ContractHandler: %+v", f)
	}
}

func TestSecurityOptions(t *testing.T) {
	const legacy = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
	w := httptest.NewRecorder()
	ApplySecurityHeaders(w)
	if got := w.Header().Get("Content-Security-Policy"); got != legacy {
		t.Fatalf("default CSP changed:\n got %s\nwant %s", got, legacy)
	}
	w = httptest.NewRecorder()
	SecurityOptions{Embeddable: true}.Apply(w)
	csp := w.Header().Get("Content-Security-Policy")
	if csp != strings.Replace(legacy, "frame-ancestors 'none'", "frame-ancestors 'self'", 1) {
		t.Fatalf("embeddable CSP must differ only in frame-ancestors 'self': %s", csp)
	}
	if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("X-Content-Type-Options") != "nosniff" || w.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("embeddable must keep the other desk headers: %v", w.Header())
	}
	rec := httptest.NewRecorder()
	SecurityOptions{Embeddable: true}.Wrap(http.NotFoundHandler()).ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if !strings.Contains(rec.Header().Get("Content-Security-Policy"), "frame-ancestors 'self'") {
		t.Fatalf("Wrap: %v", rec.Header())
	}
}

func TestCheckBind(t *testing.T) {
	for _, addr := range []string{"0.0.0.0:1", ":1", "192.0.2.1:1", "[::]:1", "example.test:1"} {
		var log bytes.Buffer
		if err := CheckBind(addr, false, &log); err == nil {
			t.Errorf("%s without --lan: accepted", addr)
		}
		if log.Len() != 0 {
			t.Errorf("%s without --lan: logged %q", addr, log.String())
		}
		log.Reset()
		if err := CheckBind(addr, true, &log); err != nil {
			t.Errorf("%s with --lan: %v", addr, err)
		}
		if !strings.Contains(log.String(), "--lan") || !strings.Contains(log.String(), addr) || !strings.Contains(log.String(), "beyond loopback") {
			t.Errorf("%s with --lan: override not logged: %q", addr, log.String())
		}
	}
	var log bytes.Buffer
	if err := CheckBind("127.0.0.1:1", false, &log); err != nil || log.Len() != 0 {
		t.Errorf("loopback without --lan: %v %q", err, log.String())
	}
	if err := CheckBind("127.0.0.1:1", true, &log); err != nil || !strings.Contains(log.String(), "--lan") {
		t.Errorf("loopback with --lan: %v %q", err, log.String())
	}
	if err := CheckBind("no-port", true, &log); err == nil {
		t.Errorf("malformed address accepted with --lan")
	}
	if err := CheckBind("0.0.0.0:1", true, failWriter{}); err == nil {
		t.Errorf("an override that cannot be logged must refuse to start")
	}
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, net.ErrClosed }
