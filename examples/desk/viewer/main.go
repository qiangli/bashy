// Command viewer is the bashy desk viewer: a VIEW-ONLY reader for reports,
// recaps, plans and documents that agents write for humans.
//
// Agents never write HTML. They give a Markdown body, or JSON data for a
// shipped template (status, plan-summary, recap, close-out); the viewer
// fills the template, the kit renderer (desk-kit/render) produces sanitized
// HTML, and a human reads it in the loopback page. Items are books: ordered
// pages per sprint (sprint-N, survives manager handoffs) or per session,
// with first/prev/next/last, an index, jump-to-page and a "changed since
// previous" highlight. Nothing is ever edited: set/edit are refused (exit 2
// / 400); a correction is the next page. Humans edit text in notepad.
//
// Doors: `viewer add|show|list|open|share|close|append|export [--json]`
// (agent) and the loopback page (human), one JSON API under both.
// Storage: $BASHY_HOME/desk/viewer/items/<id>.json (+ <id>/assets/<page>/).
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	kit "desk-kit"
	"desk-kit/render"

	"desk-chart/fence"
)

func init() {
	// ```chart fences in books reuse the C1 renderer, as in notepad.
	render.ChartSVG = fence.RenderFence
}

// renderMarkdown serves POST /api/render for kit.js deskRender: Markdown in,
// sanitized HTML out, with no item assets.
func renderMarkdown(src []byte) ([]byte, error) {
	return render.Markdown(src, render.Options{})
}

const tool = "viewer"

// breakMode is set from -break; empty means contract-clean. The red tests
// use: accept-set, open-emits-content, allow-secret, no-escape, no-validate.
var breakMode string

type assetIn struct {
	Name string `json:"name"`
	Data string `json:"data"` // base64
}

type actionRequest struct {
	Action   string          `json:"action"`
	ID       string          `json:"id"`
	Title    string          `json:"title"`
	Body     string          `json:"body"`
	Template string          `json:"template,omitempty"`
	Data     json.RawMessage `json:"data,omitempty"`
	Sprint   int             `json:"sprint,omitempty"`
	Session  string          `json:"session,omitempty"`
	Page     string          `json:"page,omitempty"`
	Invite   string          `json:"invite,omitempty"`
	Fields   []kit.Field     `json:"fields,omitempty"`
	Assets   []assetIn       `json:"assets,omitempty"`
}

type app struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	baseURL string // public origin + prefix, e.g. http://127.0.0.1:18096
}

// httpError carries a status with a user-facing message.
type httpError struct {
	code int
	msg  string
}

func (e *httpError) Error() string { return e.msg }

func fail(w http.ResponseWriter, err error) {
	if he, ok := err.(*httpError); ok {
		kit.Fail(w, he.code, he.msg)
		return
	}
	kit.Fail(w, http.StatusBadRequest, err.Error())
}

// base is the public URL of the tool root for r: its own address, or the app
// shell mount it was reached through (kit.PublicBase).
func (a *app) base(r *http.Request) string { return kit.PublicBase(r, a.baseURL) }

func (a *app) urlFor(r *http.Request, id string) string {
	return kit.ItemURL(a.base(r)+"/", "id", id)
}

func (a *app) pageURL(r *http.Request, id string, n int) string {
	return a.urlFor(r, id) + "&page=" + strconv.Itoa(n)
}

// openView is the contentless re-surface: id + stable URL + status (+ the
// page count) only.
func (a *app) openView(r *http.Request, b *book) map[string]any {
	v := kit.OpenView(b.ID, a.urlFor(r, b.ID), b.Status)
	v["pages"] = len(b.Pages)
	if breakMode == "open-emits-content" && len(b.Pages) > 0 {
		v["markdown"] = b.Pages[0].Markdown // deliberate contract break
	}
	return v
}

func (a *app) load(id string) (*book, error) {
	b, err := readBook(a.dir, id)
	if err == os.ErrNotExist {
		return nil, &httpError{http.StatusNotFound, "Book not found. Create it with viewer add first."}
	}
	if err != nil {
		return nil, &httpError{http.StatusBadRequest, err.Error()}
	}
	return b, nil
}

func (a *app) save(b *book) error {
	b.Revision++
	b.Updated = now()
	raw, err := kit.MarshalIndent(*b)
	if err != nil {
		return err
	}
	return kit.AtomicWrite(bookPath(a.dir, b.ID), raw)
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// local is a caller on this host talking to the server directly (no
// loopback proxy in between): the agent's CLI and the operator's browser.
func local(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	return err == nil && net.ParseIP(host).IsLoopback() &&
		r.Header.Get("X-Forwarded-For") == "" && r.Header.Get("X-Forwarded-Host") == ""
}

func (a *app) canRead(r *http.Request, b *book, me string) bool {
	return local(r) || kit.Grant(kit.FacilitatorKey(r), b.Owner) || b.Members[me]
}

func (a *app) canWrite(r *http.Request, b *book) bool {
	return local(r) || kit.Grant(kit.FacilitatorKey(r), b.Owner)
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ContractHandler(tool, http.HandlerFunc(a.serveHTTP)).ServeHTTP(w, r)
}

func (a *app) serveHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ApplySecurityHeaders(w)
	if !kit.GuardPOST(w, r, tool) {
		return
	}
	path, ok := kit.StripPrefix(a.prefix, r.URL.Path)
	if !ok {
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}
	switch {
	case path == "/" && r.Method == "GET":
		me := kit.ParticipantCookie(w, r, "viewer-participant")
		code, body := a.page(r, me)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(code)
		w.Write(body)
		return
	case path == "/api/render":
		kit.ServeRender(w, r, renderMarkdown)
		return
	case path == "/healthz" && r.Method == "GET":
		kit.WriteJSON(w, map[string]bool{"ok": true})
		return
	case path == "/api/state" && r.Method == "GET":
		a.mu.Lock()
		defer a.mu.Unlock()
		b, err := a.load(r.URL.Query().Get("id"))
		if err != nil {
			fail(w, err)
			return
		}
		kit.WriteJSON(w, a.openView(r, b))
		return
	case path == "/api/export" && r.Method == "GET":
		me := kit.ParticipantCookie(w, r, "viewer-participant")
		a.mu.Lock()
		defer a.mu.Unlock()
		b, err := a.load(r.URL.Query().Get("id"))
		if err != nil {
			fail(w, err)
			return
		}
		if !a.canRead(r, b, me) {
			kit.Fail(w, http.StatusForbidden, "Join this book first; ask the owner for an invite.")
			return
		}
		out, name, err := a.exportHTML(r, b, r.URL.Query().Get("page"))
		if err != nil {
			fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
		w.Write(out)
		return
	case strings.HasPrefix(path, "/assets/") && r.Method == "GET":
		a.asset(w, r, strings.TrimPrefix(path, "/assets/"))
		return
	case path == "/api/action" && r.Method == "POST":
	case path == "/api/action":
		kit.Fail(w, http.StatusMethodNotAllowed, "Method not supported")
		return
	default:
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}

	var req actionRequest
	if !kit.DecodeAction(w, r, 16<<20, &req) {
		return
	}
	me := kit.ParticipantCookie(w, r, "viewer-participant")
	a.mu.Lock()
	defer a.mu.Unlock()
	var v map[string]any
	var err error
	switch req.Action {
	case "add":
		v, err = a.add(r, req, me)
	case "append":
		v, err = a.appendPage(r, req)
	case "open":
		v, err = a.open(r, req)
	case "show":
		v, err = a.show(r, req, me)
	case "list":
		items, lerr := a.list(r)
		if lerr != nil {
			fail(w, lerr)
			return
		}
		kit.WriteJSON(w, items)
		return
	case "share":
		v, err = a.share(r, req)
	case "close":
		v, err = a.close(r, req)
	case "join":
		v, err = a.join(r, req, me)
	case "export":
		v, err = a.export(r, req, me)
	case "set", "edit", "put":
		if breakMode == "accept-set" {
			v, err = a.open(r, req) // deliberate contract break: pretends to edit
			break
		}
		err = &httpError{http.StatusBadRequest, "viewer is view-only: pages are never edited. Append a new page with viewer append, or edit the text in notepad."}
	default:
		err = &httpError{http.StatusBadRequest, "Unknown action"}
	}
	if err != nil {
		fail(w, err)
		return
	}
	kit.WriteJSON(w, v)
}

func (a *app) add(r *http.Request, req actionRequest, me string) (map[string]any, error) {
	if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
		return nil, &httpError{http.StatusBadRequest, kit.ErrSecret}
	}
	if req.Session != "" && !kit.ValidName(req.Session) {
		return nil, &httpError{http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for a session"}
	}
	if req.Sprint < 0 {
		return nil, &httpError{http.StatusBadRequest, "sprint must be a positive number"}
	}
	id, title := req.ID, strings.TrimSpace(req.Title)
	switch {
	case id != "":
	case req.Sprint > 0:
		id = "sprint-" + strconv.Itoa(req.Sprint)
	case req.Session != "":
		id = "session-" + req.Session
	default:
		id = "book-" + kit.NewToken()[:12]
	}
	if !kit.ValidName(id) {
		return nil, &httpError{http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for an id"}
	}
	if _, err := os.Stat(bookPath(a.dir, id)); err == nil {
		return nil, &httpError{http.StatusConflict, "A book with that id already exists; add a page with viewer append " + id}
	}
	if title == "" {
		switch {
		case req.Sprint > 0:
			title = "Sprint " + strconv.Itoa(req.Sprint)
		case req.Session != "":
			title = "Session " + req.Session
		default:
			title = "Untitled book"
		}
	}
	t := now()
	b := &book{
		Version: 1, ID: id, Title: title, Sprint: req.Sprint, Session: req.Session,
		Status: kit.StatusPending, Created: t, Owner: kit.NewToken(),
		Members: map[string]bool{me: true}, Invites: map[string]bool{}, Fields: req.Fields,
		Pages: []page{},
	}
	if req.Body != "" || req.Template != "" {
		req.Title = "" // the book title is not the first page's title
		if err := a.addPage(b, req); err != nil {
			return nil, err
		}
	}
	if err := a.save(b); err != nil {
		return nil, &httpError{http.StatusInternalServerError, "Could not save. Your change was not accepted."}
	}
	v := a.openView(r, b)
	v["ownerKey"] = b.Owner // revealed once, at creation
	return v, nil
}

func (a *app) appendPage(r *http.Request, req actionRequest) (map[string]any, error) {
	b, err := a.load(req.ID)
	if err != nil {
		return nil, err
	}
	if !a.canWrite(r, b) {
		return nil, &httpError{http.StatusForbidden, "Only the owner can append to this book"}
	}
	if b.Status == kit.StatusClosed {
		return nil, &httpError{http.StatusConflict, "This book is closed"}
	}
	if breakMode != "allow-secret" && kit.SecretRefused(req.Fields) {
		return nil, &httpError{http.StatusBadRequest, kit.ErrSecret}
	}
	next := *b
	next.Pages = append([]page(nil), b.Pages...)
	if err := a.addPage(&next, req); err != nil {
		return nil, err
	}
	if err := a.save(&next); err != nil {
		return nil, &httpError{http.StatusInternalServerError, "Could not save. Your change was not accepted."}
	}
	v := a.openView(r, &next)
	n := len(next.Pages)
	v["page"] = n
	v["page_url"] = a.pageURL(r, next.ID, n)
	return v, nil
}

var (
	assetNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	headingRe   = regexp.MustCompile(`(?m)^#{1,3}\s+(.+?)\s*#*\s*$`)
)

// addPage builds the next page from a body or a template + data, stores its
// assets, and appends it to b (in memory; the caller saves).
func (a *app) addPage(b *book, req actionRequest) error {
	hasData := len(strings.TrimSpace(string(req.Data))) > 0 && strings.TrimSpace(string(req.Data)) != "null"
	var md string
	var data json.RawMessage
	switch {
	case req.Body != "" && req.Template != "":
		return &httpError{http.StatusBadRequest, "Give either a Markdown body or a template with data, not both"}
	case req.Template != "":
		sprint := req.Sprint
		if sprint == 0 {
			sprint = b.Sprint
		}
		filled, stored, err := fillTemplate(a.dir, req.Template, req.Data, sprint)
		if err != nil {
			return &httpError{http.StatusBadRequest, err.Error()}
		}
		md, data = filled, stored
	case hasData:
		return &httpError{http.StatusBadRequest, "--data needs --template NAME (templates: " + strings.Join(templateNames(a.dir), ", ") + ")"}
	case req.Body != "":
		md = req.Body
	default:
		return &httpError{http.StatusBadRequest, "Nothing to show: give a Markdown body (--body / -f FILE) or --template NAME --data FILE"}
	}
	if len(md) > 1<<20 {
		return &httpError{http.StatusBadRequest, "A page is at most 1 MiB of Markdown"}
	}
	n := len(b.Pages) + 1
	p := page{N: n, Template: req.Template, Data: data, Markdown: md, Created: now()}
	p.Title = strings.TrimSpace(req.Title)
	if p.Title == "" {
		if m := headingRe.FindStringSubmatch(md); m != nil {
			p.Title = strings.ReplaceAll(m[1], `\`, "")
		} else {
			p.Title = "Page " + strconv.Itoa(n)
		}
	}
	if len(req.Assets) > 32 {
		return &httpError{http.StatusBadRequest, "At most 32 assets per page"}
	}
	dir := assetDir(a.dir, b.ID, n)
	seen := map[string]bool{}
	for _, as := range req.Assets {
		if !assetNameRe.MatchString(as.Name) || strings.Contains(as.Name, "..") || seen[as.Name] {
			return &httpError{http.StatusBadRequest, fmt.Sprintf("asset name %q: use letters, digits, ., - or _ (unique per page)", as.Name)}
		}
		seen[as.Name] = true
		raw, err := base64.StdEncoding.DecodeString(as.Data)
		if err != nil {
			return &httpError{http.StatusBadRequest, fmt.Sprintf("asset %q: data must be base64", as.Name)}
		}
		if err := kit.AtomicWrite(filepath.Join(dir, as.Name), raw); err != nil {
			return &httpError{http.StatusInternalServerError, "Could not store asset " + as.Name}
		}
		p.Assets = append(p.Assets, as.Name)
	}
	sort.Strings(p.Assets)
	b.Pages = append(b.Pages, p)
	if b.Status == kit.StatusPending {
		b.Status = kit.StatusFilled
	}
	return nil
}

func (a *app) open(r *http.Request, req actionRequest) (map[string]any, error) {
	b, err := a.load(req.ID)
	if err != nil {
		return nil, err
	}
	return a.openView(r, b), nil
}

func pageView(p page) map[string]any {
	v := map[string]any{"n": p.N, "id": p.id(), "title": p.Title, "markdown": p.Markdown, "created": p.Created}
	if p.Template != "" {
		v["template"] = p.Template
		v["data"] = p.Data
	}
	if len(p.Assets) > 0 {
		v["assets"] = p.Assets
	}
	return v
}

// show is the one read that returns content: the book's pages as Markdown
// (and template data), or one page with --page.
func (a *app) show(r *http.Request, req actionRequest, me string) (map[string]any, error) {
	b, err := a.load(req.ID)
	if err != nil {
		return nil, err
	}
	if !a.canRead(r, b, me) {
		return nil, &httpError{http.StatusForbidden, "Join this book first; ask the owner for an invite."}
	}
	v := a.openView(r, b)
	v["title"] = b.Title
	v["revision"] = b.Revision
	if b.Sprint > 0 {
		v["sprint"] = b.Sprint
	}
	if b.Session != "" {
		v["session"] = b.Session
	}
	if req.Page != "" {
		i := b.pageIndex(req.Page)
		if i < 0 {
			return nil, &httpError{http.StatusNotFound, fmt.Sprintf("No page %q in %s (it has %d)", req.Page, b.ID, len(b.Pages))}
		}
		v["page"] = pageView(b.Pages[i])
		v["page_url"] = a.pageURL(r, b.ID, i+1)
		return v, nil
	}
	pages := make([]map[string]any, len(b.Pages))
	for i, p := range b.Pages {
		pages[i] = pageView(p)
	}
	v["items"] = pages
	return v, nil
}

func (a *app) list(r *http.Request) ([]map[string]any, error) {
	names, err := filepath.Glob(filepath.Join(a.dir, "items", "*.json"))
	if err != nil {
		return nil, &httpError{http.StatusInternalServerError, "Could not list"}
	}
	out := []map[string]any{}
	for _, p := range names {
		b, err := readBook(a.dir, strings.TrimSuffix(filepath.Base(p), ".json"))
		if err != nil {
			continue
		}
		out = append(out, a.openView(r, b)) // ids + statuses, never content
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["id"].(string) < out[j]["id"].(string) })
	return out, nil
}

func (a *app) share(r *http.Request, req actionRequest) (map[string]any, error) {
	b, err := a.load(req.ID)
	if err != nil {
		return nil, err
	}
	if !kit.Grant(kit.FacilitatorKey(r), b.Owner) {
		return nil, &httpError{http.StatusForbidden, "Only the owner can share this book"}
	}
	code := kit.NewToken()
	b.Invites[code] = true
	if err := a.save(b); err != nil {
		return nil, &httpError{http.StatusInternalServerError, "Could not save. Your change was not accepted."}
	}
	v := a.openView(r, b)
	v["invite"] = code
	v["inviteURL"] = a.urlFor(r, b.ID) + "#invite=" + code
	return v, nil
}

func (a *app) close(r *http.Request, req actionRequest) (map[string]any, error) {
	b, err := a.load(req.ID)
	if err != nil {
		return nil, err
	}
	if !a.canWrite(r, b) {
		return nil, &httpError{http.StatusForbidden, "Only the owner can close this book"}
	}
	if b.Status == kit.StatusClosed {
		return nil, &httpError{http.StatusConflict, "This book is already closed"}
	}
	b.Status = kit.StatusClosed
	if err := a.save(b); err != nil {
		return nil, &httpError{http.StatusInternalServerError, "Could not save. Your change was not accepted."}
	}
	return a.openView(r, b), nil
}

func (a *app) join(r *http.Request, req actionRequest, me string) (map[string]any, error) {
	b, err := a.load(req.ID)
	if err != nil {
		return nil, err
	}
	if !kit.ValidToken(req.Invite) || !b.Invites[req.Invite] {
		return nil, &httpError{http.StatusForbidden, "This invite is invalid. Ask the owner for a new one."}
	}
	b.Members[me] = true
	delete(b.Invites, req.Invite) // single use
	if err := a.save(b); err != nil {
		return nil, &httpError{http.StatusInternalServerError, "Could not save. Your change was not accepted."}
	}
	return a.openView(r, b), nil
}

// export writes the self-contained HTML under <data>/exports and returns
// its path (the CLI's -o fetches the bytes from GET /api/export instead).
func (a *app) export(r *http.Request, req actionRequest, me string) (map[string]any, error) {
	b, err := a.load(req.ID)
	if err != nil {
		return nil, err
	}
	if !a.canRead(r, b, me) {
		return nil, &httpError{http.StatusForbidden, "Join this book first; ask the owner for an invite."}
	}
	out, name, err := a.exportHTML(r, b, req.Page)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(a.dir, "exports", name)
	if err := kit.AtomicWrite(path, out); err != nil {
		return nil, &httpError{http.StatusInternalServerError, "Could not write the export"}
	}
	return map[string]any{"id": b.ID, "format": "html", "path": path, "bytes": len(out)}, nil
}

// asset serves /assets/<id>/<page>/<name> from that page's asset dir only,
// sandboxed so an uploaded SVG or HTML file can never script this origin.
func (a *app) asset(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	if len(parts) != 3 || !kit.ValidName(parts[0]) || !assetNameRe.MatchString(parts[2]) || strings.Contains(parts[2], "..") {
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil || n < 1 {
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}
	me := kit.ParticipantCookie(w, r, "viewer-participant")
	a.mu.Lock()
	b, err := a.load(parts[0])
	a.mu.Unlock()
	if err != nil {
		fail(w, err)
		return
	}
	if !a.canRead(r, b, me) {
		kit.Fail(w, http.StatusForbidden, "Join this book first; ask the owner for an invite.")
		return
	}
	path := filepath.Join(assetDir(a.dir, b.ID, n), parts[2])
	if _, err := os.Stat(path); err != nil {
		kit.Fail(w, http.StatusNotFound, "Not found")
		return
	}
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox; frame-ancestors 'self'")
	http.ServeFile(w, r, path)
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
	a := &app{dir: data, prefix: prefix, baseURL: "http://" + net.JoinHostPort("127.0.0.1", port) + prefix}
	fmt.Printf("\nDesk viewer — keep this terminal running\nParticipant: %s/\nSaved books: %s\nCtrl-C stops the server.\n\n",
		a.baseURL, filepath.Join(data, "items"))
	srv := &http.Server{
		Handler:           a,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
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
	os.Exit(runCLI(os.Args[1:], os.Stdin, os.Stdout))
}
