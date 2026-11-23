// Content blocks: book:<id>/pN and note:<id> render another desk app's
// Markdown inline, embed:<path|url> frames a same-origin kit app (or links
// out to an engine that refuses framing), and timer:<name> is a
// facilitator-controlled countdown every face sees tick over SSE. Each
// kind's markup is one template under blocks/.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	kit "desk-kit"
	"page/blocks"
)

// TimerState is one timer block's durable state, stored on the page item.
type TimerState struct {
	Duration  int   `json:"duration"`
	Remaining int   `json:"remaining"`
	Running   bool  `json:"running,omitempty"`
	Finished  bool  `json:"finished,omitempty"`
	EndsAt    int64 `json:"ends_at,omitempty"` // Unix milliseconds, while running
}

func siblingBase(env, def string) string {
	v := strings.TrimSpace(os.Getenv(env))
	if v == "" {
		v = def
	}
	if strings.HasPrefix(v, "http://") || strings.HasPrefix(v, "https://") {
		return strings.TrimRight(v, "/")
	}
	return "http://" + v
}

func viewerBase() string  { return siblingBase("VIEWER_ADDR", "127.0.0.1:18096") }
func notepadBase() string { return siblingBase("NOTEPAD_ADDR", "127.0.0.1:18095") }

var siblingClient = &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil}}

// siblingShow POSTs a show action to a sibling desk app and decodes the
// (possibly enveloped) JSON object.
func siblingShow(base, id, pageSel string) (map[string]any, error) {
	payload := map[string]any{"action": "show", "id": id}
	if pageSel != "" {
		payload["page"] = pageSel
	}
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, base+"/api/action", bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := siblingClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("not reachable")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("not available")
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("not readable")
	}
	for _, key := range []string{"result", "data", "item"} {
		if inner, ok := obj[key].(map[string]any); ok {
			return inner, nil
		}
	}
	return obj, nil
}

var bookPageRe = regexp.MustCompile(`^(.*)/p([1-9][0-9]*)$`)

// parseBookRef splits book:<id>/pN into the book id and the page selector.
func parseBookRef(ref string) (id, pageSel string) {
	if m := bookPageRe.FindStringSubmatch(ref); m != nil {
		return m[1], "p" + m[2]
	}
	return ref, ""
}

func validBookRef(ref string) bool {
	id, _ := parseBookRef(ref)
	return kit.ValidName(id)
}

// contentHTML renders a book page (viewer) or a note (notepad) inline.
// An unreachable app or missing item degrades to a note, never an error page.
func contentHTML(b *block) string {
	render := blocks.Note
	base, id, pageSel, what := notepadBase(), b.Ref, "", "Note "+b.Ref
	if b.Kind == "book" {
		render = blocks.Book
		id, pageSel = parseBookRef(b.Ref)
		base, what = viewerBase(), "Book "+b.Ref
	}
	d := blocks.ContentData{Ref: b.Ref}
	obj, err := siblingShow(base, id, pageSel)
	if err != nil {
		d.Err = what + " is " + err.Error() + "."
		return render(d)
	}
	d.Title, _ = obj["title"].(string)
	md, _ := obj["body"].(string)
	if pg, ok := obj["page"].(map[string]any); ok {
		md, _ = pg["markdown"].(string)
		d.PageTitle, _ = pg["title"].(string)
	} else if items, ok := obj["items"].([]any); ok && len(items) > 0 {
		first, _ := items[0].(map[string]any)
		md, _ = first["markdown"].(string)
		d.PageTitle, _ = first["title"].(string)
	}
	out, err := renderMarkdown([]byte(md))
	if err != nil {
		d.Err = what + " cannot be shown."
		return render(d)
	}
	d.Body = template.HTML(out)
	return render(d)
}

// linkOnlyEngines are same-origin engines that refuse framing (not built on
// desk-kit's Embeddable opt-in): an embed of one renders a link-out card.
var linkOnlyEngines = []string{"whiteboard", "table", "klatch", "demo-queue"}

func engineRefusesFraming(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return true
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	first := segs[0]
	if first == "apps" && len(segs) > 1 {
		first = segs[1]
	}
	return slices.Contains(linkOnlyEngines, first)
}

// timerName is the timer's label: its ref, else its block id.
func timerName(b *block) string {
	if b.Options["title"] != "" {
		return b.Options["title"]
	}
	if b.Ref != "" {
		return b.Ref
	}
	return b.ID
}

// timerDuration reads the duration option (Go duration or bare seconds).
func timerDuration(b *block) int {
	raw := b.Options["duration"]
	if raw == "" {
		raw = b.Options["seconds"]
	}
	if d, err := time.ParseDuration(raw); err == nil && d >= time.Second {
		return int(d.Seconds())
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return 300
}

// timerFor is b's state without mutating the item: an untouched timer
// shows its full duration.
func timerFor(it *item, b *block) TimerState {
	if ts := it.Timers[b.ID]; ts != nil {
		return *ts
	}
	d := timerDuration(b)
	return TimerState{Duration: d, Remaining: d}
}

func timerRemaining(ts TimerState, now time.Time) int {
	if ts.Running {
		return max(0, int((ts.EndsAt-now.UnixMilli()+999)/1000))
	}
	return ts.Remaining
}

func timerStateWord(ts TimerState, rem int) string {
	switch {
	case ts.Finished || ts.Running && rem == 0:
		return "Finished"
	case ts.Running:
		return "Running"
	case rem < ts.Duration:
		return "Paused"
	}
	return "Ready"
}

// timerHTML is one face of a timer: controls on the organizer face only,
// the bare clock on the display face.
func (a *app) timerHTML(it *item, b *block, face pageFace) string {
	ts := timerFor(it, b)
	if ts.Running {
		a.startTimerTicker(it.ID, b.ID) // resume ticks after a restart
	}
	rem := timerRemaining(ts, time.Now())
	return blocks.Timer(blocks.TimerData{ID: b.ID, Name: timerName(b), Display: blocks.FormatClock(rem),
		State: timerStateWord(ts, rem), Running: ts.Running && rem > 0, EndsAt: ts.EndsAt,
		Controls: face == organizerFace, Projected: face == displayFace})
}

// timerMarkdown is the timer's place in the record: its state at export.
func timerMarkdown(it *item, b *block) string {
	ts := timerFor(it, b)
	rem := timerRemaining(ts, time.Now())
	return "> **timer** `" + b.ID + "` " + timerName(b) + ": " + blocks.FormatClock(rem) +
		" (" + strings.ToLower(timerStateWord(ts, rem)) + ")\n"
}

// timerAction is the organizer's start/stop/reset; everyone sees the result.
func (a *app) timerAction(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
	if faceFor(r, a.canOrganize(r, it)) != organizerFace {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can control a timer")
		return
	}
	var found *block
	for _, b := range blocksOf(it.Body) {
		if b.ID == req.Block && b.Kind == "timer" {
			found = &b
			break
		}
	}
	if found == nil {
		kit.Fail(w, http.StatusNotFound, "No timer block with that id on this page")
		return
	}
	next := it.clone()
	ts := timerFor(&next, found)
	now := time.Now()
	rem := timerRemaining(ts, now)
	switch req.Kind {
	case "start":
		if !ts.Running || rem == 0 {
			if rem == 0 {
				rem = ts.Duration
			}
			ts = TimerState{Duration: ts.Duration, Remaining: rem, Running: true, EndsAt: now.UnixMilli() + int64(rem)*1000}
		}
	case "stop":
		ts = TimerState{Duration: ts.Duration, Remaining: rem, Finished: rem == 0}
	case "reset":
		ts = TimerState{Duration: ts.Duration, Remaining: ts.Duration}
	default:
		kit.Fail(w, http.StatusBadRequest, "Use start, stop or reset")
		return
	}
	if next.Timers == nil {
		next.Timers = map[string]*TimerState{}
	}
	next.Timers[found.ID] = &ts
	if err := a.saveLocked(&next, found.ID); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	if ts.Running {
		a.startTimerTicker(next.ID, found.ID)
	}
	v := a.openView(r, &next)
	v["block"], v["running"], v["remaining"] = found.ID, ts.Running, ts.Remaining
	kit.WriteJSON(w, v)
}

// startTimerTicker publishes one block frame per second while the timer
// runs and records it finished at zero. Idempotent per block; the caller
// may hold a.mu (the goroutine takes it itself).
func (a *app) startTimerTicker(itemID, blockID string) {
	key := itemID + "/" + blockID
	a.tickMu.Lock()
	if a.ticking == nil {
		a.ticking = map[string]bool{}
	}
	if a.ticking[key] {
		a.tickMu.Unlock()
		return
	}
	a.ticking[key] = true
	a.tickMu.Unlock()
	go func() {
		defer func() {
			a.tickMu.Lock()
			delete(a.ticking, key)
			a.tickMu.Unlock()
		}()
		for {
			time.Sleep(time.Second)
			a.mu.Lock()
			it, err := a.loadLocked(itemID)
			if err != nil || it.Timers[blockID] == nil || !it.Timers[blockID].Running {
				a.mu.Unlock()
				return
			}
			ts := *it.Timers[blockID]
			if timerRemaining(ts, time.Now()) > 0 {
				a.events.Publish(itemID, blockID, it.Revision)
				a.mu.Unlock()
				continue
			}
			next := it.clone()
			next.Timers[blockID] = &TimerState{Duration: ts.Duration, Finished: true}
			_ = a.saveLocked(&next, blockID)
			a.mu.Unlock()
			return
		}
	}()
}

// timerScript counts running clocks down between ticks on every face and
// wires the organizer's controls.
func timerScript(base, id string, face pageFace) string {
	cfg, _ := json.Marshal(map[string]string{"action": base + "/api/action?view=" + string(face), "id": id})
	return `<script>
(function(){
  const cfg = ` + string(cfg) + `;
  function clock(rem){
    const h = Math.floor(rem/3600), m = Math.floor(rem%3600/60), s = String(rem%60).padStart(2,'0');
    return h > 0 ? h + ':' + String(m).padStart(2,'0') + ':' + s : m + ':' + s;
  }
  setInterval(function(){
    document.querySelectorAll('[data-timer][data-running=true]').forEach(function(t){
      const el = t.querySelector('[data-timer-clock]');
      if (el) el.textContent = clock(Math.max(0, Math.ceil((Number(t.dataset.endsAt) - Date.now())/1000)));
    });
  }, 250);
  document.addEventListener('click', async function(e){
    const button = e.target.closest('[data-timer-action]');
    if (!button) return;
    const block = button.closest('[data-block]').dataset.block;
    button.disabled = true;
    try {
      const r = await fetch(cfg.action, {method: 'POST', credentials: 'same-origin', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({action: 'timer', id: cfg.id, block: block, kind: button.dataset.timerAction})});
      if (!r.ok) throw new Error();
      document.dispatchEvent(new CustomEvent('page-block', {detail: {block}}));
    } catch (_) {
      if (window.deskToast) window.deskToast('Could not update the timer.', 'error');
    } finally {
      button.disabled = false;
    }
  });
})();
</script>`
}

const contentBlockStyle = `<style>
.page-timer-clock{font-size:var(--desk-text-lg);font-weight:var(--desk-weight-strong);font-variant-numeric:tabular-nums;margin:0}
.page-display .page-timer-clock{font-size:6em}
.page-content-body{border-top:1px solid var(--desk-line);padding-top:var(--desk-space-2)}
</style>`
