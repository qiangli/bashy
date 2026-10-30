// Command notepad is the bashy desk notepad tool: view any content as text
// (txt, md, html, json, PDF->text), edit Markdown in the loopback page,
// agent reads/puts content, lists and shares notes.
//
// Doors: `notepad add|open|show|set|list|share|close [--json]` (agent) and the
// loopback page (human). Storage: $BASHY_HOME/desk/notepad/items/<id>.json.
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

// breakMode is set from -break; empty means contract-clean.
var breakMode string

type note struct {
	Version  int             `json:"version"`
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
	Fields   []kit.Field     `json:"fields,omitempty"`
	Status   string          `json:"status"`
	Revision int             `json:"revision"`
	Owner    string          `json:"owner"`
	Members  map[string]bool `json:"members"`
	Invites  map[string]bool `json:"invites"`
}

type actionRequest struct {
	Action   string      `json:"action"`
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Body     string      `json:"body"`
	Fields   []kit.Field `json:"fields,omitempty"`
	Invite   string      `json:"invite,omitempty"`
	Format   string      `json:"format,omitempty"`
	Filename string      `json:"filename,omitempty"`
	To       string      `json:"to,omitempty"`
	Output   string      `json:"output,omitempty"`
}

type app struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	baseURL string // public origin, e.g. http://127.0.0.1:18095
	notes   map[string]*note
}

func notePath(dir, id string) string { return filepath.Join(dir, "items", id+".json") }

func (a *app) loadLocked(id string) (*note, error) {
	if !kit.ValidName(id) {
		return nil, fmt.Errorf("use 1-64 letters, digits, - or _ for an id")
	}
	if n, ok := a.notes[id]; ok {
		return n, nil
	}
	raw, err := os.ReadFile(notePath(a.dir, id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, err
	}
	n := &note{}
	if err := json.Unmarshal(raw, n); err != nil {
		return nil, fmt.Errorf("saved note %q is invalid; preserve it for recovery", id)
	}
	if n.Version != 1 || n.ID != id || !kit.ValidStatus(n.Status) {
		return nil, fmt.Errorf("unsupported or invalid saved note %q", id)
	}
	if n.Members == nil {
		n.Members = map[string]bool{}
	}
	if n.Invites == nil {
		n.Invites = map[string]bool{}
	}
	a.notes[id] = n
	return n, nil
}

func (a *app) saveLocked(n *note) error {
	n.Revision++
	raw, err := kit.MarshalIndent(*n)
	if err != nil {
		return err
	}
	if err := kit.AtomicWrite(notePath(a.dir, n.ID), raw); err != nil {
		return err
	}
	a.notes[n.ID] = n
	return nil
}

func (a *app) isOwner(r *http.Request, n *note) bool {
	return kit.Grant(kit.FacilitatorKey(r), n.Owner)
}

func (a *app) hasReadAccess(r *http.Request, n *note, me string) bool {
	if a.isOwner(r, n) || n.Members[me] {
		return true
	}
	// Local caller without cross-site metadata: treat as member if on loopback
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(remoteHost).IsLoopback() && r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") == "" {
		return true
	}
	return false
}

func (a *app) hasWriteAccess(r *http.Request, n *note, me string) bool {
	if a.isOwner(r, n) || n.Members[me] {
		return true
	}
	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && net.ParseIP(remoteHost).IsLoopback() && r.Header.Get("Origin") == "" && r.Header.Get("Sec-Fetch-Site") == "" {
		return true
	}
	return false
}

func (a *app) urlFor(id string) string { return kit.ItemURL(a.baseURL+"/", "id", id) }

// openView is the contentless re-surface: id + stable URL + status only.
func (a *app) openView(n *note) map[string]any {
	v := kit.OpenView(n.ID, a.urlFor(n.ID), n.Status)
	if breakMode == "open-emits-content" {
		v["body"] = n.Body // deliberate contract break for red/green proof
	}
	return v
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ContractHandler("notepad", http.HandlerFunc(a.serveHTTP)).ServeHTTP(w, r)
}

func (a *app) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if breakMode != "no-csp" {
		kit.ApplySecurityHeaders(w)
	}
	if !kit.GuardPOST(w, r, "notepad") {
		return
	}
	path, ok := kit.StripPrefix(a.prefix, r.URL.Path)
	if !ok {
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}
	if path == "/" && r.Method == "GET" {
		kit.ParticipantCookie(w, r, "notepad-participant")
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
		kit.ParticipantCookie(w, r, "notepad-participant")
		id := r.URL.Query().Get("id")
		n, err := a.loadLocked(id)
		if err != nil {
			if os.IsNotExist(err) {
				kit.Fail(w, http.StatusNotFound, "Note not found. Create it with notepad add first.")
			} else {
				kit.Fail(w, http.StatusBadRequest, err.Error())
			}
			return
		}
		kit.WriteJSON(w, a.openView(n))
		return
	}
	if path == "/api/export" {
		a.mu.Lock()
		defer a.mu.Unlock()
		me := kit.ParticipantCookie(w, r, "notepad-participant")
		var req actionRequest
		if r.Method == "GET" {
			req = actionRequest{
				Action: "export",
				ID:     r.URL.Query().Get("id"),
				To:     r.URL.Query().Get("to"),
				Format: r.URL.Query().Get("format"),
				Output: r.URL.Query().Get("output"),
			}
			if req.Output == "" {
				req.Output = r.URL.Query().Get("o")
			}
		} else if r.Method == "POST" {
			if !kit.DecodeAction(w, r, 10<<20, &req) {
				return
			}
			req.Action = "export"
		} else {
			kit.Fail(w, http.StatusMethodNotAllowed, "Method not supported")
			return
		}
		a.export(w, r, req, me)
		return
	}
	if path == "/api/action" && r.Method != "POST" {
		kit.Fail(w, http.StatusMethodNotAllowed, "Method not supported")
		return
	}
	if path != "/api/action" {
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}

	var req actionRequest
	if !kit.DecodeAction(w, r, 10<<20, &req) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	me := kit.ParticipantCookie(w, r, "notepad-participant")
	switch req.Action {
	case "add":
		a.create(w, req, me)
	case "open":
		a.open(w, req)
	case "show":
		a.get(w, r, req, me)
	case "set":
		a.mutate(w, r, req, me, false)
	case "close":
		a.mutate(w, r, req, me, true)
	case "list":
		a.list(w)
	case "share":
		a.share(w, r, req)
	case "join":
		a.join(w, req, me)
	case "convert":
		a.convert(w, req)
	case "export":
		a.export(w, r, req, me)
	default:
		kit.Fail(w, http.StatusBadRequest, "Unknown action")
	}
}

func (a *app) create(w http.ResponseWriter, req actionRequest, me string) {
	if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
		kit.Fail(w, http.StatusBadRequest, kit.ErrSecret)
		return
	}
	id := req.ID
	if id == "" {
		id = "note-" + kit.NewToken()[:12]
	}
	if !kit.ValidName(id) {
		kit.Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for an id")
		return
	}
	if _, ok := a.notes[id]; ok {
		kit.Fail(w, http.StatusConflict, "A note with that id already exists")
		return
	}
	if _, err := os.Stat(notePath(a.dir, id)); err == nil {
		kit.Fail(w, http.StatusConflict, "A note with that id already exists")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Untitled note"
	}
	body := req.Body
	if req.Format != "" || req.Filename != "" {
		conv, _, err := ConvertContent([]byte(body), req.Format+req.Filename)
		if err == nil {
			body = conv
		}
	}
	status := kit.StatusPending
	if strings.TrimSpace(body) != "" {
		status = kit.StatusFilled
	}
	n := &note{
		Version: 1,
		ID:      id,
		Title:   title,
		Body:    body,
		Fields:  req.Fields,
		Status:  status,
		Owner:   kit.NewToken(),
		Members: map[string]bool{me: true},
		Invites: map[string]bool{},
	}
	if err := a.saveLocked(n); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(n)
	v["ownerKey"] = n.Owner // revealed once, at creation
	kit.WriteJSON(w, v)
}

func (a *app) open(w http.ResponseWriter, req actionRequest) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, http.StatusNotFound, "Note not found. Create it with notepad add first.")
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	kit.WriteJSON(w, a.openView(n))
}

func (a *app) get(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, http.StatusNotFound, "Note not found. Create it with notepad add first.")
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if !a.hasReadAccess(r, n, me) {
		kit.Fail(w, http.StatusForbidden, "Join this note first; ask the owner for an invite.")
		return
	}
	kit.WriteJSON(w, map[string]any{
		"id":       n.ID,
		"url":      a.urlFor(n.ID),
		"status":   n.Status,
		"title":    n.Title,
		"body":     n.Body,
		"fields":   n.Fields,
		"revision": n.Revision,
	})
}

func (a *app) mutate(w http.ResponseWriter, r *http.Request, req actionRequest, me string, closing bool) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, http.StatusNotFound, "Note not found. Create it with notepad add first.")
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if !a.hasWriteAccess(r, n, me) {
		kit.Fail(w, http.StatusForbidden, "Only members or the owner can change this note")
		return
	}
	if n.Status == kit.StatusClosed {
		kit.Fail(w, http.StatusConflict, "This note is closed")
		return
	}
	next := *n
	if closing {
		next.Status = kit.StatusClosed
	} else {
		if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
			kit.Fail(w, http.StatusBadRequest, kit.ErrSecret)
			return
		}
		if req.Body != "" {
			body := req.Body
			if req.Format != "" || req.Filename != "" {
				conv, _, err := ConvertContent([]byte(body), req.Format+req.Filename)
				if err == nil {
					body = conv
				}
			}
			next.Body = body
		}
		if req.Fields != nil {
			next.Fields = req.Fields
		}
		if strings.TrimSpace(req.Title) != "" {
			next.Title = req.Title
		}
		next.Status = kit.StatusFilled
	}
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(&next))
}

func (a *app) list(w http.ResponseWriter) {
	names, err := filepath.Glob(filepath.Join(a.dir, "items", "*.json"))
	if err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not list")
		return
	}
	out := []map[string]any{}
	for _, p := range names {
		id := strings.TrimSuffix(filepath.Base(p), ".json")
		n, err := a.loadLocked(id)
		if err != nil {
			continue
		}
		out = append(out, a.openView(n)) // ids + statuses, never content
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	kit.WriteJSON(w, out)
}

func (a *app) share(w http.ResponseWriter, r *http.Request, req actionRequest) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, http.StatusNotFound, "Note not found. Create it with notepad add first.")
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if !a.isOwner(r, n) {
		kit.Fail(w, http.StatusForbidden, "Only the owner can share this note")
		return
	}
	next := *n
	if next.Invites == nil {
		next.Invites = map[string]bool{}
	}
	code := kit.NewToken()
	next.Invites[code] = true
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(&next)
	v["invite"] = code
	v["inviteURL"] = a.urlFor(next.ID) + "#invite=" + code
	kit.WriteJSON(w, v)
}

func (a *app) join(w http.ResponseWriter, req actionRequest, me string) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, http.StatusNotFound, "Note not found. Create it with notepad add first.")
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if !kit.ValidToken(req.Invite) || !n.Invites[req.Invite] {
		kit.Fail(w, http.StatusForbidden, "This invite is invalid. Ask the owner for a new one.")
		return
	}
	next := *n
	next.Members[me] = true
	delete(next.Invites, req.Invite) // single use
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(&next))
}

func (a *app) convert(w http.ResponseWriter, req actionRequest) {
	conv, format, err := ConvertContent([]byte(req.Body), req.Format+req.Filename)
	if err != nil {
		kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("Conversion failed: %v", err))
		return
	}
	kit.WriteJSON(w, map[string]any{"text": conv, "format": format})
}

func (a *app) export(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	if req.ID == "" {
		kit.Fail(w, http.StatusBadRequest, "Note id required for export")
		return
	}
	n, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, http.StatusNotFound, "Note not found. Create it with notepad add first.")
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	if !a.hasReadAccess(r, n, me) {
		kit.Fail(w, http.StatusForbidden, "Join this note first; ask the owner for an invite.")
		return
	}
	toFormat := req.To
	if toFormat == "" {
		toFormat = req.Format
	}
	if toFormat == "" {
		kit.Fail(w, http.StatusBadRequest, "Specify export format with --to pdf|png|html|txt")
		return
	}
	data, err := ExportContent(n.Title, n.Body, toFormat)
	if err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	normFormat := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(toFormat), "."))
	if normFormat == "text" || normFormat == "md" || normFormat == "markdown" {
		normFormat = ExportTXT
	}
	if normFormat == "htm" {
		normFormat = ExportHTML
	}

	outPath := req.Output
	if outPath == "" {
		exportDir := filepath.Join(a.dir, "exports")
		if err := os.MkdirAll(exportDir, 0700); err != nil {
			kit.Fail(w, http.StatusInternalServerError, fmt.Sprintf("Could not create exports directory: %v", err))
			return
		}
		outPath = filepath.Join(exportDir, fmt.Sprintf("%s.%s", n.ID, normFormat))
	} else {
		if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
			kit.Fail(w, http.StatusInternalServerError, fmt.Sprintf("Could not create directory for %s: %v", outPath, err))
			return
		}
	}

	if err := os.WriteFile(outPath, data, 0644); err != nil {
		kit.Fail(w, http.StatusInternalServerError, fmt.Sprintf("Could not write exported file: %v", err))
		return
	}

	kit.WriteJSON(w, map[string]any{
		"id":     n.ID,
		"format": normFormat,
		"path":   outPath,
	})
}

// page renders the human door: item header, status badges, copy buttons,
// markdown editor, file converter and theme toggle, from the kit components.
func (a *app) page(r *http.Request) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := r.URL.Query().Get("id")
	header := kit.EmptyItemHeader("new note")
	noteTitle := ""
	noteBody := ""
	currentURL := a.baseURL + "/"
	if id != "" {
		if n, err := a.loadLocked(id); err == nil {
			header = kit.ItemHeader(n.ID, n.Status, a.urlFor(n.ID))
			noteTitle = n.Title
			noteBody = n.Body
			currentURL = a.urlFor(n.ID)
		}
	}

	var badges strings.Builder
	for _, s := range []string{
		kit.StatusPending, kit.StatusNeeds, kit.StatusFilled,
		kit.StatusAnswered, kit.StatusFailed, kit.StatusClosed,
	} {
		badges.WriteString(kit.Badge(s))
	}

	// List of other notes
	var noteLinks strings.Builder
	names, _ := filepath.Glob(filepath.Join(a.dir, "items", "*.json"))
	noteLinks.WriteString(`<section class="desk-panel"><h2 class="desk-h2">All notes</h2>`)
	if len(names) > 0 {
		noteLinks.WriteString(`<ul class="np-notes">`)
		for _, p := range names {
			nid := strings.TrimSuffix(filepath.Base(p), ".json")
			u := kit.ItemURL(a.baseURL+"/", "id", nid)
			noteLinks.WriteString(`<li><a href="` + html.EscapeString(u) + `">` + html.EscapeString(nid) + `</a></li>`)
		}
		noteLinks.WriteString(`</ul>`)
	} else {
		noteLinks.WriteString(kit.State("empty", "No notes yet", "Write something and press Save (⌘/Ctrl+S), or notepad new from the CLI."))
	}
	noteLinks.WriteString(`</section>`)

	body := `<style>
.np-split{display:grid;grid-template-columns:repeat(auto-fit,minmax(min(100%,340px),1fr));gap:var(--desk-space-4);align-items:start}
.np-split .desk-panel{margin-bottom:0}.np-split textarea{min-height:320px;font-family:var(--desk-font-mono);font-size:var(--desk-text-sm)}
.np-notes{margin:0;padding-left:1.2em;columns:2 220px}
</style>` + kit.AppBar("notepad", a.baseURL+"/", header, "") + `<main class="desk-main">` +
		`<div class="desk-statuses">` + badges.String() + `</div>` +
		`<div data-desk-alert class="desk-alert"></div>` +
		`<form id="noteForm" onsubmit="return false"><div class="np-split"><section class="desk-panel">` +
		kit.TextField("title", "Title", "text", noteTitle, "") +
		kit.TextField("body", "Note content (Markdown)", "textarea", noteBody, "recommended") +
		`<div class="desk-field"><label>Import file (TXT, MD, HTML, JSON, PDF) <span class="desk-recommended">auto-convert</span></label>` +
		`<input type="file" id="importFile" accept=".txt,.md,.markdown,.html,.htm,.json,.pdf"></div>` +
		`<div class="desk-row">` +
		`<button type="button" class="desk-copy" id="saveBtn" data-desk-shortcut="save" title="Save (⌘/Ctrl+S)">Save Note</button>` +
		`<button type="button" class="desk-copy secondary" id="newBtn">New Note</button>` +
		kit.CopyButton("Copy link", currentURL) +
		kit.CopyButton("Copy text", noteBody) +
		`</div></section>` +
		`<section class="desk-panel"><h2 class="desk-h2">Preview</h2>` +
		kit.Rendered("div", "block", noteBody) + `</section></div></form>` +
		noteLinks.String() +
		`<p class="desk-footer">Local-first notepad · loopback only · re-surface with <code>notepad open ` + html.EscapeString(id) + `</code>.</p></main>` +
		`<script>
(function() {
  let currentId = ` + jsonString(id) + `;
  const titleInput = document.querySelector('input[name="title"]');
  const bodyInput = document.querySelector('textarea[name="body"]');
  const alertBox = document.querySelector('[data-desk-alert]');
  const importFile = document.getElementById('importFile');
  const saveBtn = document.getElementById('saveBtn');
  const newBtn = document.getElementById('newBtn');

  function showAlert(msg, isError) {
    if (!alertBox) return;
    alertBox.textContent = msg;
    alertBox.setAttribute('data-kind', isError ? 'error' : 'ok');
    if (!isError && window.deskToast) window.deskToast(msg, 'ok');
  }
  const preview = document.querySelector('[data-desk-render]');
  if (bodyInput && preview && window.deskRender) {
    bodyInput.addEventListener('input', () => window.deskRender(preview, bodyInput.value));
  }

  if (saveBtn) {
    saveBtn.addEventListener('click', async () => {
      const action = currentId ? 'set' : 'add';
      try {
        const res = await fetch('/api/action', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({
            action: action,
            id: currentId || undefined,
            title: titleInput ? titleInput.value : '',
            body: bodyInput ? bodyInput.value : ''
          })
        });
        const data = await res.json();
        if (!res.ok) {
          showAlert(data.error?.message || 'Failed to save', true);
          return;
        }
        showAlert('Saved successfully: ' + data.id, false);
        if (!currentId && data.id) {
          currentId = data.id;
          const u = new URL(window.location.href);
          u.searchParams.set('id', data.id);
          history.replaceState(null, '', u.toString());
          const headerId = document.querySelector('.desk-item-header .desk-id');
          if (headerId) headerId.textContent = data.id;
          const headerTools = document.querySelector('.desk-item-header .desk-tools');
          if (headerTools && data.url) {
            headerTools.innerHTML = '<button type="button" class="desk-copy" data-desk-copy data-copy="' + data.url.replace(/"/g, '&quot;') + '" data-copied="false">Copy link</button>';
          }
          document.querySelectorAll('button[data-desk-copy]').forEach(btn => {
            if (btn.textContent.includes('Copy link') && data.url) {
              btn.setAttribute('data-copy', data.url);
            }
          });
          const resurface = document.querySelector('.desk-footer code');
          if (resurface) resurface.textContent = 'notepad open ' + data.id;
        }
        document.querySelectorAll('button[data-desk-copy]').forEach(btn => {
          if (btn.textContent.includes('Copy text') && bodyInput) {
            btn.setAttribute('data-copy', bodyInput.value);
          }
        });
      } catch (err) {
        showAlert('Save error: ' + err.message, true);
      }
    });
  }

  if (newBtn) {
    newBtn.addEventListener('click', () => {
      window.location.href = window.location.pathname;
    });
  }

  if (importFile) {
    importFile.addEventListener('change', async (e) => {
      const file = e.target.files[0];
      if (!file) return;
      const reader = new FileReader();
      reader.onload = async () => {
        try {
          const res = await fetch('/api/action', {
            method: 'POST',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({
              action: 'convert',
              body: reader.result,
              filename: file.name
            })
          });
          const data = await res.json();
          if (res.ok && data.text !== undefined) {
            if (bodyInput) bodyInput.value = data.text;
            if (bodyInput && preview && window.deskRender) window.deskRender(preview, data.text);
            if (titleInput && !titleInput.value) titleInput.value = file.name;
            showAlert('Imported ' + file.name + ' as ' + (data.format || 'text'), false);
          } else {
            showAlert(data.error?.message || 'Import conversion failed', true);
          }
        } catch (err) {
          showAlert('Import error: ' + err.message, true);
        }
      };
      if (file.name.endsWith('.pdf')) {
        reader.readAsBinaryString(file);
      } else {
        reader.readAsText(file);
      }
    });
  }
})();
</script>`

	return kit.Page("notepad", "Notepad · bashy desk", body)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

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
	a := &app{
		dir:     data,
		prefix:  prefix,
		baseURL: origin + prefix,
		notes:   map[string]*note{},
	}
	fmt.Printf("\nDesk notepad — keep this terminal running\nParticipant: %s/\nSaved notes: %s\nCtrl-C stops the server.\n\n",
		a.baseURL, filepath.Join(data, "items"))
	srv := &http.Server{
		Handler:           a,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
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
