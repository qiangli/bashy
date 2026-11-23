package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	kit "desk-kit"
	"page/blocks"
)

// TimerState is one timer block's durable state, stored on the page item.
type TimerState struct {
	Name      string `json:"name"`
	BlockID   string `json:"block_id"`
	Duration  int    `json:"duration"`
	Remaining int    `json:"remaining"`
	Running   bool   `json:"running,omitempty"`
	Finished  bool   `json:"finished,omitempty"`
	EndsAt    int64  `json:"ends_at,omitempty"` // Unix milliseconds, while running
}

func siblingBase(env string, def string) string {
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
		return nil, fmt.Errorf("not available (status %d)", resp.StatusCode)
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return nil, fmt.Errorf("invalid response")
	}
	// Desk ContractHandler envelopes: unwrap a nested result object if present.
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

// validBookRef reports whether ref is <id> or <id>/pN with a valid id.
func validBookRef(ref string) bool {
	id, _ := parseBookRef(ref)
	return kit.ValidName(id)
}

func (a *app) renderBookBlock(b *block) string {
	id, pageSel := parseBookRef(b.Ref)
	obj, err := siblingShow(viewerBase(), id, pageSel)
	if err != nil {
		out, _ := blocks.Book(blocks.BookData{ID: b.ID, Err: "Book " + b.Ref + " is " + err.Error() + "."})
		return out
	}
	// Viewer show returns either "page" (single page) or "items" (pages).
	var md, pageTitle, title string
	title, _ = obj["title"].(string)
	if pg, ok := obj["page"].(map[string]any); ok {
		md, _ = pg["markdown"].(string)
		pageTitle, _ = pg["title"].(string)
	} else if items, ok := obj["items"].([]any); ok && len(items) > 0 {
		last, _ := items[len(items)-1].(map[string]any)
		md, _ = last["markdown"].(string)
		pageTitle, _ = last["title"].(string)
	}
	rendered, rerr := renderMarkdown([]byte(md))
	if rerr != nil {
		out, _ := blocks.Book(blocks.BookData{ID: b.ID, Err: "This book page cannot be shown."})
		return out
	}
	out, _ := blocks.Book(blocks.BookData{ID: b.ID, Title: title, PageTitle: pageTitle, Body: template.HTML(rendered)})
	return out
}

func (a *app) renderNoteBlock(b *block) string {
	obj, err := siblingShow(notepadBase(), b.Ref, "")
	if err != nil {
		out, _ := blocks.Note(blocks.NoteData{ID: b.ID, Err: "Note " + b.Ref + " is " + err.Error() + "."})
		return out
	}
	title, _ := obj["title"].(string)
	md, _ := obj["body"].(string)
	rendered, rerr := renderMarkdown([]byte(md))
	if rerr != nil {
		out, _ := blocks.Note(blocks.NoteData{ID: b.ID, Err: "This note cannot be shown."})
		return out
	}
	out, _ := blocks.Note(blocks.NoteData{ID: b.ID, Title: title, Body: template.HTML(rendered)})
	return out
}

func (a *app) renderEmbedBlock(b *block) string {
	title := b.Options["title"]
	if title == "" {
		title = b.Ref
	}
	out, _ := blocks.Embed(blocks.EmbedData{ID: b.ID, Src: b.Ref, Title: title})
	return out
}

// timerDuration reads the duration option (Go duration or bare seconds).
func timerDuration(b *block) int {
	raw := b.Options["duration"]
	if raw == "" {
		raw = b.Options["seconds"]
	}
	if raw == "" {
		return 300
	}
	if d, err := time.ParseDuration(raw); err == nil && d > 0 {
		return int(d.Seconds())
	}
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return 300
}

// ensureTimerLocked returns the timer state for b, creating it if needed.
// Caller holds a.mu.
func (a *app) ensureTimerLocked(it *item, b *block) *TimerState {
	if it.Timers == nil {
		it.Timers = map[string]*TimerState{}
	}
	ts, ok := it.Timers[b.ID]
	if !ok {
		d := timerDuration(b)
		ts = &TimerState{Name: b.Ref, BlockID: b.ID, Duration: d, Remaining: d}
		it.Timers[b.ID] = ts
	}
	return ts
}

func timerRemaining(ts *TimerState, now time.Time) int {
	if ts.Running && ts.EndsAt > 0 {
		rem := int((ts.EndsAt - now.UnixMilli() + 999) / 1000)
		if rem < 0 {
			rem = 0
		}
		return rem
	}
	return ts.Remaining
}

func (a *app) renderTimerBlock(b *block, it *item, owner bool) string {
	// Rendering never mutates the stored item: an unseen timer shows its
	// default duration, and timerAction persists state on first control.
	var cp TimerState
	if it.Timers != nil && it.Timers[b.ID] != nil {
		cp = *it.Timers[b.ID]
	} else {
		d := timerDuration(b)
		cp = TimerState{Name: b.Ref, BlockID: b.ID, Duration: d, Remaining: d}
	}
	rem := timerRemaining(&cp, time.Now())
	state := "Stopped"
	if cp.Running {
		state = "Running"
	} else if cp.Finished {
		state = "Finished"
	}
	out, _ := blocks.Timer(blocks.TimerData{ID: b.ID, Name: cp.Name, Display: blocks.FormatClock(rem),
		State: state, Remaining: rem, Running: cp.Running, EndsAt: cp.EndsAt, Organizer: owner})
	return out
}

// renderBlock dispatches to the renderer for b's kind. Kinds owned by other
// stories keep the skeleton placeholder.
func (a *app) renderBlock(b *block, it *item, owner bool) string {
	switch b.Kind {
	case "book":
		return a.renderBookBlock(b)
	case "note":
		return a.renderNoteBlock(b)
	case "embed":
		return a.renderEmbedBlock(b)
	case "timer":
		return a.renderTimerBlock(b, it, owner)
	default:
		return blockHTML(b)
	}
}

func (a *app) renderPageAs(it *item, owner bool) (string, error) {
	segs, err := parseBody(it.Body)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, s := range segs {
		if s.Block != nil {
			sb.WriteString(a.renderBlock(s.Block, it, owner))
			continue
		}
		out, err := renderMarkdown([]byte(s.Text))
		if err != nil {
			return "", err
		}
		sb.Write(out)
	}
	return sb.String(), nil
}

// blockFragment serves GET /api/block?id=<page>&block=<blockID>: the one
// block's current HTML, so the live stream re-fetches only what changed.
func (a *app) blockFragment(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	it := a.loadOr404(w, r.URL.Query().Get("id"))
	if it == nil || !a.viewOr403(w, r, it) {
		a.mu.Unlock()
		return
	}
	owner := a.isOwner(r, it)
	blockID := r.URL.Query().Get("block")
	var found *block
	for _, bl := range blocksOf(it.Body) {
		c := bl
		if c.ID == blockID {
			found = &c
			break
		}
	}
	a.mu.Unlock()
	if found == nil {
		kit.Fail(w, http.StatusNotFound, "Block not found")
		return
	}
	// Timer creation persists under the lock inside renderTimerBlock.
	html := a.renderBlock(found, it, owner)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, html)
}

// timerAction handles facilitator start/stop/reset for a timer block.
// Only the organizer may control a timer; everyone may view it.
func (a *app) timerAction(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
	if !a.isOwner(r, it) {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can control the timer")
		return
	}
	op := req.Kind
	blockID := req.Block
	if blockID == "" {
		blockID = req.Source
	}
	var found *block
	for _, bl := range blocksOf(it.Body) {
		c := bl
		if c.ID == blockID && c.Kind == "timer" {
			found = &c
			break
		}
	}
	if found == nil {
		kit.Fail(w, http.StatusNotFound, "Timer block not found")
		return
	}
	next := it.clone()
	ts := a.ensureTimerLocked(&next, found)
	now := time.Now()
	switch op {
	case "start":
		if !ts.Running {
			rem := timerRemaining(ts, now)
			if rem <= 0 {
				rem = ts.Duration
			}
			ts.Remaining = rem
			ts.EndsAt = now.UnixMilli() + int64(rem)*1000
			ts.Running = true
			ts.Finished = false
		}
	case "stop":
		if ts.Running {
			ts.Remaining = timerRemaining(ts, now)
			ts.Running = false
			ts.EndsAt = 0
		}
	case "reset":
		ts.Running = false
		ts.Finished = false
		ts.EndsAt = 0
		ts.Remaining = ts.Duration
	default:
		kit.Fail(w, http.StatusBadRequest, "Timer action must be start, stop or reset")
		return
	}
	if err := a.saveLocked(&next, found.ID); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	if op == "start" {
		// The action runs under a.mu; the ticker takes the lock itself.
		go a.startTimerTicker(next.ID, found.ID)
	}
	kit.WriteJSON(w, a.openView(r, &next))
}

// startTimerTicker publishes one SSE frame per second while the timer runs
// and marks it finished at zero. Safe to call more than once per block.
func (a *app) startTimerTicker(itemID, blockID string) {
	a.mu.Lock()
	if a.ticking == nil {
		a.ticking = map[string]bool{}
	}
	key := itemID + "/" + blockID
	if a.ticking[key] {
		a.mu.Unlock()
		return
	}
	a.ticking[key] = true
	a.mu.Unlock()
	go func() {
		defer func() {
			a.mu.Lock()
			delete(a.ticking, key)
			a.mu.Unlock()
		}()
		for {
			time.Sleep(time.Second)
			a.mu.Lock()
			it, err := a.loadLocked(itemID)
			if err != nil || it.Timers == nil || it.Timers[blockID] == nil {
				a.mu.Unlock()
				return
			}
			ts := it.Timers[blockID]
			if !ts.Running {
				a.mu.Unlock()
				return
			}
			if timerRemaining(ts, time.Now()) <= 0 {
				next := it.clone()
				nts := next.Timers[blockID]
				nts.Running = false
				nts.Finished = true
				nts.Remaining = 0
				nts.EndsAt = 0
				_ = a.saveLocked(&next, blockID)
				a.mu.Unlock()
				return
			}
			rev := it.Revision
			a.mu.Unlock()
			a.events.Publish(itemID, blockID, rev)
		}
	}()
}

// timerScriptFor wires timer buttons and the block re-fetch on live frames.
func timerScriptFor(pageID string) string {
	idJSON, _ := json.Marshal(pageID)
	return `<script>
(function(){
  var pageID = ` + string(idJSON) + `;
  document.addEventListener('click', function(e){
    var btn = e.target.closest('[data-timer-action]');
    if (!btn) return;
    fetch('api/action', {method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({action:'timer', id:pageID, kind:btn.getAttribute('data-timer-action'), block:btn.getAttribute('data-block')})
    }).then(function(){ location.reload(); });
  });
  document.addEventListener('page-block', function(e){
    var f = e.detail; if (!f || !f.block) return;
    fetch('api/block?id=' + encodeURIComponent(pageID) + '&block=' + encodeURIComponent(f.block))
      .then(function(r){ return r.text(); })
      .then(function(html){
        var el = document.querySelector('[data-block="' + f.block + '"]');
        if (el) { var t = document.createElement('template'); t.innerHTML = html.trim(); el.replaceWith(t.content.firstChild); }
      });
  });
  setInterval(function(){
    document.querySelectorAll('[data-timer-clock]').forEach(function(el){
      var sec = el.closest('[data-block]');
      if (!sec || sec.getAttribute('data-running') !== 'true') return;
      var ends = Number(sec.getAttribute('data-ends-at'));
      var rem = Math.max(0, Math.ceil((ends - Date.now())/1000));
      var h = Math.floor(rem/3600), m = Math.floor(rem%3600/60), s = rem%60;
      el.textContent = h > 0 ? h + ':' + String(m).padStart(2,'0') + ':' + String(s).padStart(2,'0') : m + ':' + String(s).padStart(2,'0');
    });
  }, 250);
})();
</script>`
}

// blockMarkdownFor is the record line for a block, inlining final state
// for the kinds this story owns.
func blockMarkdownFor(b *block, it *item) string {
	switch b.Kind {
	case "timer":
		if it != nil && it.Timers != nil {
			if ts := it.Timers[b.ID]; ts != nil {
				rem := timerRemaining(ts, time.Now())
				state := "stopped"
				if ts.Running {
					state = "running"
				} else if ts.Finished {
					state = "finished"
				}
				return "> **timer** `" + b.ID + "` " + blocks.FormatClock(rem) + " (" + state + ")\n"
			}
		}
	case "embed":
		return "> **embed** `" + b.ID + "` `" + b.Ref + "`\n"
	case "book", "note":
		return "> **" + b.Kind + "** `" + b.ID + "` `" + b.Ref + "`\n"
	}
	return blockMarkdown(b)
}
