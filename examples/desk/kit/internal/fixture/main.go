// Command memo is the desk-contract fixture tool: the smallest possible
// desk item server (named memos) that the shared contract test runs green
// against. Real tools (notepad, form, chart) copy this shape; the R1
// retrofit copies only the contract points into whiteboard/brainstorm
// without changing their behaviour.
//
// Doors: `memo new|open|get|put|list|share|close [--json]` (agent) and the
// loopback page (human). Storage: $BASHY_HOME/desk/memo/items/<id>.json.
//
// The -break flag deliberately violates one contract point at a time so the
// contract test can prove it goes red:
//
//	-break open-emits-content   open re-emits item content
//	-break no-csp                security headers are dropped
//	-break allow-secret          secret fields are stored
package main

import (
	"context"
	"encoding/json"
	"fmt"
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

type item struct {
	Version  int             `json:"version"`
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
	Fields   []kit.Field     `json:"fields"`
	Status   string          `json:"status"`
	Revision int             `json:"revision"`
	Owner    string          `json:"owner"`
	Members  map[string]bool `json:"members"`
	Invites  map[string]bool `json:"invites"`
}

type app struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	baseURL string // public origin, e.g. http://127.0.0.1:18097
	items   map[string]*item
}

// clone deep-copies the item so a failed save never leaves the in-memory
// copy mutated.
func (it *item) clone() item {
	c := *it
	c.Fields = append([]kit.Field(nil), it.Fields...)
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
		return nil, fmt.Errorf("saved item %q is invalid; preserve it for recovery", id)
	}
	if it.Version != 1 || it.ID != id || !kit.ValidStatus(it.Status) {
		return nil, fmt.Errorf("unsupported or invalid saved item %q", id)
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
	v := kit.OpenView(it.ID, a.urlFor(it.ID), it.Status)
	if breakMode == "open-emits-content" {
		v["body"] = it.Body // deliberate contract break for red/green proof
	}
	return v
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if breakMode != "no-csp" {
		kit.ApplySecurityHeaders(w)
	}
	if !kit.GuardPOST(w, r, "memo") {
		return
	}
	path, ok := kit.StripPrefix(a.prefix, r.URL.Path)
	if !ok {
		kit.Fail(w, 404, "Not found")
		return
	}
	if path == "/" && r.Method == "GET" {
		kit.ParticipantCookie(w, r, "memo-participant")
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
		kit.ParticipantCookie(w, r, "memo-participant")
		id := r.URL.Query().Get("id")
		it, err := a.loadLocked(id)
		if err != nil {
			if os.IsNotExist(err) {
				kit.Fail(w, 404, "Item not found. Create it with memo new first.")
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
	var req struct {
		Action string      `json:"action"`
		ID     string      `json:"id"`
		Title  string      `json:"title"`
		Body   string      `json:"body"`
		Fields []kit.Field `json:"fields"`
		Invite string      `json:"invite"`
	}
	if !kit.DecodeAction(w, r, 65536, &req) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	me := kit.ParticipantCookie(w, r, "memo-participant")
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

func (a *app) create(w http.ResponseWriter, req struct {
	Action string      `json:"action"`
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Body   string      `json:"body"`
	Fields []kit.Field `json:"fields"`
	Invite string      `json:"invite"`
}, me string) {
	if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
		kit.Fail(w, 400, kit.ErrSecret)
		return
	}
	id := req.ID
	if id == "" {
		id = "memo-" + kit.NewToken()[:12]
	}
	if !kit.ValidName(id) {
		kit.Fail(w, 400, "Use 1-64 letters, digits, - or _ for an id")
		return
	}
	if _, ok := a.items[id]; ok {
		kit.Fail(w, 409, "An item with that id already exists")
		return
	}
	if _, err := os.Stat(itemPath(a.dir, id)); err == nil {
		kit.Fail(w, 409, "An item with that id already exists")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Untitled memo"
	}
	it := &item{Version: 1, ID: id, Title: title, Body: req.Body,
		Fields: req.Fields, Status: kit.StatusPending, Owner: kit.NewToken(),
		Members: map[string]bool{me: true}, Invites: map[string]bool{}}
	if err := a.saveLocked(it); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(it)
	v["ownerKey"] = it.Owner // revealed once, at creation, like a facilitator key
	kit.WriteJSON(w, v)
}

func (a *app) open(w http.ResponseWriter, req struct {
	Action string      `json:"action"`
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Body   string      `json:"body"`
	Fields []kit.Field `json:"fields"`
	Invite string      `json:"invite"`
}) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with memo new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	kit.WriteJSON(w, a.openView(it))
}

func (a *app) get(w http.ResponseWriter, r *http.Request, req struct {
	Action string      `json:"action"`
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Body   string      `json:"body"`
	Fields []kit.Field `json:"fields"`
	Invite string      `json:"invite"`
}, me string) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with memo new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) && !it.Members[me] {
		kit.Fail(w, 403, "Join this memo first; ask the owner for an invite.")
		return
	}
	kit.WriteJSON(w, map[string]any{"id": it.ID, "url": a.urlFor(it.ID),
		"status": it.Status, "title": it.Title, "body": it.Body,
		"fields": it.Fields, "revision": it.Revision})
}

func (a *app) mutate(w http.ResponseWriter, r *http.Request, req struct {
	Action string      `json:"action"`
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Body   string      `json:"body"`
	Fields []kit.Field `json:"fields"`
	Invite string      `json:"invite"`
}, me string, closing bool) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with memo new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can change this memo")
		return
	}
	if it.Status == kit.StatusClosed {
		kit.Fail(w, 409, "This memo is closed")
		return
	}
	next := it.clone()
	if closing {
		next.Status = kit.StatusClosed
	} else {
		if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
			kit.Fail(w, 400, kit.ErrSecret)
			return
		}
		if req.Body != "" {
			next.Body = req.Body
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

func (a *app) share(w http.ResponseWriter, r *http.Request, req struct {
	Action string      `json:"action"`
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Body   string      `json:"body"`
	Fields []kit.Field `json:"fields"`
	Invite string      `json:"invite"`
}) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with memo new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can share this memo")
		return
	}
	next := it.clone()
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

func (a *app) join(w http.ResponseWriter, req struct {
	Action string      `json:"action"`
	ID     string      `json:"id"`
	Title  string      `json:"title"`
	Body   string      `json:"body"`
	Fields []kit.Field `json:"fields"`
	Invite string      `json:"invite"`
}, me string) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with memo new first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !kit.ValidToken(req.Invite) || !it.Invites[req.Invite] {
		kit.Fail(w, 403, "This invite is invalid. Ask the owner for a new one.")
		return
	}
	next := it.clone()
	next.Members[me] = true
	delete(next.Invites, req.Invite) // single use
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(&next))
}

// page renders the human door: item header, status badges, copy buttons,
// field components and a theme toggle, all from the kit snippets.
func (a *app) page(r *http.Request) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := r.URL.Query().Get("id")
	header := `<div class="desk-item-header"><span class="desk-id">memo</span>` +
		kit.Badge(kit.StatusPending) + `<span class="desk-tools"></span></div>`
	if id != "" {
		if it, err := a.loadLocked(id); err == nil {
			header = kit.ItemHeader(it.ID, it.Status, a.urlFor(it.ID))
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
		kit.TextField("title", "Title", "text", "", "") +
		kit.TextField("body", "Body", "textarea", "", "recommended") +
		`<div style="display:flex;gap:8px">` +
		kit.CopyButton("Copy link", a.baseURL+"/") +
		`<button type="button" class="desk-copy" data-desk-theme>Toggle theme</button></div>` +
		`<p class="desk-muted">Local-first memo · loopback only · re-surface with <code>memo open ID</code>.</p></main>`
	return kit.Shell("Memo · desk fixture", body)
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
	a := &app{dir: data, prefix: prefix, baseURL: origin + prefix, items: map[string]*item{}}
	fmt.Printf("\nDesk memo fixture — keep this terminal running\nParticipant: %s/\nSaved memos: %s\nCtrl-C stops the server.\n\n",
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
