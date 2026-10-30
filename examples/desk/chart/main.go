package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

	"desk-chart/fence"
	kit "desk-kit"
	"desk-kit/render"
)

func init() {
	// Markdown ```chart fences served by this tool reuse the C1 renderer.
	render.ChartSVG = fence.RenderFence
}

// renderMarkdown serves POST /api/render: Markdown in, sanitized HTML out.
// There is no item context here, so local embeds degrade to links, remote
// embeds stay off, and chart fences render through the init hook above.
func renderMarkdown(src []byte) ([]byte, error) {
	return render.Markdown(src, render.Options{})
}

type request struct {
	Action string       `json:"action"`
	ID     string       `json:"id"`
	Title  string       `json:"title"`
	Body   string       `json:"body"`
	Fields []kit.Field  `json:"fields"`
	Invite string       `json:"invite"`
	Config *chartConfig `json:"chart,omitempty"`
}

type item struct {
	Chart    chartConfig     `json:"chart"`
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
	baseURL string // public origin, e.g. http://127.0.0.1:18098
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

// base is the public URL of the tool root for r: its own address, or the app
// shell mount it was reached through (kit.PublicBase).
func (a *app) base(r *http.Request) string { return kit.PublicBase(r, a.baseURL) }

func (a *app) urlFor(r *http.Request, id string) string {
	return kit.ItemURL(a.base(r)+"/", "id", id)
}

// openView is the contentless re-surface: id + stable URL + status only.
func (a *app) openView(r *http.Request, it *item) map[string]any {
	v := kit.OpenView(it.ID, a.urlFor(r, it.ID), it.Status)

	return v
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ContractHandler("chart", http.HandlerFunc(a.serveHTTP)).ServeHTTP(w, r)
}

func (a *app) serveHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ApplySecurityHeaders(w)
	if !kit.GuardPOST(w, r, "chart") {
		return
	}
	path, ok := kit.StripPrefix(a.prefix, r.URL.Path)
	if !ok {
		kit.Fail(w, 404, "Not found")
		return
	}
	if path == "/" && r.Method == "GET" {
		me := kit.ParticipantCookie(w, r, "chart-participant")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(a.page(r, me))
		return
	}
	if path == "/healthz" && r.Method == "GET" {
		kit.WriteJSON(w, map[string]bool{"ok": true})
		return
	}
	if path == "/api/state" && r.Method == "GET" {
		a.mu.Lock()
		defer a.mu.Unlock()
		kit.ParticipantCookie(w, r, "chart-participant")
		id := r.URL.Query().Get("id")
		it, err := a.loadLocked(id)
		if err != nil {
			if os.IsNotExist(err) {
				kit.Fail(w, 404, "Item not found. Create it with chart add first.")
			} else {
				kit.Fail(w, 400, err.Error())
			}
			return
		}
		kit.WriteJSON(w, a.openView(r, it))
		return
	}
	if path == "/api/render" {
		kit.ServeRender(w, r, renderMarkdown)
		return
	}
	if path != "/api/action" || r.Method != "POST" {
		kit.Fail(w, 404, "Not found")
		return
	}
	var req request
	if !decodeRequest(w, r, &req) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	me := kit.ParticipantCookie(w, r, "chart-participant")
	switch req.Action {
	case "add":
		a.create(w, r, req, me)
	case "open":
		a.open(w, r, req)
	case "show":
		a.get(w, r, req, me)
	case "set":
		a.mutate(w, r, req, me, false)
	case "close":
		a.mutate(w, r, req, me, true)
	case "list":
		a.list(w, r)
	case "share":
		a.share(w, r, req)
	case "revoke":
		a.revoke(w, r, req)
	case "join":
		a.join(w, r, req, me)
	default:
		kit.Fail(w, 400, "Unknown action")
	}
}

func (a *app) create(w http.ResponseWriter, r *http.Request, req request, me string) {
	if kit.SecretRefused(req.Fields) {
		kit.Fail(w, 400, kit.ErrSecret)
		return
	}
	id := req.ID
	if id == "" {
		id = "chart-" + kit.NewToken()[:12]
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
		title = "Untitled chart"
	}
	it := &item{Version: 1, ID: id, Title: title, Body: req.Body,
		Fields: req.Fields, Status: kit.StatusPending, Owner: kit.NewToken(),
		Members: map[string]bool{me: true}, Invites: map[string]bool{}}
	if req.Config != nil {
		if err := prepareConfig(req.Config); err != nil {
			kit.Fail(w, 400, err.Error())
			return
		}
		it.Chart = *req.Config
		it.Chart.Title = it.Title
	}
	if err := a.saveLocked(it); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, it)
	v["ownerKey"] = it.Owner // revealed once, at creation, like a facilitator key
	kit.WriteJSON(w, v)
}

func (a *app) open(w http.ResponseWriter, r *http.Request, req request) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with chart add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	kit.WriteJSON(w, a.openView(r, it))
}

func (a *app) get(w http.ResponseWriter, r *http.Request, req request, me string) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with chart add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) && !it.Members[me] {
		kit.Fail(w, 403, "Join this chart first; ask the owner for an invite.")
		return
	}
	kit.WriteJSON(w, a.viewData(r, it))
}

func (a *app) mutate(w http.ResponseWriter, r *http.Request, req request, me string, closing bool) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with chart add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can change this chart")
		return
	}
	if it.Status == kit.StatusClosed {
		kit.Fail(w, 409, "This chart is closed")
		return
	}
	next := cloneItem(it)
	if closing {
		next.Status = kit.StatusClosed
	} else {
		if kit.SecretRefused(req.Fields) {
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
		if req.Config != nil {
			if err := prepareConfig(req.Config); err != nil {
				kit.Fail(w, 400, err.Error())
				return
			}
			next.Chart = *req.Config
		}
		next.Chart.Title = next.Title
		next.Status = kit.StatusFilled
	}
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(r, &next))
}

func (a *app) list(w http.ResponseWriter, r *http.Request) {
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
		out = append(out, a.openView(r, it)) // ids + statuses, never content
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	kit.WriteJSON(w, out)
}

func (a *app) share(w http.ResponseWriter, r *http.Request, req request) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with chart add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can share this chart")
		return
	}
	next := cloneItem(it)
	if next.Invites == nil {
		next.Invites = map[string]bool{}
	}
	code := kit.NewToken()
	next.Invites[code] = true
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, &next)
	v["invite"] = code
	v["inviteURL"] = a.urlFor(r, next.ID) + "#invite=" + code
	kit.WriteJSON(w, v)
}

func (a *app) join(w http.ResponseWriter, r *http.Request, req request, me string) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Item not found. Create it with chart add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !kit.ValidToken(req.Invite) || !it.Invites[req.Invite] {
		kit.Fail(w, 403, "This invite is invalid. Ask the owner for a new one.")
		return
	}
	next := cloneItem(it)
	next.Members[me] = true
	delete(next.Invites, req.Invite) // single use
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(r, &next))
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
	fmt.Printf("\nDesk chart — keep this terminal running\nParticipant: %s/\nSaved charts: %s\nCtrl-C stops the server.\n\n",
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

func cloneItem(it *item) item {
	next := *it
	next.Members = map[string]bool{}
	for k, v := range it.Members {
		next.Members[k] = v
	}
	next.Invites = map[string]bool{}
	for k, v := range it.Invites {
		next.Invites[k] = v
	}
	return next
}

func (a *app) revoke(w http.ResponseWriter, r *http.Request, req request) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		kit.Fail(w, 404, "Item not found")
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can revoke sharing")
		return
	}
	next := cloneItem(it)
	next.Invites = map[string]bool{}
	next.Members = map[string]bool{}
	if err = a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save")
		return
	}
	kit.WriteJSON(w, a.openView(r, &next))
}

// K0 DecodeAction does not reject a second valid JSON value. Keep this guard
// local until the kit can tighten its decoder without changing other tools.
func decodeRequest(w http.ResponseWriter, r *http.Request, req *request) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(req); err != nil {
		kit.Fail(w, 400, "Invalid request")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		kit.Fail(w, 400, "Expected one JSON object")
		return false
	}
	return true
}
