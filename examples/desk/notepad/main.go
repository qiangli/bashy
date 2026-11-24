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
	"errors"
	"fmt"
	"html"
	"log"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/qiangli/bashy/examples/desk/chart/fence"
	kit "github.com/qiangli/bashy/examples/desk/kit"
	"github.com/qiangli/bashy/examples/desk/kit/render"
)

// breakMode is set from -break; empty means contract-clean.
var breakMode string

func init() {
	// Markdown ```chart fences in notes reuse the C1 renderer.
	render.ChartSVG = fence.RenderFence
}

// renderMarkdown serves POST /api/render: Markdown in, sanitized HTML out.
// Notes have no item assets, so local embeds degrade to links and remote
// embeds stay off.
func renderMarkdown(src []byte) ([]byte, error) {
	return render.Markdown(src, render.Options{})
}

const noteParticipantCookie = "notepad-participant"

type note struct {
	Version      int                         `json:"version"`
	ID           string                      `json:"id"`
	Title        string                      `json:"title"`
	Body         string                      `json:"body"`
	Fields       []kit.Field                 `json:"fields,omitempty"`
	Status       string                      `json:"status"`
	Revision     int                         `json:"revision"`
	Active       string                      `json:"-"`
	Owner        string                      `json:"owner"`
	Creator      string                      `json:"creator,omitempty"`
	OwnerCookie  string                      `json:"owner_cookie,omitempty"`
	Participants map[string]*kit.Participant `json:"participants,omitempty"`
	Invites      map[string]bool             `json:"invites"`
}

type actionRequest struct {
	kit.RevisionAction
	Title    string      `json:"title"`
	Body     *string     `json:"body"`
	Fields   []kit.Field `json:"fields,omitempty"`
	Invite   string      `json:"invite,omitempty"`
	Handle   *string     `json:"handle,omitempty"`
	Format   string      `json:"format,omitempty"`
	Filename string      `json:"filename,omitempty"`
	To       string      `json:"to,omitempty"`
	Output   string      `json:"output,omitempty"`
}

type app struct {
	mu               sync.Mutex
	dir              string
	prefix           string
	baseURL          string // public origin, e.g. http://127.0.0.1:18095
	notes            map[string]*note
	outpostSecret    string
	consolePrincipal string
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
	if n.Invites == nil {
		n.Invites = map[string]bool{}
	}
	active, body, err := a.revisions().Current(id, "")
	if err != nil {
		return nil, err
	}
	if active.Active != "" {
		n.Active, n.Body = active.Active, string(body)
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

func (a *app) revisions() kit.RevisionStore {
	return kit.RevisionStore{Dir: a.dir}
}

func (a *app) caller(r *http.Request, allowBareMount bool) (kit.ParticipantCaller, bool) {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	loopback := net.ParseIP(host).IsLoopback()
	h := r.Header
	hop := h.Get("X-Forwarded-For")
	stamped := h.Get("X-Outpost-Identity-Ts") != "" || h.Get("X-Outpost-Relationship") != "" ||
		h.Get("X-Outpost-Identity-Sig") != "" || h.Get("X-Outpost-Relationship-Sig") != "" || h.Get("Remote-User") != ""
	// The local command contract uses a bare mount hint to generate the
	// public URL on add. Existing notes still require the usual admission.
	direct := loopback && !stamped && hop == "" && h.Get("Forwarded") == "" &&
		(allowBareMount || h.Get("X-Forwarded-Prefix") == "") &&
		h.Get("X-Forwarded-Host") == "" && h.Get("X-Forwarded-Proto") == ""
	principal := ""
	if direct || (loopback && !stamped && net.ParseIP(hop).IsLoopback()) {
		principal = a.consolePrincipal
	}
	caller := kit.IdentifyParticipant(r, noteParticipantCookie, principal, a.outpostSecret, time.Now())
	if stamped && !caller.Tunnel {
		// An unverified proxy must not bind a guest to its loopback address.
		caller.Address = ""
	}
	return caller, direct
}

func isNoteOwner(r *http.Request, n *note, caller kit.ParticipantCaller, local bool) bool {
	return kit.Grant(kit.FacilitatorKey(r), n.Owner) ||
		(caller.Relationship == "owner" && caller.Auth == "member") ||
		(caller.Principal != "" && caller.Principal == n.Creator) ||
		(local && kit.Grant(caller.Cookie, n.OwnerCookie))
}

func noteAuthor(r *http.Request, n *note, caller kit.ParticipantCaller, local bool) (kit.Participant, bool) {
	if caller.Auth == "member" {
		return kit.Participant{Handle: caller.Handle, Auth: caller.Auth}, true
	}
	if isNoteOwner(r, n, caller, local) {
		return kit.Participant{Handle: "owner", Auth: "anonymous"}, true
	}
	key := caller.Principal
	if key == "" {
		key = kit.ParticipantCookieKey(n.Participants, caller)
	}
	if p := n.Participants[key]; p != nil && !p.Kicked {
		return *p, true
	}
	if local {
		return kit.Participant{Handle: "local", Auth: "anonymous"}, true
	}
	return kit.Participant{}, false
}

// Save the item and content under the same app lock. The item write can be
// rolled back if publishing the revision fails.
func (a *app) saveRevisionLocked(prev, next *note, base string, author kit.Participant) error {
	store := a.revisions()
	if base != "" && (prev == nil || base != prev.Active) {
		active := ""
		if prev != nil {
			active = prev.Active
		}
		return &kit.RevisionConflictError{Item: next.ID, BaseRev: base, Active: active}
	}
	if prev != nil && prev.Active == "" {
		// Keep the first undo available for notes saved before history existed.
		// The old note has no author metadata.
		seed, err := store.SaveRevision(kit.SaveInput{
			Item: next.ID, Author: "legacy", Kind: "anonymous", Data: []byte(prev.Body),
		})
		if err != nil {
			return err
		}
		base, prev.Active = seed.Active, seed.Active
	}
	if err := a.saveLocked(next); err != nil {
		return err
	}
	state, err := store.SaveRevision(kit.SaveInput{
		Item: next.ID, BaseRev: base, Author: author.Handle, Kind: author.Auth, Data: []byte(next.Body),
	})
	if err != nil {
		if prev == nil {
			_ = os.Remove(notePath(a.dir, next.ID))
			delete(a.notes, next.ID)
		} else {
			raw, marshalErr := kit.MarshalIndent(*prev)
			if marshalErr == nil {
				_ = kit.AtomicWrite(notePath(a.dir, prev.ID), raw)
			}
			a.notes[prev.ID] = prev
		}
		return err
	}
	next.Active = state.Active
	return nil
}

// base is the public URL of the tool root for r: its own address, or the app
// shell mount it was reached through (kit.PublicBase).
func (a *app) base(r *http.Request) string { return kit.PublicBase(r, a.baseURL) }

func (a *app) urlFor(r *http.Request, id string) string {
	return kit.ItemURL(a.base(r)+"/", "id", id)
}

// openView is the contentless re-surface: id + stable URL + status only.
func (a *app) openView(r *http.Request, n *note) map[string]any {
	v := kit.OpenView(n.ID, a.urlFor(r, n.ID), n.Status)
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
		kit.SecurityOptions{Embeddable: true}.Apply(w)
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
		status, body := a.page(r)
		kit.ParticipantCookie(w, r, noteParticipantCookie)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		w.Write(body)
		return
	}
	if path == "/healthz" && r.Method == "GET" {
		kit.WriteJSON(w, map[string]bool{"ok": true})
		return
	}
	if path == "/api/state" && r.Method == "GET" {
		a.mu.Lock()
		defer a.mu.Unlock()
		kit.ParticipantCookie(w, r, noteParticipantCookie)
		id := r.URL.Query().Get("id")
		n, err := a.loadLocked(id)
		if err != nil {
			respondNoteLoadError(w, err)
			return
		}
		caller, local := a.caller(r, false)
		if _, ok := noteAuthor(r, n, caller, local); !ok {
			kit.Fail(w, http.StatusForbidden, "Join this note first; ask the owner for an invite.")
			return
		}
		kit.WriteJSON(w, a.openView(r, n))
		return
	}
	if path == "/api/export" {
		a.mu.Lock()
		defer a.mu.Unlock()
		caller, local := a.caller(r, false)
		var req actionRequest
		if r.Method == "GET" {
			req = actionRequest{
				RevisionAction: kit.RevisionAction{Action: "export", ID: r.URL.Query().Get("id"), Rev: r.URL.Query().Get("rev")},
				To:             r.URL.Query().Get("to"),
				Format:         r.URL.Query().Get("format"),
				Output:         r.URL.Query().Get("output"),
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
		a.export(w, r, req, caller, local)
		return
	}
	if path == "/api/render" {
		kit.ServeRender(w, r, renderMarkdown)
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
	caller, local := a.caller(r, req.Action == "add")
	switch req.Action {
	case "add":
		a.create(w, r, req, caller, local)
	case "open":
		a.open(w, r, req, caller, local)
	case "show":
		a.get(w, r, req, caller, local)
	case "set":
		a.mutate(w, r, req, caller, local, false)
	case "close":
		a.mutate(w, r, req, caller, local, true)
	case "list":
		a.list(w, r, caller, local)
	case "share":
		a.share(w, r, req, caller, local)
	case "join":
		a.join(w, r, req, caller)
	case "history", "undo", "restore":
		a.revisionAction(w, r, req, caller, local)
	case "convert":
		a.convert(w, req)
	case "export":
		a.export(w, r, req, caller, local)
	default:
		kit.Fail(w, http.StatusBadRequest, "Unknown action")
	}
}

func (a *app) create(w http.ResponseWriter, r *http.Request, req actionRequest, caller kit.ParticipantCaller, local bool) {
	if !local && caller.Auth != "member" {
		kit.Fail(w, http.StatusForbidden, "Only roster members can create a note")
		return
	}
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
	body := noteBody(req)
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
		Creator: caller.Principal,
		Invites: map[string]bool{},
	}
	if local {
		kit.MintParticipantCookie(w, r, noteParticipantCookie, &caller)
		n.OwnerCookie = caller.Cookie
	}
	author := kit.Participant{Handle: "owner", Auth: "anonymous"}
	if caller.Auth == "member" {
		author.Handle, author.Auth = caller.Handle, caller.Auth
	}
	if err := a.saveRevisionLocked(nil, n, "", author); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, n)
	v["ownerKey"] = n.Owner // revealed once, at creation
	v["active"] = n.Active
	kit.WriteJSON(w, v)
}

func (a *app) open(w http.ResponseWriter, r *http.Request, req actionRequest, caller kit.ParticipantCaller, local bool) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		respondNoteLoadError(w, err)
		return
	}
	if _, ok := noteAuthor(r, n, caller, local); !ok {
		kit.Fail(w, http.StatusForbidden, "Join this note first; ask the owner for an invite.")
		return
	}
	kit.WriteJSON(w, a.openView(r, n))
}

func (a *app) get(w http.ResponseWriter, r *http.Request, req actionRequest, caller kit.ParticipantCaller, local bool) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		respondNoteLoadError(w, err)
		return
	}
	if _, ok := noteAuthor(r, n, caller, local); !ok {
		kit.Fail(w, http.StatusForbidden, "Join this note first; ask the owner for an invite.")
		return
	}
	kit.WriteJSON(w, map[string]any{
		"id":       n.ID,
		"url":      a.urlFor(r, n.ID),
		"status":   n.Status,
		"title":    n.Title,
		"body":     n.Body,
		"fields":   n.Fields,
		"revision": n.Revision,
		"active":   n.Active,
	})
}

func (a *app) mutate(w http.ResponseWriter, r *http.Request, req actionRequest, caller kit.ParticipantCaller, local, closing bool) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		respondNoteLoadError(w, err)
		return
	}
	author, ok := noteAuthor(r, n, caller, local)
	if !ok {
		kit.Fail(w, http.StatusForbidden, "Only members or the owner can change this note")
		return
	}
	if n.Status == kit.StatusClosed {
		kit.Fail(w, http.StatusConflict, "This note is closed")
		return
	}
	next := *n
	req.BaseRev = noteRevision(req.BaseRev)
	if closing {
		next.Status = kit.StatusClosed
	} else {
		if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
			kit.Fail(w, http.StatusBadRequest, kit.ErrSecret)
			return
		}
		if req.Body != nil {
			next.Body = noteBody(req)
		}
		if req.Fields != nil {
			next.Fields = req.Fields
		}
		if strings.TrimSpace(req.Title) != "" {
			next.Title = req.Title
		}
		next.Status = kit.StatusFilled
	}
	var errSave error
	switch {
	case !closing && req.Body != nil:
		errSave = a.saveRevisionLocked(n, &next, req.BaseRev, author)
	case !closing && req.BaseRev != "" && req.BaseRev != n.Active:
		errSave = &kit.RevisionConflictError{Item: n.ID, BaseRev: req.BaseRev, Active: n.Active}
	default:
		errSave = a.saveLocked(&next)
	}
	if errSave != nil {
		if !kit.WriteRevisionConflict(w, errSave) {
			kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		}
		return
	}
	view := a.openView(r, &next)
	if !closing {
		view["active"] = next.Active
	}
	kit.WriteJSON(w, view)
}

func noteBody(req actionRequest) string {
	if req.Body == nil {
		return ""
	}
	body := *req.Body
	if req.Format != "" || req.Filename != "" {
		if converted, _, err := ConvertContent([]byte(body), req.Format+req.Filename); err == nil {
			return converted
		}
	}
	return body
}

func (a *app) revisionAction(w http.ResponseWriter, r *http.Request, req actionRequest, caller kit.ParticipantCaller, local bool) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		respondNoteLoadError(w, err)
		return
	}
	if _, ok := noteAuthor(r, n, caller, local); !ok {
		kit.Fail(w, http.StatusForbidden, "Join this note first; ask the owner for an invite.")
		return
	}
	if req.Action != "history" && n.Status == kit.StatusClosed {
		kit.Fail(w, http.StatusConflict, "This note is closed")
		return
	}
	if req.Part != "" {
		kit.Fail(w, http.StatusBadRequest, "Notepad tracks revisions for the whole note body")
		return
	}
	req.Rev = noteRevision(req.Rev)
	req.Revision = noteRevision(req.Revision)
	a.revisions().ServeAction(w, req.RevisionAction, func(state kit.RevisionState, body []byte) error {
		next := *n
		next.Body, next.Active = string(body), state.Active
		return a.saveLocked(&next)
	})
}

func noteRevision(value string) string {
	value = strings.TrimSpace(value)
	if n, err := strconv.Atoi(value); err == nil && n > 0 {
		return "r" + strconv.Itoa(n)
	}
	return value
}

func respondNoteLoadError(w http.ResponseWriter, err error) {
	if os.IsNotExist(err) {
		kit.Fail(w, http.StatusNotFound, "Note not found. Create it with notepad add first.")
	} else {
		kit.Fail(w, http.StatusBadRequest, err.Error())
	}
}

func (a *app) list(w http.ResponseWriter, r *http.Request, caller kit.ParticipantCaller, local bool) {
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
		if _, ok := noteAuthor(r, n, caller, local); !ok {
			continue
		}
		out = append(out, a.openView(r, n)) // ids + statuses, never content
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	kit.WriteJSON(w, out)
}

func (a *app) share(w http.ResponseWriter, r *http.Request, req actionRequest, caller kit.ParticipantCaller, local bool) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		respondNoteLoadError(w, err)
		return
	}
	if !isNoteOwner(r, n, caller, local) {
		kit.Fail(w, http.StatusForbidden, "Only the owner can share this note")
		return
	}
	next := *n
	next.Invites = maps.Clone(n.Invites)
	code := kit.NewToken()
	next.Invites[code] = true
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, &next)
	v["invite"] = code
	v["inviteURL"] = a.urlFor(r, next.ID) + "#invite=" + code
	kit.WriteJSON(w, v)
}

func (a *app) join(w http.ResponseWriter, r *http.Request, req actionRequest, caller kit.ParticipantCaller) {
	n, err := a.loadLocked(req.ID)
	if err != nil {
		respondNoteLoadError(w, err)
		return
	}
	if !kit.ValidToken(req.Invite) || !n.Invites[req.Invite] {
		kit.Fail(w, http.StatusForbidden, "This invite is invalid. Ask the owner for a new one.")
		return
	}
	if req.Handle == nil {
		kit.Fail(w, http.StatusForbidden, "Enter a name or email to join this note.")
		return
	}
	handle, err := kit.CleanHandle(*req.Handle)
	if err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	next := *n
	next.Participants = maps.Clone(n.Participants)
	if caller.Principal == "" {
		kit.MintParticipantCookie(w, r, noteParticipantCookie, &caller)
	}
	// ResolveParticipant updates the row in place. Copy existing rows so a
	// failed invite acceptance keeps the prior bindings and the invite.
	for key, p := range next.Participants {
		if p != nil {
			copy := *p
			next.Participants[key] = &copy
		}
	}
	if _, err := kit.ResolveParticipant(&next.Participants, false, caller, &handle, false, time.Now()); err != nil {
		status := http.StatusForbidden
		if errors.Is(err, kit.ErrHandleInUse) {
			status = http.StatusConflict
		}
		kit.Fail(w, status, err.Error())
		return
	}
	next.Invites = maps.Clone(n.Invites)
	delete(next.Invites, req.Invite) // single use
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(r, &next))
}

func (a *app) convert(w http.ResponseWriter, req actionRequest) {
	var body string
	if req.Body != nil {
		body = *req.Body
	}
	conv, format, err := ConvertContent([]byte(body), req.Format+req.Filename)
	if err != nil {
		kit.Fail(w, http.StatusBadRequest, fmt.Sprintf("Conversion failed: %v", err))
		return
	}
	kit.WriteJSON(w, map[string]any{"text": conv, "format": format})
}

func (a *app) export(w http.ResponseWriter, r *http.Request, req actionRequest, caller kit.ParticipantCaller, local bool) {
	if req.ID == "" {
		kit.Fail(w, http.StatusBadRequest, "Note id required for export")
		return
	}
	n, err := a.loadLocked(req.ID)
	if err != nil {
		respondNoteLoadError(w, err)
		return
	}
	if _, ok := noteAuthor(r, n, caller, local); !ok {
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
	if req.Output != "" && !local && !kit.Grant(kit.FacilitatorKey(r), n.Owner) {
		kit.Fail(w, http.StatusForbidden, "A custom export path requires local access or the owner key")
		return
	}
	rev := noteRevision(req.Rev)
	body := n.Body
	if rev != "" {
		snapshot, _, err := a.revisions().Read(n.ID, "", rev)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, kit.ErrUnknownRevision) {
				status = http.StatusNotFound
			}
			kit.Fail(w, status, err.Error())
			return
		}
		body = string(snapshot)
	}
	data, err := ExportContent(n.Title, body, toFormat)
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
		name := n.ID
		if rev != "" {
			name += "-" + rev
		}
		outPath = filepath.Join(exportDir, fmt.Sprintf("%s.%s", name, normFormat))
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

	view := map[string]any{"id": n.ID, "format": normFormat, "path": outPath}
	if rev != "" {
		view["revision"] = rev
	}
	kit.WriteJSON(w, view)
}

// page renders the human door: item header, status badges, copy buttons,
// markdown editor, file converter and theme toggle, from the kit components.
func (a *app) page(r *http.Request) (int, []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	caller, local := a.caller(r, false)
	id := r.URL.Query().Get("id")
	header := kit.EmptyItemHeader("new note")
	noteTitle, noteBody, active := "", "", ""
	if id != "" {
		n, err := a.loadLocked(id)
		if err == nil {
			if _, ok := noteAuthor(r, n, caller, local); !ok {
				return http.StatusForbidden, joinNotePage(id)
			}
			header = kit.ItemHeader(n.ID, n.Status, a.urlFor(r, n.ID))
			noteTitle, noteBody, active = n.Title, n.Body, n.Active
		}
	}

	// List of other notes
	var noteLinks strings.Builder
	names, _ := filepath.Glob(filepath.Join(a.dir, "items", "*.json"))
	noteLinks.WriteString(`<section class="desk-panel"><h2 class="desk-h2">All notes</h2>`)
	if len(names) > 0 {
		noteLinks.WriteString(`<ul class="np-notes">`)
		for _, p := range names {
			nid := strings.TrimSuffix(filepath.Base(p), ".json")
			n, err := a.loadLocked(nid)
			if err != nil {
				continue
			}
			if _, ok := noteAuthor(r, n, caller, local); !ok {
				continue
			}
			u := kit.ItemURL(a.base(r)+"/", "id", nid)
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
</style>` + kit.AppBar("notepad", a.base(r)+"/", header, "") + `<main class="desk-main">` +
		`<div data-desk-alert class="desk-alert"></div>` +
		`<form id="noteForm" onsubmit="return false">` +
		`<div class="desk-row" role="group" aria-label="Editor layout"><span class="desk-muted">Layout</span>` +
		`<button type="button" class="desk-copy secondary" id="modeEdit">Edit</button>` +
		`<button type="button" class="desk-copy secondary" id="modeSplit">Split</button>` +
		`<button type="button" class="desk-copy secondary" id="modeView">View</button></div>` +
		`<div class="np-split"><section class="desk-panel" id="npEdit">` +
		kit.TextField("title", "Title", "text", noteTitle, "") +
		kit.TextField("body", "Note content (Markdown)", "textarea", noteBody, "recommended") +
		`<div class="desk-field"><label>Import file (TXT, MD, HTML, JSON, PDF) <span class="desk-recommended">auto-convert</span></label>` +
		`<input type="file" id="importFile" accept=".txt,.md,.markdown,.html,.htm,.json,.pdf"></div>` +
		`<div class="desk-row">` +
		`<button type="button" class="desk-copy" id="saveBtn" data-desk-shortcut="save" title="Save (⌘/Ctrl+S)">Save Note</button>` +
		`<button type="button" class="desk-copy secondary" id="newBtn">New Note</button>` +
		kit.CopyButton("Copy text", noteBody) +
		`</div></section>` +
		`<section class="desk-panel" id="npView"><h2 class="desk-h2">Preview</h2>` +
		kit.Rendered("div", "block", noteBody) + `</section></div></form>` +
		noteLinks.String() +
		`<p class="desk-footer">Local-first notepad · loopback only · re-surface with <code>notepad open ` + html.EscapeString(id) + `</code>.</p></main>` +
		`<script>
(function() {
  let currentId = ` + jsonString(id) + `;
  let activeRevision = ` + jsonString(active) + `;
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

  // Edit/split/view toggle: the preview is server-rendered through the
  // tool's POST api/render endpoint, so every mode shares one renderer.
  const editPane = document.getElementById('npEdit');
  const viewPane = document.getElementById('npView');
  const modeBtns = {
    edit: document.getElementById('modeEdit'),
    split: document.getElementById('modeSplit'),
    view: document.getElementById('modeView')
  };
  function setMode(m) {
    if (m !== 'edit' && m !== 'view') m = 'split';
    try { localStorage.setItem('notepad-mode', m); } catch (e) {}
    if (editPane) editPane.hidden = (m === 'view');
    if (viewPane) viewPane.hidden = (m === 'edit');
    for (const k in modeBtns) {
      if (modeBtns[k]) modeBtns[k].disabled = (k === m);
    }
    if (m !== 'edit' && bodyInput && preview && window.deskRender) {
      window.deskRender(preview, bodyInput.value);
    }
  }
  if (modeBtns.edit) modeBtns.edit.addEventListener('click', () => setMode('edit'));
  if (modeBtns.split) modeBtns.split.addEventListener('click', () => setMode('split'));
  if (modeBtns.view) modeBtns.view.addEventListener('click', () => setMode('view'));
  let savedMode = 'split';
  try {
    const s = localStorage.getItem('notepad-mode');
    if (s === 'edit' || s === 'split' || s === 'view') savedMode = s;
  } catch (e) {}
  setMode(savedMode);

  if (saveBtn) {
    saveBtn.addEventListener('click', async () => {
      const action = currentId ? 'set' : 'add';
      try {
        const res = await fetch('api/action', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({
            action: action,
            id: currentId || undefined,
            title: titleInput ? titleInput.value : '',
            body: bodyInput ? bodyInput.value : '',
            base_rev: activeRevision || undefined
          })
        });
        const data = await res.json();
        if (!res.ok) {
          showAlert(typeof data.error === 'string' ? data.error : data.error?.message || 'Failed to save', true);
          return;
        }
        activeRevision = data.active || '';
        showAlert('Saved successfully: ' + data.id, false);
        if (!currentId && data.id) {
          currentId = data.id;
          const u = new URL(window.location.href);
          u.searchParams.set('id', data.id);
          history.replaceState(null, '', u.toString());
          const headerId = document.querySelector('.desk-item-header .desk-id');
          if (headerId) headerId.textContent = data.id;
          const shareLink = document.getElementById('deskShareLink');
          if (shareLink && data.url) shareLink.setAttribute('data-copy', data.url);
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
          const res = await fetch('api/action', {
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

	return http.StatusOK, kit.Page("notepad", "Notepad · bashy desk", body)
}

func joinNotePage(id string) []byte {
	body := `<main class="desk-main"><section class="desk-panel">
<h1>Join this note</h1>
<p id="inviteMessage">Ask the owner for an invite.</p>
<form id="inviteForm" hidden>
<label for="handle">Name or email</label>
<input id="handle" name="handle" type="text" maxlength="80" required>
<button type="submit">Join</button>
</form>
</section></main>
<script>
(function() {
  const invite = new URLSearchParams(window.location.hash.slice(1)).get('invite');
  if (!invite) return;
  const form = document.getElementById('inviteForm');
  const message = document.getElementById('inviteMessage');
  form.hidden = false;
  message.textContent = '';
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    try {
      const res = await fetch('api/action', {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({
          action: 'join', id: ` + jsonString(id) + `, invite: invite, handle: form.elements.handle.value
        })
      });
      if (!res.ok) {
        const data = await res.json();
        message.textContent = data.error?.message || 'Could not join this note';
        return;
      }
      const url = new URL(window.location.href);
      url.hash = '';
      window.location.replace(url);
    } catch (err) {
      message.textContent = 'Could not join this note';
    }
  });
})();
</script>`
	return kit.Page("notepad", "Join this note", body)
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
		dir:           data,
		prefix:        prefix,
		baseURL:       origin + prefix,
		notes:         map[string]*note{},
		outpostSecret: notepadOutpostSecret(),
	}
	if os.Getenv("BASHY_APP") == "notepad" && os.Getenv("BASHY_APP_ADDR") == addr {
		a.consolePrincipal = strings.TrimSpace(os.Getenv("BASHY_PRINCIPAL"))
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

func notepadOutpostSecret() string {
	if secret := strings.TrimSpace(os.Getenv("NOTEPAD_SSO_SECRET")); secret != "" {
		return secret
	}
	bin := os.Getenv("OUTPOST_BIN")
	if bin == "" {
		bin = "outpost"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(ctx, bin, "apps", "secret", "notepad").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func main() {
	os.Exit(runCLI(os.Args[1:], os.Stdout))
}
