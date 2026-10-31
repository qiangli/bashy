// Command tutor-server serves the projector, teacher and per-student pages of a
// tutor session. It holds no session state of its own: status comes from
// `bashy tutor status N --json`, actions run `bashy tutor done|next|extend`.
package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	kit "desk-kit"
)

const defaultAddr = "0.0.0.0:18093"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: tutor-server serve N|join URL|stop N")
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
		fmt.Fprintln(os.Stderr, "tutor:", err)
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

type Status struct {
	Sprint  int            `json:"sprint"`
	Title   string         `json:"title"`
	Clock   map[string]any `json:"clock"`
	Current *Current       `json:"current"`
	Steps   []Step         `json:"steps"`
}

type tokenFile struct {
	Sprint   int               `json:"sprint"`
	Teacher  string            `json:"teacher"`
	Students map[string]string `json:"students"`
	Addr     string            `json:"addr"`
	Prefix   string            `json:"prefix"`
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
	s.tok = tokenFile{Sprint: n, Students: map[string]string{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s.tok)
	}
	if s.tok.Students == nil {
		s.tok.Students = map[string]string{}
	}
	if !kit.ValidToken(s.tok.Teacher) {
		s.tok.Teacher = kit.NewToken()
	}
	for name, t := range s.tok.Students {
		s.byToken[t] = name
	}
	return s
}

func (s *server) bashy(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.bin, append([]string{"tutor"}, args...)...)
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
			if _, ok := s.tok.Students[o]; !ok {
				t := kit.NewToken()
				s.tok.Students[o] = t
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
	b, _ := json.MarshalIndent(s.tok, "", "  ")
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// snapshot returns a copy of the status with times advanced to now.
func (s *server) snapshot() (*Status, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.st == nil {
		return nil, s.lastErr
	}
	c := *s.st
	if c.Current != nil {
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
	p, ok := s.byToken[token]
	return p, ok
}

func (s *server) teacherOK(key string) bool {
	return subtle.ConstantTimeCompare([]byte(key), []byte(s.tok.Teacher)) == 1
}

// ---- HTTP ------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeBody(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, 413, map[string]string{"error": "body too large"})
		return false
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return true
	}
	if err := json.Unmarshal(b, v); err != nil {
		writeJSON(w, 400, map[string]string{"error": "bad json"})
		return false
	}
	return true
}

// publicSteps drops evidence and ownership detail the projector must not show.
func publicView(st *Status, errMsg string) map[string]any {
	steps := make([]map[string]any, 0, len(st.Steps))
	for _, x := range st.Steps {
		steps = append(steps, map[string]any{"index": x.Index, "title": x.Title, "owner": x.Owner,
			"status": x.Status, "overdue": x.Overdue})
	}
	return map[string]any{"title": st.Title, "clock": st.Clock, "current": st.Current, "steps": steps, "error": errMsg}
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	p := s.prefix
	mux.HandleFunc("GET "+p+"/{$}", s.projectorPage)
	mux.HandleFunc("GET "+p+"/p", s.projectorPage)
	mux.HandleFunc("GET "+p+"/api/projector", func(w http.ResponseWriter, r *http.Request) {
		st, e := s.snapshot()
		if st == nil {
			writeJSON(w, 503, map[string]string{"error": "no status yet: " + e})
			return
		}
		writeJSON(w, 200, publicView(st, e))
	})
	mux.HandleFunc("GET "+p+"/t/{key}", s.teacherPage)
	mux.HandleFunc("GET "+p+"/api/t/{key}", func(w http.ResponseWriter, r *http.Request) {
		if !s.teacherOK(r.PathValue("key")) {
			writeJSON(w, 403, map[string]string{"error": "teacher key required"})
			return
		}
		st, e := s.snapshot()
		if st == nil {
			writeJSON(w, 503, map[string]string{"error": "no status yet: " + e})
			return
		}
		writeJSON(w, 200, map[string]any{"sprint": st.Sprint, "title": st.Title, "clock": st.Clock,
			"current": st.Current, "steps": st.Steps, "error": e})
	})
	mux.HandleFunc("POST "+p+"/t/{key}/next", s.teacherAction("next"))
	mux.HandleFunc("POST "+p+"/t/{key}/extend", s.teacherAction("extend"))
	mux.HandleFunc("POST "+p+"/t/{key}/stop", func(w http.ResponseWriter, r *http.Request) {
		if !s.teacherOK(r.PathValue("key")) {
			writeJSON(w, 403, map[string]string{"error": "teacher key required"})
			return
		}
		writeJSON(w, 200, map[string]string{"stopped": "ok"})
		s.once.Do(func() { close(s.quit) })
	})
	mux.HandleFunc("GET "+p+"/s/{token}", s.studentPage)
	mux.HandleFunc("GET "+p+"/api/s/{token}", s.studentState)
	mux.HandleFunc("POST "+p+"/s/{token}/done", s.studentDone)
	return mux
}

func (s *server) teacherAction(verb string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.teacherOK(r.PathValue("key")) {
			writeJSON(w, 403, map[string]string{"error": "teacher key required"})
			return
		}
		var body struct {
			Step string `json:"step"`
			By   string `json:"by"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		args := []string{verb, fmt.Sprint(s.n)}
		if verb == "extend" {
			st, _ := s.snapshot()
			step := body.Step
			if step == "" && st != nil && st.Current != nil {
				step = st.Current.ID
			}
			by := body.By
			if by == "" {
				by = "5m"
			}
			if step == "" || !durRe(by) || strings.HasPrefix(step, "-") {
				writeJSON(w, 400, map[string]string{"error": "extend needs a step and a duration like 5m"})
				return
			}
			args = append(args, step, "--by", by)
		}
		args = append(args, "--json")
		out, err := s.bashy(args...)
		s.refresh()
		if err != nil {
			writeJSON(w, 502, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "output": strings.TrimSpace(string(out))})
	}
}

func durRe(v string) bool {
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

// studentView is what one person sees: their own step and nothing of others.
func (s *server) studentView(person string) (map[string]any, bool) {
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
	v := map[string]any{"person": person, "title": st.Title, "clock": st.Clock, "error": e, "mine": nil, "upcoming": []map[string]any{}}
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
		if x.Status == "done" {
			doneN++
		} else if st.Current == nil || x.ID != st.Current.ID {
			up = append(up, map[string]any{"index": x.Index, "title": x.Title, "timebox_s": x.TimeboxS})
		}
	}
	v["upcoming"], v["done_count"] = up, doneN
	return v, true
}

func (s *server) studentState(w http.ResponseWriter, r *http.Request) {
	person, ok := s.personOf(r.PathValue("token"))
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "unknown link"})
		return
	}
	v, ok := s.studentView(person)
	if !ok {
		writeJSON(w, 503, map[string]string{"error": "no status yet"})
		return
	}
	writeJSON(w, 200, v)
}

func (s *server) studentDone(w http.ResponseWriter, r *http.Request) {
	person, ok := s.personOf(r.PathValue("token"))
	if !ok {
		writeJSON(w, 404, map[string]string{"error": "unknown link"})
		return
	}
	var body struct {
		Evidence string `json:"evidence"`
		Step     string `json:"step"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	s.refresh()
	st, _ := s.snapshot()
	if st == nil || st.Current == nil {
		writeJSON(w, 409, map[string]string{"error": "no step is running"})
		return
	}
	mine := false
	for _, o := range st.Current.Owners {
		mine = mine || o == person
	}
	if !mine {
		writeJSON(w, 403, map[string]string{"error": "the current step is not yours"})
		return
	}
	if body.Step != "" && body.Step != st.Current.ID && body.Step != fmt.Sprint(st.Current.Index) {
		writeJSON(w, 403, map[string]string{"error": "you can only finish your own current step"})
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
		writeJSON(w, 502, map[string]string{"error": err.Error()})
		return
	}
	s.refresh()
	writeJSON(w, 200, map[string]any{"ok": true, "step": st.Current.ID, "person": person})
}

// ---- pages -------------------------------------------------------------------

func page(title, body, script string) []byte {
	return kit.Page("tutor", title, `<style>`+pageCSS+`</style>`+body+`<script>`+commonJS+script+`</script>`)
}

func (s *server) projectorPage(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(page("Tutor projector", projectorBody, "start("+jsq(s.prefix+"/api/projector")+",renderProjector)"))
}

func (s *server) teacherPage(w http.ResponseWriter, r *http.Request) {
	if !s.teacherOK(r.PathValue("key")) {
		http.Error(w, "teacher key required", 403)
		return
	}
	base := s.prefix + "/t/" + r.PathValue("key")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Write(page("Tutor teacher", teacherBody, "const BASE="+jsq(base)+";start("+jsq(s.prefix+"/api/t/"+r.PathValue("key"))+",renderTeacher)"))
}

func (s *server) studentPage(w http.ResponseWriter, r *http.Request) {
	tk := r.PathValue("token")
	if _, ok := s.personOf(tk); !ok {
		http.Error(w, "unknown link", 404)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Write(page("Tutor", studentBody, "const BASE="+jsq(s.prefix+"/s/"+tk)+";start("+jsq(s.prefix+"/api/s/"+tk)+",renderStudent)"))
}

func jsq(v string) string { b, _ := json.Marshal(v); return string(b) }

// ---- serve / join / stop -------------------------------------------------------

func dataDir() string {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		h, _ := os.UserHomeDir()
		base = filepath.Join(h, ".local", "share")
	}
	return filepath.Join(base, "bashy", "tutor")
}

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

func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", defaultAddr, "listen address")
	prefix := fs.String("prefix", "", "URL path prefix")
	poll := fs.Duration("poll", 3*time.Second, "status poll interval")
	data := fs.String("data", dataDir(), "data directory")
	asJSON := fs.Bool("json", false, "print links as JSON")
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	var n int
	if _, err := fmt.Sscanf(pos, "%d", &n); err != nil || n <= 0 || fmt.Sprint(n) != pos {
		return errors.New("usage: tutor serve N [--addr A] [--prefix P]")
	}
	bin := os.Getenv("BASHY_BIN")
	if bin == "" {
		bin = "bashy"
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
		fmt.Fprintln(os.Stderr, "tutor: warning: no status yet:", s.lastErr)
	}
	err = s.saveLocked()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	origin := "http://" + net.JoinHostPort(lanHost(ln.Addr().String()), fmt.Sprint(ln.Addr().(*net.TCPAddr).Port))
	s.mu.Lock()
	names := make([]string, 0, len(s.tok.Students))
	links := map[string]string{}
	for k, t := range s.tok.Students {
		names = append(names, k)
		links[k] = origin + s.prefix + "/s/" + t
	}
	s.mu.Unlock()
	sort.Strings(names)
	projector, teacher := origin+s.prefix+"/", origin+s.prefix+"/t/"+s.tok.Teacher
	if *asJSON {
		b, _ := json.Marshal(map[string]any{"sprint": n, "projector": projector, "teacher": teacher, "students": links})
		fmt.Println(string(b))
	} else {
		fmt.Printf("Projector: %s\nTeacher (keep private): %s\n", projector, teacher)
		for _, k := range names {
			fmt.Printf("Join %s: %s\n", k, links[k])
		}
	}
	os.Stdout.Sync()

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
		return errors.New("usage: tutor join URL [--done -m EVIDENCE]  (URL is your student link)")
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
	pos, err := parse(fs, args)
	if err != nil {
		return err
	}
	var n int
	if _, err := fmt.Sscanf(pos, "%d", &n); err != nil || n <= 0 {
		return errors.New("usage: tutor stop N")
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
	resp, err := c.Post("http://"+net.JoinHostPort(host, port)+t.Prefix+"/t/"+t.Teacher+"/stop", "application/json", nil)
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
		fmt.Printf("stopped tutor server for sprint %d\n", n)
	}
	return nil
}
