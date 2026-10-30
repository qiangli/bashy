// Command form is the desk form tool (F1): an agent defines a form in
// JSON or YAML and the human fills it on the loopback page; the agent
// reads answers back as JSON keyed by field id.
//
// Doors: `form new|open|get|put|list|share|close|wait [--json]` (agent)
// and the loopback page (human). Storage: $BASHY_HOME/desk/form/items/<id>.json.
//
// The definition schema and answer store in schema.go are the reusable
// engine shared with replyform (F2) and questionnaire (F3).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	kit "desk-kit"
)

type item struct {
	Version    int             `json:"version"`
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Body       string          `json:"body"`
	Fields     []kit.Field     `json:"fields"`
	Definition Definition      `json:"definition,omitempty"`
	Answers    Answers         `json:"answers,omitempty"`
	Status     string          `json:"status"`
	Revision   int             `json:"revision"`
	Owner      string          `json:"owner"`
	Members    map[string]bool `json:"members"`
	Invites    map[string]bool `json:"invites"`
}

type app struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	baseURL string // public origin, e.g. http://127.0.0.1:18107
	items   map[string]*item
}

func itemPath(dir, id string) string { return filepath.Join(dir, "items", id+".json") }

func (a *app) loadLocked(id string) (*item, error) {
	if !kit.ValidName(id) {
		return nil, fmt.Errorf("use 1-64 letters, digits, - or _ for an id")
	}
	if it, ok := a.items[id]; ok {
		return it, nil
	}
	raw, err := os.ReadFile(itemPath(a.dir, id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	it := &item{}
	if err := json.Unmarshal(raw, it); err != nil {
		return nil, fmt.Errorf("saved form %q is invalid; preserve it for recovery", id)
	}
	if it.Version != 1 || it.ID != id || !kit.ValidStatus(it.Status) {
		return nil, fmt.Errorf("unsupported or invalid saved form %q", id)
	}
	if err := ValidateDefinition(it.Definition); err != nil {
		return nil, fmt.Errorf("saved form %q is invalid: %v", id, err)
	}
	if it.Members == nil {
		it.Members = map[string]bool{}
	}
	if it.Invites == nil {
		it.Invites = map[string]bool{}
	}
	if it.Answers == nil {
		it.Answers = Answers{}
	}
	a.items[id] = it
	return it, nil
}

func (a *app) saveLocked(it *item) error {
	it.Revision++
	raw, err := kit.MarshalIndent(*it)
	if err != nil {
		return err
	}
	if err := kit.AtomicWrite(itemPath(a.dir, it.ID), raw); err != nil {
		return err
	}
	a.items[it.ID] = it
	return nil
}

func (a *app) isOwner(r *http.Request, it *item) bool {
	return kit.Grant(kit.FacilitatorKey(r), it.Owner)
}

func (a *app) urlFor(id string) string { return kit.ItemURL(a.baseURL+"/", "id", id) }

// openView is the contentless re-surface: id + stable URL + status only.
func (a *app) openView(it *item) map[string]any {
	return kit.OpenView(it.ID, a.urlFor(it.ID), it.Status)
}

// request is the shared action envelope. Definition and Answers ride as
// raw messages so definition errors can name the offending field instead
// of failing as a generic "Invalid request".
type request struct {
	Action     string          `json:"action"`
	ID         string          `json:"id"`
	Title      string          `json:"title"`
	Body       string          `json:"body"`
	Fields     []kit.Field     `json:"fields"`
	Definition json.RawMessage `json:"definition"`
	Answers    json.RawMessage `json:"answers"`
	Complete   bool            `json:"complete"`
	Invite     string          `json:"invite"`
}

func decodeDefinition(raw json.RawMessage) (Definition, bool, error) {
	t := strings.TrimSpace(string(raw))
	if len(raw) == 0 || t == "" || t == "null" {
		return nil, false, nil
	}
	var def Definition
	dec := json.NewDecoder(strings.NewReader(t))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&def); err != nil {
		return nil, true, fmt.Errorf("invalid definition: %v", err)
	}
	if err := ValidateDefinition(def); err != nil {
		return nil, true, err
	}
	return def, true, nil
}

func decodeAnswers(raw json.RawMessage) (Answers, bool, error) {
	t := strings.TrimSpace(string(raw))
	if len(raw) == 0 || t == "" || t == "null" {
		return nil, false, nil
	}
	var ans Answers
	if err := json.Unmarshal(raw, &ans); err != nil {
		return nil, true, fmt.Errorf("invalid answers: %v", err)
	}
	if ans == nil {
		ans = Answers{}
	}
	return ans, true, nil
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ApplySecurityHeaders(w)
	if !kit.GuardPOST(w, r, "form") {
		return
	}
	path, ok := kit.StripPrefix(a.prefix, r.URL.Path)
	if !ok {
		kit.Fail(w, 404, "Not found")
		return
	}
	if path == "/" && r.Method == "GET" {
		kit.ParticipantCookie(w, r, "form-participant")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(a.page(r))
		return
	}
	if path == "/healthz" && r.Method == "GET" {
		kit.WriteJSON(w, map[string]bool{"ok": true})
		return
	}
	if path == "/api/state" && r.Method == "GET" {
		a.mu.Lock()
		defer a.mu.Unlock()
		kit.ParticipantCookie(w, r, "form-participant")
		id := r.URL.Query().Get("id")
		it, err := a.loadLocked(id)
		if err != nil {
			if os.IsNotExist(err) {
				kit.Fail(w, 404, "Form not found. Create it with form new first.")
			} else {
				kit.Fail(w, 400, err.Error())
			}
			return
		}
		kit.WriteJSON(w, a.openView(it))
		return
	}
	if path != "/api/action" || r.Method != "POST" {
		kit.Fail(w, 404, "Not found")
		return
	}
	var req request
	if !kit.DecodeAction(w, r, 1<<20, &req) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	me := kit.ParticipantCookie(w, r, "form-participant")
	switch req.Action {
	case "new":
		a.create(w, req, me)
	case "open":
		a.open(w, req)
	case "get":
		a.get(w, r, req, me)
	case "put":
		a.mutate(w, r, req, me, false)
	case "close":
		a.mutate(w, r, req, me, true)
	case "answer":
		a.answer(w, req)
	case "list":
		a.list(w)
	case "share":
		a.share(w, r, req)
	case "join":
		a.join(w, req, me)
	default:
		kit.Fail(w, 400, "Unknown action")
	}
}

func (a *app) create(w http.ResponseWriter, req request, me string) {
	if kit.SecretRefused(req.Fields) {
		kit.Fail(w, 400, kit.ErrSecret)
		return
	}
	def, _, err := decodeDefinition(req.Definition)
	if err != nil {
		kit.Fail(w, 400, err.Error())
		return
	}
	id := req.ID
	if id == "" {
		id = "form-" + kit.NewToken()[:12]
	}
	if !kit.ValidName(id) {
		kit.Fail(w, 400, "Use 1-64 letters, digits, - or _ for an id")
		return
	}
	if _, ok := a.items[id]; ok {
		kit.Fail(w, 409, "A form with that id already exists")
		return
	}
	if _, err := os.Stat(itemPath(a.dir, id)); err == nil {
		kit.Fail(w, 409, "A form with that id already exists")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Untitled form"
	}
	it := &item{Version: 1, ID: id, Title: title, Body: req.Body,
		Fields: req.Fields, Definition: def, Answers: Answers{},
		Status: kit.StatusPending, Owner: kit.NewToken(),
		Members: map[string]bool{me: true}, Invites: map[string]bool{}}
	if err := a.saveLocked(it); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(it)
	v["ownerKey"] = it.Owner // revealed once, at creation, like a facilitator key
	kit.WriteJSON(w, v)
}

func (a *app) open(w http.ResponseWriter, req request) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	kit.WriteJSON(w, a.openView(it))
}

func (a *app) get(w http.ResponseWriter, r *http.Request, req request, me string) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) && !it.Members[me] {
		kit.Fail(w, 403, "Join this form first; ask the owner for an invite.")
		return
	}
	kit.WriteJSON(w, map[string]any{"id": it.ID, "url": a.urlFor(it.ID),
		"status": it.Status, "title": it.Title, "body": it.Body,
		"fields": it.Fields, "definition": it.Definition,
		"answers": it.Answers, "revision": it.Revision})
}

func (a *app) mutate(w http.ResponseWriter, r *http.Request, req request, me string, closing bool) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can change this form")
		return
	}
	if it.Status == kit.StatusClosed {
		kit.Fail(w, 409, "This form is closed")
		return
	}
	next := *it
	if closing {
		next.Status = kit.StatusClosed
	} else {
		if kit.SecretRefused(req.Fields) {
			kit.Fail(w, 400, kit.ErrSecret)
			return
		}
		def, hasDef, err := decodeDefinition(req.Definition)
		if err != nil {
			kit.Fail(w, 400, err.Error())
			return
		}
		ans, hasAns, err := decodeAnswers(req.Answers)
		if err != nil {
			kit.Fail(w, 400, err.Error())
			return
		}
		if req.Body != "" {
			next.Body = req.Body
		}
		if req.Fields != nil {
			next.Fields = req.Fields
		}
		if hasDef {
			next.Definition = def
		}
		if strings.TrimSpace(req.Title) != "" {
			next.Title = req.Title
		}
		checkDef := next.Definition
		if hasAns {
			norm, err := NormalizeAnswers(checkDef, ans, false)
			if err != nil {
				kit.Fail(w, 400, err.Error())
				return
			}
			next.Answers = norm
		} else if hasDef {
			// A replaced definition re-validates the stored answers so
			// stale keys cannot linger under new field ids.
			norm, err := NormalizeAnswers(def, next.Answers, false)
			if err != nil {
				kit.Fail(w, 400, fmt.Sprintf("stored answers do not fit the new definition: %v", err))
				return
			}
			next.Answers = norm
		}
		next.Status = kit.StatusFilled
	}
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(&next))
}

// answer is the human door: the loopback page posts partial or complete
// answers under the participant cookie. Every save persists, so partial
// answers survive reload; a complete save with all required fields filled
// moves the form to answered.
func (a *app) answer(w http.ResponseWriter, req request) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if it.Status == kit.StatusClosed {
		kit.Fail(w, 409, "This form is closed")
		return
	}
	ans, _, err := decodeAnswers(req.Answers)
	if err != nil {
		kit.Fail(w, 400, err.Error())
		return
	}
	if ans == nil {
		ans = Answers{}
	}
	norm, err := NormalizeAnswers(it.Definition, ans, req.Complete)
	if err != nil {
		kit.Fail(w, 400, err.Error())
		return
	}
	next := *it
	next.Answers = norm
	if req.Complete {
		next.Status = kit.StatusAnswered
	} else if next.Status != kit.StatusAnswered {
		next.Status = kit.StatusPending
	}
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(&next))
}

func (a *app) list(w http.ResponseWriter) {
	names, err := filepath.Glob(filepath.Join(a.dir, "items", "*.json"))
	if err != nil {
		kit.Fail(w, 500, "Could not list")
		return
	}
	out := []map[string]any{}
	for _, p := range names {
		id := strings.TrimSuffix(filepath.Base(p), ".json")
		it, err := a.loadLocked(id)
		if err != nil {
			continue
		}
		out = append(out, a.openView(it)) // ids + statuses, never content
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	kit.WriteJSON(w, out)
}

func (a *app) share(w http.ResponseWriter, r *http.Request, req request) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can share this form")
		return
	}
	next := *it
	if next.Invites == nil {
		next.Invites = map[string]bool{}
	}
	code := kit.NewToken()
	next.Invites[code] = true
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(&next)
	v["invite"] = code
	v["inviteURL"] = a.urlFor(next.ID) + "#invite=" + code
	kit.WriteJSON(w, v)
}

func (a *app) join(w http.ResponseWriter, req request, me string) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !kit.ValidToken(req.Invite) || !it.Invites[req.Invite] {
		kit.Fail(w, 403, "This invite is invalid. Ask the owner for a new one.")
		return
	}
	next := *it
	next.Members[me] = true
	delete(next.Invites, req.Invite) // single use
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(&next))
}

// fieldHTML renders one declared field with the kit field components: the
// recommended option of a choice/multi carries a mark, required fields are
// flagged, and saved answers are pre-filled so partial saves survive reload.
func fieldHTML(f FormField, saved any) string {
	var b strings.Builder
	required := ""
	if f.Required {
		required = ` <span aria-hidden="true">*</span>`
	}
	label := html.EscapeString(f.Label) + required
	savedStr, _ := saved.(string)
	open := `<div class="desk-field" data-field="` + html.EscapeString(f.ID) +
		`" data-kind="` + html.EscapeString(f.Kind) + `"><label>` + label + `</label>`
	help := ""
	if f.Help != "" {
		help = `<p class="desk-muted">` + html.EscapeString(f.Help) + `</p>`
	}
	req := ""
	if f.Required {
		req = " required"
	}
	switch f.Kind {
	case KindTextarea:
		b.WriteString(open + `<textarea name="` + html.EscapeString(f.ID) + `"` + req + `>` +
			html.EscapeString(savedStr) + `</textarea>` + help + `</div>`)
	case KindChoice:
		b.WriteString(open + `<div role="radiogroup">`)
		for _, opt := range f.Options {
			mark := ""
			if opt == f.Recommended {
				mark = `<span class="desk-recommended">recommended</span>`
			}
			checked := ""
			if opt == savedStr {
				checked = " checked"
			}
			b.WriteString(`<label class="desk-option"><input type="radio" name="` +
				html.EscapeString(f.ID) + `" value="` + html.EscapeString(opt) + `"` + checked + `>` +
				html.EscapeString(opt) + mark + `</label>`)
		}
		b.WriteString(`</div>` + help + `</div>`)
	case KindMulti:
		var chosen []string
		switch t := saved.(type) {
		case []string:
			chosen = t
		case []any:
			for _, e := range t {
				if s, ok := e.(string); ok {
					chosen = append(chosen, s)
				}
			}
		}
		b.WriteString(open)
		for _, opt := range f.Options {
			mark := ""
			if opt == f.Recommended {
				mark = `<span class="desk-recommended">recommended</span>`
			}
			checked := ""
			if contains(chosen, opt) {
				checked = " checked"
			}
			b.WriteString(`<label class="desk-option"><input type="checkbox" name="` +
				html.EscapeString(f.ID) + `" value="` + html.EscapeString(opt) + `"` + checked + `>` +
				html.EscapeString(opt) + mark + `</label>`)
		}
		b.WriteString(help + `</div>`)
	case KindYesNo:
		b.WriteString(open + `<div role="radiogroup">`)
		for _, opt := range []string{"yes", "no"} {
			checked := ""
			if opt == savedStr {
				checked = " checked"
			}
			b.WriteString(`<label class="desk-option"><input type="radio" name="` +
				html.EscapeString(f.ID) + `" value="` + opt + `"` + checked + `>` + opt + `</label>`)
		}
		b.WriteString(`</div>` + help + `</div>`)
	case KindFile:
		current := ""
		switch t := saved.(type) {
		case string:
			if t != "" {
				current = `<p class="desk-muted" data-current-file="` + html.EscapeString(t) + `">Currently: ` + html.EscapeString(t) + `</p>`
			}
		case map[string]any:
			if name, _ := t["name"].(string); name != "" {
				current = `<p class="desk-muted" data-current-file="` + html.EscapeString(name) + `">Currently: ` + html.EscapeString(name) + `</p>`
			}
		}
		b.WriteString(open + current + `<input type="file" name="` + html.EscapeString(f.ID) + `">` + help + `</div>`)
	default: // text
		b.WriteString(open + `<input type="text" name="` + html.EscapeString(f.ID) + `" value="` +
			html.EscapeString(savedStr) + `"` + req + `>` + help + `</div>`)
	}
	return b.String()
}

// page renders the human door: item header, status badges, copy buttons,
// the kit field components and a theme toggle, all from the kit snippets.
func (a *app) page(r *http.Request) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := r.URL.Query().Get("id")
	header := `<div class="desk-item-header"><span class="desk-id">form</span>` +
		kit.Badge(kit.StatusPending) + `<span class="desk-tools"></span></div>`
	formBody := `<div class="desk-field"><p class="desk-muted">Open a form link (form open ID) to fill it here.</p></div>`
	if id != "" {
		if it, err := a.loadLocked(id); err == nil {
			header = kit.ItemHeader(it.ID, it.Status, a.urlFor(it.ID))
			var fb strings.Builder
			for _, f := range it.Definition {
				fb.WriteString(fieldHTML(f, it.Answers[f.ID]))
			}
			if len(it.Definition) == 0 {
				fb.WriteString(`<div class="desk-field"><p class="desk-muted">This form has no fields yet.</p></div>`)
			}
			formBody = `<form id="desk-form" data-form-id="` + html.EscapeString(it.ID) + `">` + fb.String() +
				`<div style="display:flex;gap:8px;flex-wrap:wrap">` +
				`<button type="button" class="desk-copy" data-desk-save>Save (partial)</button>` +
				`<button type="button" class="desk-copy" data-desk-submit>Submit answers</button></div></form>`
		} else {
			formBody = `<div data-desk-alert class="desk-alert">Form not found. Create it with form new first.</div>`
		}
	}
	var badges strings.Builder
	for _, s := range []string{kit.StatusPending, kit.StatusNeeds, kit.StatusFilled,
		kit.StatusAnswered, kit.StatusFailed, kit.StatusClosed} {
		badges.WriteString(kit.Badge(s))
	}
	body := `<main style="max-width:720px;margin:0 auto;padding:20px">` + header +
		`<div class="desk-tools" style="margin-bottom:12px">` + badges.String() + `</div>` +
		`<div data-desk-alert class="desk-alert"></div>` +
		formBody +
		`<div style="display:flex;gap:8px;margin-top:12px;flex-wrap:wrap">` +
		kit.CopyButton("Copy link", a.baseURL+"/") +
		`<button type="button" class="desk-copy" data-desk-theme>Toggle theme</button></div>` +
		`<p class="desk-muted">Local-first form · loopback only · partial saves persist · re-surface with <code>form open ID</code>.</p></main>` +
		`<script>` + formPageJS + `</script>`
	return kit.Shell("Form · bashy desk", body)
}

// formPageJS collects answers from the kit field components and posts
// partial (Save) or complete (Submit) saves. File answers send name+size;
// bytes never leave the machine through this page.
const formPageJS = `(function () {
  'use strict';
  function currentForm() { return document.getElementById('desk-form'); }
  function collect() {
    var form = currentForm();
    if (!form) return null;
    var answers = {};
    form.querySelectorAll('[data-field]').forEach(function (wrap) {
      var id = wrap.getAttribute('data-field');
      var kind = wrap.getAttribute('data-kind');
      if (kind === 'multi') {
        var vals = [];
        wrap.querySelectorAll('input[type=checkbox]:checked').forEach(function (el) { vals.push(el.value); });
        answers[id] = vals;
      } else if (kind === 'choice' || kind === 'yes-no') {
        var picked = wrap.querySelector('input[type=radio]:checked');
        answers[id] = picked ? picked.value : '';
      } else if (kind === 'file') {
        var input = wrap.querySelector('input[type=file]');
        if (input && input.files && input.files.length > 0) {
          answers[id] = { name: input.files[0].name, size: input.files[0].size };
        } else {
          var cur = wrap.querySelector('[data-current-file]');
          answers[id] = cur ? cur.getAttribute('data-current-file') : '';
        }
      } else {
        var field = wrap.querySelector('input,textarea,select');
        answers[id] = field ? field.value : '';
      }
    });
    return { id: form.getAttribute('data-form-id'), answers: answers };
  }
  async function send(complete) {
    var msg = document.querySelector('[data-desk-alert]');
    var payload = collect();
    if (!payload) return;
    payload.action = 'answer';
    payload.complete = !!complete;
    try {
      var r = await fetch('api/action', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      var body = await r.json();
      if (!r.ok) {
        if (msg) msg.textContent = (body && body.error) || 'Could not save.';
        return;
      }
      if (msg) msg.textContent = '';
      var badge = document.querySelector('.desk-item-header .desk-badge');
      if (badge && body.status) {
        badge.setAttribute('data-status', body.status);
        badge.textContent = body.status;
      }
    } catch (e) {
      if (msg) msg.textContent = 'Could not save. The server may be down; your answers are still on this page.';
    }
  }
  document.addEventListener('click', function (ev) {
    if (ev.target.closest('[data-desk-save]')) send(false);
    if (ev.target.closest('[data-desk-submit]')) send(true);
  });
})();`

func serve(addr, data, prefix string) {
	if err := kit.CheckLoopback(addr); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(data, "items"), 0700); err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	origin := "http://" + net.JoinHostPort("127.0.0.1", port)
	a := &app{dir: data, prefix: prefix, baseURL: origin + prefix, items: map[string]*item{}}
	fmt.Printf("\nDesk form — keep this terminal running\nParticipant: %s/\nSaved forms: %s\nCtrl-C stops the server.\n\n",
		a.baseURL, filepath.Join(data, "items"))
	srv := &http.Server{Handler: a, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sig
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()
	if err = srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout))
}
