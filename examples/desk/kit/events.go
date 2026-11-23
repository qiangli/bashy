package kit

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Frame is one live-layer notice: item changed, in block, now at revision.
// It never carries content: the page re-fetches only the named block (or
// the whole item when Block is "") through the tool's own API, so a stream
// needs no more access than GET /api/state. Revisions, never merges.
type Frame struct {
	Item     string `json:"item"`
	Block    string `json:"block"`
	Revision int    `json:"revision"`
	// Resync marks a whole-item frame sent because the stream could not
	// continue from the client's Last-Event-ID (unknown id, a restarted
	// server, or a client that fell behind the retained history).
	Resync bool `json:"resync,omitempty"`
}

// Events is the desk live layer: one Server-Sent Events stream per item,
// GET /api/events?id=ITEM, carrying Frames. Pattern from meet
// (yoke/pkg/meet/events_sse.go) and demo-queue: every frame has an `id:`,
// so a browser's EventSource reconnects with Last-Event-ID and resumes with
// no gap and no replay; a position the history can no longer continue is
// answered with one Resync frame at the current revision before delivery
// goes on. A fresh stream (no Last-Event-ID) opens with one whole-item frame
// at the current revision, so a client that fetched state just before
// subscribing can tell whether it missed a change.
//
// The zero value is ready to use. Tools validate the item id and access
// first (400 bad id, 404 unknown item) and then hand the request to Serve;
// they call Publish after every acknowledged save.
type Events struct {
	// Keep is how many frames per item are retained for resume (256).
	Keep int
	// KeepAlive is the idle comment interval that keeps proxies from
	// reaping a quiet stream (15s).
	KeepAlive time.Duration
	// MaxStreams caps concurrent streams across all items (256); beyond it
	// a stream is refused with 503 and Retry-After.
	MaxStreams int

	mu      sync.Mutex
	epoch   string
	items   map[string]*eventLog
	streams int
}

type eventLog struct {
	seq    int64
	rev    int
	frames []seqFrame // oldest first, at most Keep
	subs   map[chan struct{}]struct{}
}

type seqFrame struct {
	seq   int64
	frame Frame
}

// sseWriteWait bounds one frame write; the deadline is cleared again after
// each write so a long, quiet stream outlives the server's WriteTimeout.
const sseWriteWait = 10 * time.Second

func (e *Events) keep() int {
	if e.Keep > 0 {
		return e.Keep
	}
	return 256
}

func (e *Events) keepAlive() time.Duration {
	if e.KeepAlive > 0 {
		return e.KeepAlive
	}
	return 15 * time.Second
}

func (e *Events) maxStreams() int {
	if e.MaxStreams > 0 {
		return e.MaxStreams
	}
	return 256
}

func (e *Events) logLocked(item string) *eventLog {
	if e.items == nil {
		e.items = map[string]*eventLog{}
		// The epoch names this process's history: an id minted by an
		// earlier run (or another server) never silently resumes here.
		e.epoch = NewToken()[:12]
	}
	l, ok := e.items[item]
	if !ok {
		l = &eventLog{subs: map[chan struct{}]struct{}{}}
		e.items[item] = l
	}
	return l
}

func (e *Events) idLocked(seq int64) string { return e.epoch + "-" + strconv.FormatInt(seq, 10) }

// Publish records that item changed in block (empty: the whole item) and is
// now at revision, wakes every open stream of that item, and returns the
// frame's event id. It never blocks on a slow client.
func (e *Events) Publish(item, block string, revision int) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	l := e.logLocked(item)
	l.seq++
	if revision > l.rev {
		l.rev = revision
	}
	l.frames = append(l.frames, seqFrame{l.seq, Frame{Item: item, Block: block, Revision: revision}})
	if over := len(l.frames) - e.keep(); over > 0 {
		l.frames = l.frames[over:]
	}
	for ch := range l.subs {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
	return e.idLocked(l.seq)
}

// framesAfterLocked returns what a stream positioned at after must receive
// next: the retained frames with a larger seq, or one Resync frame when the
// history no longer reaches back to after+1.
func (e *Events) framesAfterLocked(l *eventLog, item string, after int64) []seqFrame {
	if after >= l.seq {
		return nil
	}
	if after < 0 || len(l.frames) == 0 || l.frames[0].seq > after+1 {
		return []seqFrame{{l.seq, Frame{Item: item, Revision: l.rev, Resync: true}}}
	}
	var out []seqFrame
	for _, f := range l.frames {
		if f.seq > after {
			out = append(out, f)
		}
	}
	return out
}

// startLocked positions a new stream from its Last-Event-ID.
func (e *Events) startLocked(l *eventLog, item, lastID string) []seqFrame {
	lastID = strings.TrimSpace(lastID)
	if lastID == "" {
		return []seqFrame{{l.seq, Frame{Item: item, Revision: l.rev}}}
	}
	epoch, rawSeq, ok := strings.Cut(lastID, "-")
	seq, err := strconv.ParseInt(rawSeq, 10, 64)
	if !ok || err != nil || epoch != e.epoch || seq < 0 || seq > l.seq {
		return []seqFrame{{l.seq, Frame{Item: item, Revision: l.rev, Resync: true}}}
	}
	return e.framesAfterLocked(l, item, seq)
}

// Serve streams item's frames to one client until it goes away. current is
// the item's revision as the tool knows it now (its saved Revision), so a
// restarted server still opens with the right baseline. Serve answers 405
// for anything but GET, 400 for an invalid id and 503 over MaxStreams; the
// tool has already answered 404 for an unknown item.
func (e *Events) Serve(w http.ResponseWriter, r *http.Request, item string, current int) {
	if r.Method != http.MethodGet {
		Fail(w, http.StatusMethodNotAllowed, "Method not supported")
		return
	}
	if !ValidName(item) {
		Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for an id")
		return
	}
	e.mu.Lock()
	if e.streams >= e.maxStreams() {
		e.mu.Unlock()
		w.Header().Set("Retry-After", "5")
		Fail(w, http.StatusServiceUnavailable, "Too many live streams; retry shortly")
		return
	}
	l := e.logLocked(item)
	if current > l.rev {
		l.rev = current
	}
	wake := make(chan struct{}, 1)
	l.subs[wake] = struct{}{}
	e.streams++
	pending := e.startLocked(l, item, r.Header.Get("Last-Event-ID"))
	after := l.seq
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(l.subs, wake)
		e.streams--
		e.mu.Unlock()
	}()

	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Time{})
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	write := func(prefix string, frames []seqFrame) bool {
		var b strings.Builder
		b.WriteString(prefix)
		for _, f := range frames {
			data, _ := json.Marshal(f.frame)
			fmt.Fprintf(&b, "id: %s\ndata: %s\n\n", e.idFor(f.seq), data)
		}
		_ = rc.SetWriteDeadline(time.Now().Add(sseWriteWait))
		_, err := w.Write([]byte(b.String()))
		if err == nil {
			err = rc.Flush()
		}
		_ = rc.SetWriteDeadline(time.Time{})
		return err == nil
	}
	if !write("retry: 2000\n\n", pending) {
		return
	}
	tick := time.NewTicker(e.keepAlive())
	defer tick.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-tick.C:
			if !write(": keepalive\n\n", nil) {
				return
			}
		case <-wake:
			e.mu.Lock()
			next := e.framesAfterLocked(l, item, after)
			after = l.seq
			e.mu.Unlock()
			if len(next) > 0 && !write("", next) {
				return
			}
		}
	}
}

func (e *Events) idFor(seq int64) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.idLocked(seq)
}
