// Command session-server serves the display, owner and per-participant pages of a
// session. It holds no session state of its own: status comes from
// `bashy session status N --json`, actions run `bashy session done|next|pause|resume|time|goto|skip|move`.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	kit "desk-kit"
)

const defaultAddr = "0.0.0.0:18093"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: session-server serve N|join URL|stop N")
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = cmdServe(os.Args[2:])
	case "join":
		err = cmdJoin(os.Args[2:])
	case "stop":
		err = cmdStop(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "session:", err)
		os.Exit(1)
	}
}

// ---- status source -------------------------------------------------------

type Step struct {
	Index     int    `json:"index"`
	ID        string `json:"id"`
	Title     string `json:"title"`
	Owner     string `json:"owner"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	TimeboxS  int    `json:"timebox_s"`
	DoneWhen  string `json:"done_when,omitempty"`
	Overdue   bool   `json:"overdue"`
	Evidence  any    `json:"evidence"`
	StartedAt any    `json:"started_at"`
	DoneAt    any    `json:"done_at"`
}

type Current struct {
	Index    int      `json:"index"`
	Total    int      `json:"total"`
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Owners   []string `json:"owners"`
	TimeboxS int      `json:"timebox_s"`
	ElapsedS int      `json:"elapsed_s"`
	RemainS  int      `json:"remaining_s"`
	Overdue  bool     `json:"overdue"`
	DoneWhen string   `json:"done_when,omitempty"`
}

type Roles struct {
	Owner        string   `json:"owner"`
	Participants []string `json:"participants"`
}

type Status struct {
	Roles   Roles          `json:"roles"`
	Format  string         `json:"format"`
	Sprint  int            `json:"sprint"`
	Title   string         `json:"title"`
	Clock   map[string]any `json:"clock"`
	Paused  bool           `json:"paused"`
	Current *Current       `json:"current"`
	Steps   []Step         `json:"steps"`
}

type tokenFile struct {
	Sprint       int               `json:"sprint"`
	Owner        string            `json:"owner"`
	Participants map[string]string `json:"participants"`
	Addr         string            `json:"addr"`
	Prefix       string            `json:"prefix"`
}

type server struct {
	n       int
	bin     string
	prefix  string
	path    string // token file
	mu      sync.Mutex
	tok     tokenFile
	byToken map[string]string
	st      *Status
	at      time.Time
	lastErr string
	quit    chan struct{}
	once    sync.Once
}

func newServer(n int, bin, prefix, path string) *server {
	s := &server{n: n, bin: bin, prefix: normPrefix(prefix), path: path,
		byToken: map[string]string{}, quit: make(chan struct{})}
	s.tok = tokenFile{Sprint: n, Participants: map[string]string{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s.tok)
	}
	if s.tok.Participants == nil {
		s.tok.Participants = map[string]string{}
	}
	if !kit.ValidToken(s.tok.Owner) {
		s.tok.Owner = kit.NewToken()
	}
	for name, t := range s.tok.Participants {
		s.byToken[t] = name
	}
	return s
}

func (s *server) bashy(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.bin, append([]string{"session"}, args...)...)
	cmd.Env = append(os.Environ(), "BASHY_HINTS=off")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return out.Bytes(), errors.New(msg)
	}
	return out.Bytes(), nil
}

func (s *server) refresh() {
	out, err := s.bashy("status", fmt.Sprint(s.n), "--json")
	var st Status
	if err == nil {
		err = json.Unmarshal(out, &st)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.lastErr = err.Error()
		return
	}
	s.lastErr = ""
	s.st, s.at = &st, time.Now()
	s.mintLocked()
}

// mintLocked gives every step owner a stable random token (idempotent).
func (s *server) mintLocked() {
	changed := false
	for _, st := range s.st.Steps {
		for _, o := range splitOwners(st.Owner) {
			if _, ok := s.tok.Participants[o]; !ok {
				t := kit.NewToken()
				s.tok.Participants[o] = t
				s.byToken[t] = o
				changed = true
			}
		}
	}
	if changed {
		_ = s.saveLocked()
	}
}

func splitOwners(v string) []string {
	var r []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			r = append(r, p)
		}
	}
	return r
}

func (s *server) saveLocked() error {
	b, err := kit.MarshalIndent(s.tok)
	if err != nil {
		return err
	}
	return kit.AtomicWrite(s.path, b)
}

// snapshot returns a copy of the status with times advanced to now.
func (s *server) snapshot() (*Status, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st == nil {
		return nil, s.lastErr
	}
	c := *s.st
	if c.Current != nil && !c.Paused {
		cur := *c.Current
		d := int(time.Since(s.at).Seconds())
		cur.ElapsedS += d
		cur.RemainS -= d
		cur.Overdue = cur.RemainS < 0
		c.Current = &cur
	}
	return &c, s.lastErr
}

func (s *server) personOf(token string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p, ok := s.byToken[token]; ok {
		return p, ok
	}
	for name, sl := range slugsLocked(s.tok.Participants) {
		if sl == token {
			return name, true
		}
	}
	return "", false
}

// slugsLocked gives every participant a readable link name from their own
// name ("Ana Li" -> "ana-li"); a duplicate gets -2, -3 … in name order.
// Callers hold s.mu.
func slugsLocked(participants map[string]string) map[string]string {
	names := make([]string, 0, len(participants))
	for n := range participants {
		names = append(names, n)
	}
	sort.Strings(names)
	out, used := map[string]string{}, map[string]bool{}
	for _, n := range names {
		base := slugify(n)
		sl := base
		for i := 2; used[sl]; i++ {
			sl = fmt.Sprintf("%s-%d", base, i)
		}
		used[sl] = true
		out[n] = sl
	}
	return out
}

func slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	sl := strings.TrimRight(b.String(), "-")
	if sl == "" {
		sl = "person"
	}
	return sl
}

func (s *server) ownerOK(r *http.Request) bool {
	return kit.Grant(kit.FacilitatorKey(r), s.tok.Owner)
}

// ---- HTTP ------------------------------------------------------------------

// publicSteps drops evidence and ownership detail the display must not show.
func publicView(st *Status, errMsg string) map[string]any {
	steps := make([]map[string]any, 0, len(st.Steps))
	for _, x := range st.Steps {
		steps = append(steps, map[string]any{"index": x.Index, "title": x.Title, "owner": x.Owner,
			"status": x.Status, "overdue": x.Overdue})
	}
	return map[string]any{"title": st.Title, "clock": st.Clock, "paused": st.Paused, "current": st.Current, "steps": steps, "error": errMsg}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { kit.WriteJSON(w, map[string]bool{"ok": true}) })
	mux.HandleFunc("GET /{$}", s.displayPage)
	mux.HandleFunc("GET /p", s.displayPage)
	mux.HandleFunc("GET /api/display", func(w http.ResponseWriter, r *http.Request) {
		st, e := s.snapshot()
		if st == nil {
			kit.Fail(w, 503, "no status yet: "+e)
			return
		}
		kit.WriteJSON(w, publicView(st, e))
	})
	mux.HandleFunc("GET /t/{key}", s.ownerPage)
	mux.HandleFunc("GET /api/owner", func(w http.ResponseWriter, r *http.Request) {
		if !s.ownerOK(r) {
			kit.Fail(w, 403, "owner key required")
			return
		}
		st, e := s.snapshot()
		if st == nil {
			kit.Fail(w, 503, "no status yet: "+e)
			return
		}
		kit.WriteJSON(w, map[string]any{"sprint": st.Sprint, "title": st.Title, "clock": st.Clock, "paused": st.Paused, "roles": st.Roles, "format": st.Format,
			"current": st.Current, "steps": st.Steps, "error": e})
	})
	mux.HandleFunc("POST /api/owner/next", s.ownerAction("next"))
	for _, verb := range []string{"pause", "resume", "time", "goto", "skip", "move"} {
		mux.HandleFunc("POST /api/owner/"+verb, s.ownerAction(verb))
	}
	mux.HandleFunc("POST /api/owner/stop", func(w http.ResponseWriter, r *http.Request) {
		if !s.ownerOK(r) {
			kit.Fail(w, 403, "owner key required")
			return
		}
		kit.WriteJSON(w, map[string]string{"stopped": "ok"})
		s.once.Do(func() { close(s.quit) })
	})
	mux.HandleFunc("GET /s/{token}", s.participantPage)
	mux.HandleFunc("GET /api/s/{token}", s.participantState)
	mux.HandleFunc("POST /s/{token}/done", s.participantDone)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		kit.ApplySecurityHeaders(w)
		if !kit.GuardPOST(w, r, "session") {
			return
		}
		rest, ok := kit.StripPrefix(s.prefix, r.URL.Path)
		if !ok {
			kit.Fail(w, 404, "not found")
			return
		}
		r2 := r.Clone(r.Context())
		r2.URL.Path, r2.URL.RawPath = rest, ""
		mux.ServeHTTP(w, r2)
	})
}

func (s *server) ownerAction(verb string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.ownerOK(r) {
			kit.Fail(w, 403, "owner key required")
			return
		}
		var body struct {
			Step string `json:"step"`
			Add  string `json:"add"`
			Dir  string `json:"dir"`
			To   int    `json:"to"`
		}
		if !kit.DecodeAction(w, r, 8<<10, &body) {
			return
		}
		args := []string{verb, fmt.Sprint(s.n)}
		bad := func(msg string) { kit.Fail(w, 400, msg) }
		if body.Step != "" && !stepRe(body.Step) {
			bad("invalid step")
			return
		}
		switch verb {
		case "time":
			if !durRe(body.Add) {
				bad("time needs a signed duration like 5m or -1m")
				return
			}
			if body.Step != "" {
				args = append(args, body.Step)
			}
			args = append(args, "--add="+body.Add)
		case "goto":
			if body.Step == "" {
				bad("goto needs a step")
				return
			}
			args = append(args, body.Step)
		case "move":
			if body.Step == "" {
				bad("move needs a step")
				return
			}
			args = append(args, body.Step)
			switch body.Dir {
			case "up", "down":
				args = append(args, "--"+body.Dir)
			case "to":
				if body.To < 1 || body.To > 999 {
					bad("move --to needs a position")
					return
				}
				args = append(args, "--to", fmt.Sprint(body.To))
			default:
				bad("move needs dir up, down or to")
				return
			}
		}
		args = append(args, "--json")
		out, err := s.bashy(args...)
		s.refresh()
		if err != nil {
			kit.Fail(w, 502, err.Error())
			return
		}
		kit.WriteJSON(w, map[string]any{"ok": true, "output": strings.TrimSpace(string(out))})
	}
}

func stepRe(v string) bool {
	if v == "" || len(v) > 64 || v[0] == '-' {
		return false
	}
	for _, c := range v {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' || c == '_' || c == '.') {
			return false
		}
	}
	return true
}

func durRe(v string) bool {
	if v != "" && (v[0] == '-' || v[0] == '+') {
		v = v[1:]
	}
	if len(v) < 2 || len(v) > 6 || v[0] == '0' {
		return false
	}
	for _, c := range v[:len(v)-1] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return strings.ContainsRune("smh", rune(v[len(v)-1]))
}

// participantView is what one person sees: their own step and nothing of others.
func (s *server) participantView(person string) (map[string]any, bool) {
	st, e := s.snapshot()
	if st == nil {
		return nil, false
	}
	own := func(x string) bool {
		for _, o := range splitOwners(x) {
			if o == person {
				return true
			}
		}
		return false
	}
	v := map[string]any{"person": person, "roles": st.Roles, "role": "", "title": st.Title, "clock": st.Clock, "paused": st.Paused, "error": e, "mine": nil, "upcoming": []map[string]any{}}
	if c := st.Current; c != nil {
		for _, o := range c.Owners {
			if o == person {
				doneWhen := c.DoneWhen
				for _, x := range st.Steps {
					if x.ID == c.ID && doneWhen == "" {
						doneWhen = x.DoneWhen
					}
				}
				v["mine"] = map[string]any{"index": c.Index, "total": c.Total, "title": c.Title, "id": c.ID,
					"remaining_s": c.RemainS, "timebox_s": c.TimeboxS, "overdue": c.Overdue, "done_when": doneWhen}
			}
		}
		v["now"] = map[string]any{"index": c.Index, "title": c.Title}
	}
	up := []map[string]any{}
	doneN := 0
	for _, x := range st.Steps {
		if !own(x.Owner) {
			continue
		}
		v["role"] = x.Role
		if x.Status == "done" {
			doneN++
		} else if st.Current == nil || x.ID != st.Current.ID {
			up = append(up, map[string]any{"index": x.Index, "title": x.Title, "timebox_s": x.TimeboxS})
		}
	}
	v["upcoming"], v["done_count"] = up, doneN
	return v, true
}

func (s *server) participantState(w http.ResponseWriter, r *http.Request) {
	person, ok := s.personOf(r.PathValue("token"))
	if !ok {
		kit.Fail(w, 404, "unknown link")
		return
	}
	v, ok := s.participantView(person)
	if !ok {
		kit.Fail(w, 503, "no status yet")
		return
	}
	kit.WriteJSON(w, v)
}

func (s *server) participantDone(w http.ResponseWriter, r *http.Request) {
	person, ok := s.personOf(r.PathValue("token"))
	if !ok {
		kit.Fail(w, 404, "unknown link")
		return
	}
	var body struct {
		Evidence string `json:"evidence"`
		Step     string `json:"step"`
	}
	if !kit.DecodeAction(w, r, 8<<10, &body) {
		return
	}
	s.refresh()
	st, _ := s.snapshot()
	if st == nil || st.Current == nil {
		kit.Fail(w, 409, "no step is running")
		return
	}
	mine := false
	for _, o := range st.Current.Owners {
		mine = mine || o == person
	}
	if !mine {
		kit.Fail(w, 403, "the current step is not yours")
		return
	}
	if body.Step != "" && body.Step != st.Current.ID && body.Step != fmt.Sprint(st.Current.Index) {
		kit.Fail(w, 403, "you can only finish your own current step")
		return
	}
	ev := strings.TrimSpace(body.Evidence)
	if len(ev) > 500 {
		ev = ev[:500]
	}
	args := []string{"done", fmt.Sprint(s.n), st.Current.ID}
	if ev != "" {
		args = append(args, "-m", ev)
	}
	args = append(args, "--json")
	if _, err := s.bashy(args...); err != nil {
		kit.Fail(w, 502, err.Error())
		return
	}
	s.refresh()
	kit.WriteJSON(w, map[string]any{"ok": true, "step": st.Current.ID, "person": person})
}

// ---- pages -------------------------------------------------------------------

func (s *server) render(w http.ResponseWriter, r *http.Request, title, body, script string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page(title, s.statusLabel(), body, "const PFX="+jsq(kit.PublicPrefix(r, s.prefix))+";"+script))
}

func (s *server) displayPage(w http.ResponseWriter, r *http.Request) {
	s.render(w, r, "Display", displayMain, "navKeys(()=>{try{return localStorage.getItem('session-owner-key')}catch(e){return null}});start('/api/display',renderDisplay)")
}

func (s *server) ownerPage(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if !kit.Grant(key, s.tok.Owner) {
		kit.Fail(w, 403, "owner key required")
		return
	}
	s.render(w, r, s.roleTitle(true, ""), ownerMain, "const KEY="+jsq(key)+";try{localStorage.setItem('session-owner-key',KEY)}catch(e){}navKeys(()=>KEY);start('/api/owner',renderOwner,{'X-Facilitator-Key':KEY})")
}

func (s *server) participantPage(w http.ResponseWriter, r *http.Request) {
	tk := r.PathValue("token")
	if _, ok := s.personOf(tk); !ok {
		kit.Fail(w, 404, "unknown link")
		return
	}
	s.render(w, r, s.roleTitle(false, tk), participantMain, "const TOKEN="+jsq(tk)+";start('/api/s/'+TOKEN,renderParticipant)")
}

func (s *server) statusLabel() string { return fmt.Sprintf("Sprint %d", s.n) }

func (s *server) roleTitle(owner bool, token string) string {
	st, _ := s.snapshot()
	name := ""
	if st != nil {
		if owner {
			name = st.Roles.Owner
		} else {
			person, _ := s.personOf(token)
			for _, step := range st.Steps {
				for _, p := range splitOwners(step.Owner) {
					if p == person {
						name = step.Role
						break
					}
				}
			}
			if name == "" && len(st.Roles.Participants) > 0 {
				name = st.Roles.Participants[0]
			}
		}
	}
	if name == "" {
		if owner {
			return "Organizer"
		}
		return "Participant"
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

func jsq(v string) string { b, _ := json.Marshal(v); return string(b) }

// ---- serve / join / stop -------------------------------------------------------

func dataDir() string { return kit.DefaultDataDir("session") }

// parse lets flags appear before or after the positional argument.
func parse(fs *flag.FlagSet, args []string) (string, error) {
	var pos string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return "", err
		}
		args = fs.Args()
		if len(args) > 0 {
			if pos != "" {
				return "", fmt.Errorf("unexpected argument %q", args[0])
			}
			pos, args = args[0], args[1:]
		}
	}
	return pos, nil
}

func normPrefix(p string) string {
	p = strings.Trim(p, "/")
	if p == "" {
		return ""
	}
	return "/" + p
}

func lanHost(addr string) string {
	host, _, _ := net.SplitHostPort(addr)
	if host != "" && host != "0.0.0.0" && host != "::" {
		return host
	}
	if ifs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range ifs {
			if ipn, ok := a.(*net.IPNet); ok && !ipn.IP.IsLoopback() && ipn.IP.To4() != nil {
				return ipn.IP.String()
			}
		}
	}
	return "127.0.0.1"
}

func sprintArg(pos string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(pos, "%d", &n); err != nil || n <= 0 || fmt.Sprint(n) != pos {
		return 0, errors.New("usage: session serve N [--addr A] [--prefix P]")
	}
	return n, nil
}

// resolveConsole finds the console-managed deployment of this app: the
// SESSION_ADDR/SESSION_DATA handoff (set by a caller that already ran
// discovery) wins; otherwise it asks the console through
// `bashy app show session --runtime --json` (Sprint 682 C1b contract:
// addr, data, console app URL). Failure is one actionable error — the
// caller never falls back to a fixed port or its own server.
func resolveConsole(bin string) (addr, data, appURL string, err error) {
	addr, data, appURL = os.Getenv("SESSION_ADDR"), os.Getenv("SESSION_DATA"), os.Getenv("SESSION_APP_URL")
	if addr != "" && data != "" {
		return addr, data, appURL, nil
	}
	fail := func(why string) (string, string, string, error) {
		return "", "", "", fmt.Errorf("no console-managed session app found (%s): open the session app once in the console, or set SESSION_ADDR and SESSION_DATA", why)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "app", "show", "session", "--runtime", "--json")
	cmd.Env = append(os.Environ(), "BASHY_HINTS=off")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return fail("app show --runtime: " + msg)
	}
	var rt map[string]any
	if err := json.Unmarshal(out.Bytes(), &rt); err != nil {
		return fail("unreadable runtime record")
	}
	// The record may carry the deployment at top level or nested
	// under "runtime"; normalize keys before looking anything up.
	norm := map[string]any{}
	var merge func(map[string]any)
	merge = func(m map[string]any) {
		for k, v := range m {
			lk := strings.ToLower(k)
			if lk == "runtime" {
				if inner, ok := v.(map[string]any); ok {
					merge(inner)
				}
				continue
			}
			norm[lk] = v
		}
	}
	merge(rt)
	get := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := norm[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	addr, data = get("addr", "address"), get("data", "data_dir")
	appURL = get("url", "app_url", "console_url")
	if addr == "" || data == "" {
		return fail("the runtime record carries no addr/data yet")
	}
	return addr, data, appURL, nil
}

// printLinks prints the display, owner and per-person join links under
// origin (which already excludes the sprint prefix; s.prefix adds it).
func printLinks(s *server, origin string, asJSON bool) {
	s.mu.Lock()
	names := make([]string, 0, len(s.tok.Participants))
	links := map[string]string{}
	slugs := slugsLocked(s.tok.Participants)
	for k := range s.tok.Participants {
		names = append(names, k)
		links[k] = origin + s.prefix + "/s/" + slugs[k]
	}
	s.mu.Unlock()
	sort.Strings(names)
	display, owner := origin+s.prefix+"/", origin+s.prefix+"/t/"+s.tok.Owner
	if asJSON {
		b, _ := json.Marshal(map[string]any{"sprint": s.n, "display": display, "owner": owner, "participants": links})
		fmt.Println(string(b))
	} else {
		fmt.Printf("Display: %s\n%s (keep private): %s\n", display, s.roleTitle(true, ""), owner)
		for _, k := range names {
			fmt.Printf("Join %s: %s\n", k, links[k])
		}
	}
	os.Stdout.Sync()
}

// registerSprint writes (or refreshes) sprint N's token file in data
// and prints its links, without serving: the console host picks the
// file up and mounts the sprint at /<N>.
func registerSprint(n int, bin, data, origin string, asJSON bool) error {
	if err := os.MkdirAll(data, 0o700); err != nil {
		return err
	}
	s := newServer(n, bin, fmt.Sprintf("/%d", n), filepath.Join(data, fmt.Sprintf("%d-serve.json", n)))
	s.tok.Sprint, s.tok.Prefix = n, s.prefix
	s.refresh()
	s.mu.Lock()
	if s.st == nil {
		fmt.Fprintln(os.Stderr, "session: warning: no status yet:", s.lastErr)
	}
	err := s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	printLinks(s, strings.TrimRight(origin, "/"), asJSON)
	return nil
}

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", defaultAddr, "listen address")
	prefix := fs.String("prefix", "", "URL path prefix")
	poll := fs.Duration("poll", 3*time.Second, "status poll interval")
	data := fs.String("data", dataDir(), "data directory")
	asJSON := fs.Bool("json", false, "print links as JSON")
	console := fs.Bool("console", false, "register sprint N with the console-managed host instead of serving")
	registerOnly := fs.Bool("register-only", false, "write sprint N's token file and print links, without serving")
	originOpt := fs.String("origin", "", "link origin override (e.g. the console app URL)")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	bin := os.Getenv("BASHY_BIN")
	if bin == "" {
		bin = "bashy"
	}
	visited := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { visited[f.Name] = true })
	if *console {
		for _, name := range []string{"addr", "data", "prefix"} {
			if visited[name] {
				return fmt.Errorf("session serve -console resolves address and data from the console; --%s does not apply", name)
			}
		}
		n, err := sprintArg(pos)
		if err != nil {
			return err
		}
		caddr, cdata, appURL, err := resolveConsole(bin)
		if err != nil {
			return err
		}
		origin := strings.TrimRight(*originOpt, "/")
		if origin == "" {
			origin = strings.TrimRight(appURL, "/")
		}
		if origin == "" {
			origin = "http://" + caddr
		}
		return registerSprint(n, bin, cdata, origin, *asJSON)
	}
	if pos == "" {
		if *registerOnly {
			return errors.New("usage: session serve N --register-only")
		}
		if visited["prefix"] || visited["origin"] {
			return errors.New("session host mounts each sprint at /<N>; --prefix and --origin do not apply")
		}
		// No sprint: host mode — one console process serving every
		// sprint registered in the data directory (see cmdHost).
		return cmdHost(*addr, *data, *poll, bin)
	}
	n, err := sprintArg(pos)
	if err != nil {
		return err
	}
	if *registerOnly {
		if visited["prefix"] {
			return errors.New("session serve --register-only mounts the sprint at /<N>; --prefix does not apply")
		}
		origin := strings.TrimRight(*originOpt, "/")
		if origin == "" {
			origin = "http://" + net.JoinHostPort(lanHost(*addr), portOf(*addr))
		}
		return registerSprint(n, bin, *data, origin, *asJSON)
	}
	if err := os.MkdirAll(*data, 0o700); err != nil {
		return err
	}
	s := newServer(n, bin, *prefix, filepath.Join(*data, fmt.Sprintf("%d-serve.json", n)))
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	s.tok.Sprint, s.tok.Addr, s.tok.Prefix = n, ln.Addr().String(), s.prefix
	s.refresh()
	s.mu.Lock()
	if s.st == nil {
		fmt.Fprintln(os.Stderr, "session: warning: no status yet:", s.lastErr)
	}
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	origin := strings.TrimRight(*originOpt, "/")
	if origin == "" {
		origin = "http://" + net.JoinHostPort(lanHost(ln.Addr().String()), fmt.Sprint(ln.Addr().(*net.TCPAddr).Port))
	}
	printLinks(s, origin, *asJSON)

	srv := &http.Server{Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		t := time.NewTicker(*poll)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s.refresh()
			case <-s.quit:
				return
			}
		}
	}()
	go func() {
		<-s.quit
		ctx, c := context.WithTimeout(context.Background(), 3*time.Second)
		defer c()
		srv.Shutdown(ctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func portOf(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "18093"
	}
	return port
}

// ---- console host ----------------------------------------------------------
//
// The console runs one server process per principal (shaped.json:
// serve -addr $ADDR -data $DATA, no sprint argument). The host mounts
// the existing per-sprint handler at /<N> for every sprint with a
// token file in the data directory — `session serve N` (console mode)
// writes that file, `session stop N` removes it, and the host picks
// both up on its scan cadence. The standalone serve N path is
// unchanged for go/demo and direct use.

type hostInst struct {
	s       *server
	handler http.Handler
	stop    chan struct{}
	once    sync.Once
}

type host struct {
	bin, data, addr string
	poll            time.Duration
	mu              sync.Mutex
	live            map[int]*hostInst
}

func tokenPath(data string, n int) string {
	return filepath.Join(data, fmt.Sprintf("%d-serve.json", n))
}

func (h *host) scan() {
	ents, _ := filepath.Glob(filepath.Join(h.data, "*-serve.json"))
	want := map[int]bool{}
	for _, e := range ents {
		base := filepath.Base(e)
		n, err := strconv.Atoi(strings.TrimSuffix(base, "-serve.json"))
		if err == nil && n > 0 && fmt.Sprintf("%d-serve.json", n) == base {
			want[n] = true
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for n := range h.live {
		if !want[n] {
			h.dropLocked(n)
		}
	}
	for n := range want {
		if _, ok := h.live[n]; ok {
			continue
		}
		s := newServer(n, h.bin, fmt.Sprintf("/%d", n), tokenPath(h.data, n))
		s.tok.Sprint, s.tok.Addr, s.tok.Prefix = n, h.addr, s.prefix
		inst := &hostInst{s: s, handler: s.handler(), stop: make(chan struct{})}
		h.live[n] = inst
		go h.serveInst(n, inst)
	}
}

func (h *host) dropLocked(n int) {
	inst, ok := h.live[n]
	if !ok {
		return
	}
	delete(h.live, n)
	inst.once.Do(func() { close(inst.stop) })
}

// serveInst polls one sprint's status until the instance is dropped
// (token file gone) or its owner stops it from the owner page, which
// in host mode retires just that sprint — token file included, so a
// rescan does not resurrect it.
func (h *host) serveInst(n int, inst *hostInst) {
	inst.s.refresh()
	inst.s.mu.Lock()
	_ = inst.s.saveLocked()
	inst.s.mu.Unlock()
	t := time.NewTicker(h.poll)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			inst.s.refresh()
		case <-inst.stop:
			return
		case <-inst.s.quit:
			h.mu.Lock()
			h.dropLocked(n)
			h.mu.Unlock()
			_ = os.Remove(tokenPath(h.data, n))
			return
		}
	}
}

func (h *host) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/healthz" && r.Method == http.MethodGet {
			kit.ApplySecurityHeaders(w)
			kit.WriteJSON(w, map[string]bool{"ok": true})
			return
		}
		seg := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)[0]
		if n, err := strconv.Atoi(seg); err == nil && n > 0 {
			h.mu.Lock()
			inst := h.live[n]
			h.mu.Unlock()
			if inst == nil {
				kit.ApplySecurityHeaders(w)
				kit.Fail(w, 404, "no such session")
				return
			}
			inst.handler.ServeHTTP(w, r)
			return
		}
		if path == "/" && r.Method == http.MethodGet {
			h.index(w, r)
			return
		}
		kit.ApplySecurityHeaders(w)
		kit.Fail(w, 404, "not found")
	})
}

// index lists the live sessions with their (public) display links. It
// never carries owner keys or participant links.
func (h *host) index(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	ns := make([]int, 0, len(h.live))
	insts := make(map[int]*hostInst, len(h.live))
	for n, inst := range h.live {
		ns = append(ns, n)
		insts[n] = inst
	}
	h.mu.Unlock()
	sort.Ints(ns)
	var b strings.Builder
	b.WriteString(`<main class="desk-main"><h1 class="desk-h1">Sessions</h1>`)
	if len(ns) == 0 {
		b.WriteString(`<p class="desk-muted">No live sessions. Start one with <code>bashy session serve N</code>.</p>`)
	}
	for _, n := range ns {
		title := fmt.Sprintf("Sprint %d", n)
		if st, _ := insts[n].s.snapshot(); st != nil && st.Title != "" {
			title = st.Title
		}
		fmt.Fprintf(&b, `<p><a href="%s/%d/">%s</a></p>`, kit.PublicPrefix(r, ""), n, html.EscapeString(title))
	}
	b.WriteString(`</main>`)
	kit.ApplySecurityHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(kit.Page("session", "Sessions", kit.AppBar("session", "", "", "")+b.String()))
}

func cmdHost(addr, data string, poll time.Duration, bin string) error {
	if err := os.MkdirAll(data, 0o700); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	h := &host{bin: bin, data: data, addr: ln.Addr().String(), poll: poll, live: map[int]*hostInst{}}
	h.scan()
	go func() {
		t := time.NewTicker(poll)
		defer t.Stop()
		for range t.C {
			h.scan()
		}
	}()
	fmt.Printf("session host: serving on %s (data %s)\n", ln.Addr(), data)
	os.Stdout.Sync()
	srv := &http.Server{Handler: h.handler(), ReadHeaderTimeout: 10 * time.Second}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func cmdJoin(args []string) error {
	fs := flag.NewFlagSet("join", flag.ContinueOnError)
	done := fs.Bool("done", false, "mark your current step done")
	ev := fs.String("m", "", "evidence (text or desk tool link)")
	open := fs.Bool("open", false, "open the page in a browser")
	asJSON := fs.Bool("json", false, "print JSON")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	u, err := url.Parse(pos)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || !strings.Contains(u.Path, "/s/") {
		return errors.New("usage: session join URL [--done -m EVIDENCE]  (URL is your participant link)")
	}
	u.RawQuery, u.Fragment = "", ""
	if !*done {
		if *asJSON {
			b, _ := json.Marshal(map[string]string{"url": u.String()})
			fmt.Println(string(b))
		} else {
			fmt.Println(u.String())
		}
		if *open {
			return openURL(u.String())
		}
		return nil
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/done"
	b, _ := json.Marshal(map[string]string{"evidence": *ev})
	c := &http.Client{Timeout: 30 * time.Second}
	resp, err := c.Post(u.String(), "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode != 200 {
		var e struct{ Error string }
		_ = json.Unmarshal(body, &e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return errors.New(e.Error)
	}
	fmt.Println(strings.TrimSpace(string(body)))
	return nil
}

func openURL(u string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", u).Run()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Run()
	}
	return exec.Command("xdg-open", u).Run()
}

func cmdStop(args []string) error {
	fs := flag.NewFlagSet("stop", flag.ContinueOnError)
	data := fs.String("data", dataDir(), "data directory")
	asJSON := fs.Bool("json", false, "print JSON")
	console := fs.Bool("console", false, "unregister sprint N from the console-managed host")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	var n int
	if _, err := fmt.Sscanf(pos, "%d", &n); err != nil || n <= 0 {
		return errors.New("usage: session stop N")
	}
	if *console {
		// The console owns the process; stopping a sprint is removing
		// its registration so the host drops the mount.
		bin := os.Getenv("BASHY_BIN")
		if bin == "" {
			bin = "bashy"
		}
		_, cdata, _, err := resolveConsole(bin)
		if err != nil {
			return err
		}
		if err := os.Remove(tokenPath(cdata, n)); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("no session registered for sprint %d", n)
			}
			return err
		}
		if *asJSON {
			fmt.Printf("{\"sprint\":%d,\"stopped\":true}\n", n)
		} else {
			fmt.Printf("stopped session for sprint %d\n", n)
		}
		return nil
	}
	b, err := os.ReadFile(filepath.Join(*data, fmt.Sprintf("%d-serve.json", n)))
	if err != nil {
		return fmt.Errorf("no server recorded for sprint %d", n)
	}
	var t tokenFile
	if err := json.Unmarshal(b, &t); err != nil {
		return err
	}
	host, port, _ := net.SplitHostPort(t.Addr)
	if host == "" || host == "0.0.0.0" || host == "::" || host == "[::]" {
		host = "127.0.0.1"
	}
	c := &http.Client{Timeout: 5 * time.Second}
	req, _ := http.NewRequest("POST", "http://"+net.JoinHostPort(host, port)+t.Prefix+"/api/owner/stop", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Facilitator-Key", t.Owner)
	resp, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("no server running for sprint %d", n)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("stop refused: %s", resp.Status)
	}
	if *asJSON {
		fmt.Printf("{\"sprint\":%d,\"stopped\":true}\n", n)
	} else {
		fmt.Printf("stopped session server for sprint %d\n", n)
	}
	return nil
}
