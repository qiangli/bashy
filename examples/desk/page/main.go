// Command page is the bashy desk page tool: one fixed, live page for a
// group. An item is Markdown written by the organizer plus ```block fences,
// each a live view over an item another desk app owns (blocks.go), an
// audience, the participants and a revision counter
// (docs/page-live-group-page.md in the umbrella).
//
// Doors: `page add|open|show|set|list|share|close|export [--json]` (agent)
// and the page (human). Storage: $BASHY_HOME/desk/page/items/<id>.json.
// Live: GET /api/events?id= streams {item, block, revision} frames
// (kit.Events) after every acknowledged save.
//
// The -break flag deliberately violates one contract point so the tests can
// prove the contract test goes red:
//
//	-break open-emits-content   open re-emits the page body
//	-break no-csp                security headers are dropped
//	-break allow-secret          secret fields are stored
//	-break events-no-publish     set saves but publishes no live frame
//	-break export-addresses      the export carries participant addresses
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	kit "desk-kit"
	"desk-kit/render"
)

// breakMode is set from -break; empty means contract-clean.
var breakMode string

const tool = "page"

// Audiences: who may open the page. host and satellite pages are served to
// loopback peers only (this host, and the outpost tunnel that terminates
// here with --require-login); a lan page is also served to LAN peers when
// the server runs with the logged --lan override.
const (
	audienceHost      = "host"
	audienceLAN       = "lan"
	audienceSatellite = "satellite"
)

func validAudience(s string) bool {
	return s == audienceHost || s == audienceLAN || s == audienceSatellite
}

// renderMarkdown serves POST /api/render: Markdown in, sanitized HTML out.
func renderMarkdown(src []byte) ([]byte, error) {
	return render.Markdown(src, render.Options{})
}

// Participant contains the private fields used to bind an identity.
// Public views and exports use exportedParticipants.
type Participant struct {
	Handle      string `json:"handle"`
	Principal   string `json:"principal,omitempty"`
	Address     string `json:"address,omitempty"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Cookie      string `json:"cookie,omitempty"`
	Auth        string `json:"auth"` // member | signed-in | anonymous
	JoinedAt    string `json:"joined_at,omitempty"`
	LastSeen    string `json:"last_seen,omitempty"`
	Kicked      bool   `json:"kicked,omitempty"`
}

type item struct {
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Body         string            `json:"body"`
	Audience     string            `json:"audience"`
	Participants participantMap    `json:"participants"`
	JoinCode     string            `json:"join_code,omitempty"`
	OpenHandles  bool              `json:"open_handles,omitempty"`
	Votes        map[string]string `json:"votes,omitempty"`
	// Natives are the poll and form blocks this page owns (form owner
	// keys); Respondents is the cookie jar towards form (participant key
	// -> form respondent token). Neither leaves the store.
	Natives     map[string]*nativeBlock `json:"natives,omitempty"`
	Respondents map[string]string       `json:"respondents,omitempty"`
	Status      string                  `json:"status"`
	Revision    int                     `json:"revision"`
	Owner       string                  `json:"owner"`
	Members     map[string]bool         `json:"members"`
	Invites     map[string]bool         `json:"invites"`
}

type actionRequest struct {
	Action   string      `json:"action"`
	ID       string      `json:"id"`
	Title    string      `json:"title"`
	Body     string      `json:"body"`
	Audience string      `json:"audience,omitempty"`
	Fields   []kit.Field `json:"fields,omitempty"`
	Invite   string      `json:"invite,omitempty"`
	JoinCode string      `json:"join_code,omitempty"`
	Kind     string      `json:"kind,omitempty"`
	Source   string      `json:"source,omitempty"`
	Target   string      `json:"target,omitempty"`
	// Native blocks (block_form.go).
	Block          string          `json:"block,omitempty"`
	Options        []string        `json:"options,omitempty"`
	Votes          int             `json:"votes,omitempty"`
	Definition     json.RawMessage `json:"definition,omitempty"`
	Answers        json.RawMessage `json:"answers,omitempty"`
	Anonymous      bool            `json:"anonymous,omitempty"`
	ResultsVisible bool            `json:"results_visible,omitempty"`
	Closed         bool            `json:"closed,omitempty"`
}

type app struct {
	mu          sync.Mutex
	dir         string
	prefix      string
	baseURL     string // public origin, e.g. http://127.0.0.1:18113
	items       map[string]*item
	events      kit.Events
	joinOptions pageJoinOptions
	lanBase     string
	formURL     string // the form app's origin (PAGE_FORM_URL)
}

// clone deep-copies the item so a failed save never leaves the in-memory
// copy mutated.
func (it *item) clone() item {
	c := *it
	c.Participants = make(participantMap, len(it.Participants))
	for key, p := range it.Participants {
		if p == nil {
			continue
		}
		copy := *p
		c.Participants[key] = &copy
	}
	c.Votes = maps.Clone(it.Votes)
	c.Respondents = maps.Clone(it.Respondents)
	if it.Natives != nil {
		c.Natives = make(map[string]*nativeBlock, len(it.Natives))
		for id, nb := range it.Natives {
			if nb == nil {
				continue
			}
			copy := *nb
			copy.Options = slices.Clone(nb.Options)
			c.Natives[id] = &copy
		}
	}
	c.Members = make(map[string]bool, len(it.Members))
	for k, v := range it.Members {
		c.Members[k] = v
	}
	c.Invites = make(map[string]bool, len(it.Invites))
	for k, v := range it.Invites {
		c.Invites[k] = v
	}
	return c
}

func itemDir(dir string) string      { return filepath.Join(dir, "items") }
func itemPath(dir, id string) string { return filepath.Join(itemDir(dir), id+".json") }

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
		return nil, fmt.Errorf("saved page %q is invalid; preserve it for recovery", id)
	}
	if it.Version != 1 || it.ID != id || !kit.ValidStatus(it.Status) || !validAudience(it.Audience) {
		return nil, fmt.Errorf("unsupported or invalid saved page %q", id)
	}
	if it.Members == nil {
		it.Members = map[string]bool{}
	}
	if it.Invites == nil {
		it.Invites = map[string]bool{}
	}
	a.items[id] = it
	return it, nil
}

// loadOr404 loads id and writes the refusal when it cannot.
func (a *app) loadOr404(w http.ResponseWriter, id string) *item {
	it, err := a.loadLocked(id)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, http.StatusNotFound, "Page not found. Create it with page add first.")
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return nil
	}
	return it
}

// saveLocked persists it at the next revision and, once the save is
// acknowledged, publishes the live frame for block ("" = the whole page).
func (a *app) saveLocked(it *item, block string) error {
	it.Revision++
	if err := a.persistLocked(it); err != nil {
		return err
	}
	if breakMode != "events-no-publish" {
		a.events.Publish(it.ID, block, it.Revision)
	}
	return nil
}

func (a *app) persistLocked(it *item) error {
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

// loopbackPeer reports whether the TCP peer is this host (or the outpost
// tunnel terminating here).
func loopbackPeer(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// canView is the audience gate for reads (page, show, state, events): the
// owner always; loopback peers always; LAN peers only for a lan page.
func (a *app) canView(r *http.Request, it *item) bool {
	return a.isOwner(r, it) || loopbackPeer(r) || it.Audience == audienceLAN
}

func (a *app) viewOr403(w http.ResponseWriter, r *http.Request, it *item) bool {
	if !a.canView(r, it) {
		kit.Fail(w, http.StatusForbidden, "This page is not shared with your network. Ask the organizer for the link.")
		return false
	}
	return true
}

// base is the public URL of the tool root for r: its own address, or the app
// shell mount it was reached through (kit.PublicBase).
func (a *app) base(r *http.Request) string { return kit.PublicBase(r, a.baseURL) }

func (a *app) urlFor(r *http.Request, id string) string {
	if it := a.items[id]; it != nil && it.JoinCode != "" {
		return pageCodeURL(a.base(r), it.JoinCode)
	}
	return kit.ItemURL(a.base(r)+"/", "id", id)
}

// openView is the contentless re-surface: id + stable URL + status only.
func (a *app) openView(r *http.Request, it *item) map[string]any {
	v := kit.OpenView(it.ID, a.urlFor(r, it.ID), it.Status)
	if it.JoinCode != "" {
		v["join_code"] = it.JoinCode
	}
	if a.lanBase != "" && it.JoinCode != "" && it.Audience == audienceLAN {
		v["lan_url"] = pageCodeURL(a.lanBase, it.JoinCode)
	}
	if breakMode == "open-emits-content" {
		v["body"] = it.Body // deliberate contract break for red/green proof
	}
	return v
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ContractHandler(tool, http.HandlerFunc(a.serveHTTP)).ServeHTTP(w, r)
}

func (a *app) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if breakMode != "no-csp" {
		kit.ApplySecurityHeaders(w)
	}
	if !kit.GuardPOST(w, r, tool) {
		return
	}
	path, ok := kit.StripPrefix(a.prefix, r.URL.Path)
	if !ok {
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}
	switch {
	case path == "/" || pageJoinCode(path) != "":
		a.joinPage(w, r, path)
	case path == "/healthz" && r.Method == "GET":
		kit.WriteJSON(w, map[string]bool{"ok": true})
	case path == "/api/state" && r.Method == "GET":
		a.mu.Lock()
		defer a.mu.Unlock()
		kit.ParticipantCookie(w, r, "page-participant")
		it := a.loadOr404(w, r.URL.Query().Get("id"))
		if it == nil || !a.viewOr403(w, r, it) {
			return
		}
		r, done, ok := a.knownParticipantLocked(w, r, it)
		if !ok {
			return
		}
		defer done()
		kit.WriteJSON(w, a.openView(r, it))
	case path == "/api/events" && r.Method == "GET":
		a.mu.Lock()
		id := r.URL.Query().Get("id")
		it := a.loadOr404(w, id)
		if it == nil || !a.viewOr403(w, r, it) {
			a.mu.Unlock()
			return
		}
		r, done, ok := a.knownParticipantLocked(w, r, it)
		revision := it.Revision
		a.mu.Unlock()
		if !ok {
			return
		}
		defer done()
		// Frames are contentless: the stream needs no more than a view.
		a.events.Serve(w, r, id, revision)
	case path == "/api/export" && r.Method == "GET":
		a.mu.Lock()
		defer a.mu.Unlock()
		a.export(w, r, r.URL.Query().Get("id"), r.URL.Query().Get("to"))
	case path == "/api/block" && r.Method == "GET":
		a.serveBlock(w, r)
	case path == "/api/render":
		kit.ServeRender(w, r, renderMarkdown)
	case path == "/api/action" && r.Method == "POST":
		var req actionRequest
		if !kit.DecodeAction(w, r, 1<<20, &req) {
			return
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		me := kit.ParticipantCookie(w, r, "page-participant")
		a.action(w, r, req, me)
	case path == "/api/action" || path == "/api/export":
		kit.Fail(w, http.StatusMethodNotAllowed, "Method not supported")
	default:
		kit.Fail(w, http.StatusNotFound, "Not found")
	}
}

func (a *app) action(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	switch req.Action {
	case "add":
		a.create(w, r, req, me)
	case "list":
		a.list(w, r)
	case "open", "show", "set", "close", "share", "join", "participant", "block", "results", "answer":
		it := a.loadOr404(w, req.ID)
		if it == nil {
			return
		}
		if a.canView(r, it) {
			var done func()
			var ok bool
			r, done, ok = a.knownParticipantLocked(w, r, it)
			if !ok {
				return
			}
			defer done()
			it = a.items[it.ID]
		}
		switch req.Action {
		case "open":
			if !a.viewOr403(w, r, it) {
				return
			}
			if it.JoinCode == "" && a.isOwner(r, it) {
				next := it.clone()
				code, err := a.assignCodeLocked("")
				next.JoinCode = code
				if err != nil || a.saveLocked(&next, "") != nil {
					kit.Fail(w, http.StatusInternalServerError, "Could not assign a join code")
					return
				}
				it = &next
			}
			kit.WriteJSON(w, a.openView(r, it))
		case "show":
			if a.viewOr403(w, r, it) {
				a.show(w, r, it)
			}
		case "set", "close":
			a.mutate(w, r, it, req, req.Action == "close")
		case "share":
			a.share(w, r, it)
		case "join":
			a.join(w, r, it, req, me)
		case "participant":
			a.participantAction(w, r, it, req)
		case "block":
			if req.Kind == "poll" || req.Kind == "form" {
				a.blockAdd(w, r, it, req)
			} else {
				a.blockSet(w, r, it, req)
			}
		case "results":
			a.blockResults(w, r, it, req)
		case "answer":
			if a.viewOr403(w, r, it) {
				a.answer(w, r, it, req)
			}
		}
	default:
		kit.Fail(w, http.StatusBadRequest, "Unknown action")
	}
}

// checkContent validates a body and refuses secrets before anything is
// stored.
func checkContent(w http.ResponseWriter, req actionRequest) bool {
	if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
		kit.Fail(w, http.StatusBadRequest, kit.ErrSecret)
		return false
	}
	if req.Audience != "" && !validAudience(req.Audience) {
		kit.Fail(w, http.StatusBadRequest, "Audience must be host, lan or satellite")
		return false
	}
	if _, err := parseBody(req.Body); err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}

// titleOf is the first-level heading of body, or "".
func titleOf(body string) string {
	for _, l := range strings.Split(body, "\n") {
		if t, ok := strings.CutPrefix(strings.TrimSpace(l), "# "); ok {
			return strings.TrimSpace(t)
		}
	}
	return ""
}

func (a *app) create(w http.ResponseWriter, r *http.Request, req actionRequest, me string) {
	if !checkContent(w, req) {
		return
	}
	id := req.ID
	if id == "" {
		id = "page-" + kit.NewToken()[:12]
	}
	if !kit.ValidName(id) {
		kit.Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for an id")
		return
	}
	if _, ok := a.items[id]; ok {
		kit.Fail(w, http.StatusConflict, "A page with that id already exists")
		return
	}
	if _, err := os.Stat(itemPath(a.dir, id)); err == nil {
		kit.Fail(w, http.StatusConflict, "A page with that id already exists")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = titleOf(req.Body)
	}
	if title == "" {
		title = "Untitled page"
	}
	audience := req.Audience
	if audience == "" {
		audience = audienceHost
	}
	status := kit.StatusPending
	if strings.TrimSpace(req.Body) != "" {
		status = kit.StatusFilled
	}
	code, err := a.assignCodeLocked(req.JoinCode)
	if err != nil {
		if errors.Is(err, errJoinCodeInUse) {
			kit.Fail(w, http.StatusConflict, err.Error())
		} else {
			kit.Fail(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	it := &item{Version: 1, ID: id, Title: title, Body: req.Body, Audience: audience, JoinCode: code,
		Participants: participantMap{}, Status: status, Owner: kit.NewToken(),
		Members: map[string]bool{me: true}, Invites: map[string]bool{}}
	if err := a.saveLocked(it, ""); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, it)
	v["ownerKey"] = it.Owner // revealed once, at creation
	kit.WriteJSON(w, v)
}

// show is the one read that returns content: the Markdown, its blocks, the
// audience, participants by handle and auth, and the revision.
func (a *app) show(w http.ResponseWriter, r *http.Request, it *item) {
	kit.WriteJSON(w, map[string]any{"id": it.ID, "url": a.urlFor(r, it.ID),
		"status": it.Status, "title": it.Title, "body": it.Body,
		"blocks": blocksOf(it.Body), "audience": it.Audience,
		"participants": exportedParticipants(it), "revision": it.Revision})
}

func (a *app) mutate(w http.ResponseWriter, r *http.Request, it *item, req actionRequest, closing bool) {
	if !a.isOwner(r, it) {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can change this page")
		return
	}
	if it.Status == kit.StatusClosed {
		kit.Fail(w, http.StatusConflict, "This page is closed")
		return
	}
	next := it.clone()
	if closing {
		next.Status = kit.StatusClosed
	} else {
		if !checkContent(w, req) {
			return
		}
		if req.Body != "" {
			next.Body = req.Body
		}
		if strings.TrimSpace(req.Title) != "" {
			next.Title = strings.TrimSpace(req.Title)
		}
		if req.Audience != "" {
			next.Audience = req.Audience
		}
		next.Status = kit.StatusFilled
	}
	if err := a.saveLocked(&next, ""); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(r, &next))
}

func (a *app) list(w http.ResponseWriter, r *http.Request) {
	names, err := filepath.Glob(filepath.Join(a.dir, "items", "*.json"))
	if err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not list")
		return
	}
	out := []map[string]any{}
	for _, p := range names {
		it, err := a.loadLocked(strings.TrimSuffix(filepath.Base(p), ".json"))
		if err != nil || !a.canView(r, it) {
			continue
		}
		out = append(out, a.openView(r, it)) // ids + statuses, never content
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	kit.WriteJSON(w, out)
}

func (a *app) share(w http.ResponseWriter, r *http.Request, it *item) {
	if !a.isOwner(r, it) {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can share this page")
		return
	}
	next := it.clone()
	code := kit.NewToken()
	next.Invites[code] = true
	if err := a.saveLocked(&next, ""); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, &next)
	v["audience"] = next.Audience
	v["invite"] = code
	v["inviteURL"] = a.urlFor(r, next.ID) + "#invite=" + code
	kit.WriteJSON(w, v)
}

// join redeems a single-use invite into a member (the join story adds the
// handle and the participant record).
func (a *app) join(w http.ResponseWriter, r *http.Request, it *item, req actionRequest, me string) {
	if !kit.ValidToken(req.Invite) || !it.Invites[req.Invite] {
		kit.Fail(w, http.StatusForbidden, "This invite is invalid. Ask the organizer for a new one.")
		return
	}
	next := it.clone()
	next.Members[me] = true
	delete(next.Invites, req.Invite)
	if err := a.saveLocked(&next, ""); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(r, &next))
}

// export is the organizer's record: md is the Markdown with each block as a
// labelled line, html the rendered page. Participants appear by handle and
// auth only; addresses and fingerprints never leave the store.
func (a *app) export(w http.ResponseWriter, r *http.Request, id, to string) {
	it := a.loadOr404(w, id)
	if it == nil {
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can export this page")
		return
	}
	_, done, ok := a.knownParticipantLocked(w, r, it)
	if !ok {
		return
	}
	defer done()
	switch to {
	case "md", "markdown":
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		io.WriteString(w, a.exportMarkdownLocked(it))
	case "html":
		body, err := renderPage(it, func(b *block) string { return a.blockFace(r, it, b) })
		if err != nil {
			kit.Fail(w, http.StatusInternalServerError, "Could not render")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(kit.Page(tool, it.Title, `<main class="desk-main">`+body+participantsHTML(it)+`</main>`))
	default:
		kit.Fail(w, http.StatusBadRequest, "Export to md or html (--to md|html)")
	}
}

func exportMarkdown(it *item) string { return exportMarkdownWith(it, blockMarkdown) }

func (a *app) exportMarkdownLocked(it *item) string {
	return exportMarkdownWith(it, func(b *block) string { return a.blockMarkdownLocked(it, b) })
}

func exportMarkdownWith(it *item, blockMD func(*block) string) string {
	var b strings.Builder
	if titleOf(it.Body) == "" {
		b.WriteString("# " + it.Title + "\n\n")
	}
	segs, _ := parseBody(it.Body)
	for _, s := range segs {
		if s.Block != nil {
			b.WriteString(blockMD(s.Block))
		} else {
			b.WriteString(s.Text)
		}
	}
	if !strings.HasSuffix(b.String(), "\n") {
		b.WriteString("\n")
	}
	if rows := exportedParticipants(it); len(rows) > 0 {
		b.WriteString("\n## Participants\n\n")
		for _, p := range rows {
			b.WriteString("- " + escapeParticipantMarkdown(p.Handle) + " (" + p.Auth + ")\n")
		}
		if breakMode == "export-addresses" {
			for _, p := range it.Participants {
				if p != nil {
					b.WriteString(p.Address + "\n")
				}
			}
		}
	}
	return b.String()
}

func participantsHTML(it *item) string {
	rows := exportedParticipants(it)
	if len(rows) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<section class="desk-panel"><h2 class="desk-h2">Participants</h2><ul>`)
	for _, p := range rows {
		b.WriteString(`<li>` + html.EscapeString(p.Handle) + ` <span class="desk-muted">` + html.EscapeString(p.Auth) + `</span></li>`)
	}
	b.WriteString(`</ul></section>`)
	return b.String()
}

// renderPage renders the body: Markdown runs through the kit renderer,
// block fences become their block's place on the page.
func renderPage(it *item, blockFn func(*block) string) (string, error) {
	segs, err := parseBody(it.Body)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, s := range segs {
		if s.Block != nil {
			b.WriteString(blockFn(s.Block))
			continue
		}
		out, err := renderMarkdown([]byte(s.Text))
		if err != nil {
			return "", err
		}
		b.Write(out)
	}
	return b.String(), nil
}

// page renders an item after the human door has resolved its identity.
func (a *app) page(r *http.Request) (int, []byte) {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := r.URL.Query().Get("id")
	code := http.StatusOK
	header := kit.EmptyItemHeader("page")
	var main string
	openForm := `<form method="get" action="` + html.EscapeString(a.base(r)+"/") + `" class="desk-panel">` +
		kit.TextField("id", "Page", "text", "", "") +
		`<div class="desk-row"><button type="submit" class="desk-copy">Open</button></div></form>`
	it, err := a.loadLocked(id)
	switch {
	case id == "":
		main = kit.State("empty", "No page open", "Open the link the organizer shared, or type the page here.") + openForm
	case err != nil:
		code = http.StatusNotFound
		main = kit.State("error", "Page not found", "Check the link with the organizer.") + openForm
	case !a.canView(r, it):
		code = http.StatusForbidden
		main = kit.State("error", "This page is not shared with your network", "Ask the organizer for the link.")
	default:
		header = kit.ItemHeader(it.ID, it.Status, a.urlFor(r, it.ID))
		body, err := renderPage(it, func(b *block) string { return a.blockFace(r, it, b) })
		if err != nil {
			body = kit.State("error", "This page cannot be shown", err.Error())
		}
		main = `<h1 class="desk-h1">` + html.EscapeString(it.Title) + `</h1>` + body +
			`<p class="desk-footer">Revision <span data-page-revision>` + fmt.Sprint(it.Revision) +
			`</span> · re-surface with <code>page open ` + html.EscapeString(it.ID) + `</code>.</p>` +
			liveScript(a.base(r), it.ID) + nativeScript(a.base(r), it.ID)
	}
	body := `<style>.page-block{border-style:dashed}</style>` +
		kit.AppBar(tool, a.base(r)+"/", header, "") + `<main class="desk-main desk-narrow">` +
		`<div data-desk-alert class="desk-alert"></div>` + main + `</main>`
	return code, kit.Page(tool, "Page · bashy desk", body)
}

// liveScript follows the page's event stream. The skeleton reloads on a
// whole-page frame; block renderers re-fetch only their own block.
func liveScript(base, id string) string {
	u, _ := json.Marshal(base + "/api/events?id=" + id)
	return `<script>
(function(){
  if (!window.EventSource) return;
  const rev = document.querySelector('[data-page-revision]');
  const es = new EventSource(` + string(u) + `);
  es.onmessage = function(e){
    let f; try { f = JSON.parse(e.data); } catch (_) { return; }
    if (!rev || f.revision <= Number(rev.textContent)) return;
    if (!f.block) { location.reload(); return; }
    document.dispatchEvent(new CustomEvent('page-block', {detail: f}));
  };
})();
</script>`
}

// bindGuard is the start-up listen guard: loopback only, unless the
// explicit --lan override is given, which kit.CheckBind logs to logw.
func bindGuard(addr string, lan bool, logw io.Writer) error {
	if lan {
		_, port, err := net.SplitHostPort(addr)
		if err != nil || !fixedPort(port) {
			return fmt.Errorf("page serve --lan needs a fixed port (1–65535)")
		}
	}
	return kit.CheckBind(addr, lan, logw)
}

func serve(addr, data, prefix string, lan, suffix bool) {
	if err := bindGuard(addr, lan, os.Stderr); err != nil {
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
	a := &app{dir: data, prefix: prefix, baseURL: origin + prefix, items: map[string]*item{},
		formURL: os.Getenv("PAGE_FORM_URL"),
		joinOptions: pageJoinOptions{consolePrincipal: processConsolePrincipal(addr), outpostSecret: pageOutpostSecret(),
			suffix: suffix}}
	if lan {
		host, _ := os.Hostname()
		a.lanBase = lanJoinBase(a.baseURL, host)
	}
	fmt.Printf("\nDesk page — keep this terminal running\nOrganizer: %s/\nSaved pages: %s\nCtrl-C stops the server.\n\n",
		a.baseURL, filepath.Join(data, "items"))
	// kit.Events sets a write deadline per frame, so event streams outlive
	// WriteTimeout.
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
	os.Exit(runCLI(os.Args[1:], os.Stdin, os.Stdout))
}
