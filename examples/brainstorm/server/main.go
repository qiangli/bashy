package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

//go:embed index.html
var page []byte

var columns = []string{"Problems we notice", "Who needs help?", "Possible solutions", "What could we build?", "How would we test it?"}

type Card struct {
	ID     string          `json:"id"`
	Column int             `json:"column"`
	Text   string          `json:"text"`
	Name   string          `json:"name"`
	Owner  string          `json:"owner"`
	Votes  map[string]bool `json:"votes"`
}
type Board struct {
	Version  int    `json:"version"`
	Phase    string `json:"phase"`
	Teacher  string `json:"teacher"`
	Cards    []Card `json:"cards"`
	Revision int    `json:"revision"`
}
type app struct {
	mu    sync.Mutex
	board Board
	path  string
}

func token() string {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func validToken(s string) bool {
	if len(s) != 48 {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}
func save(path string, b Board) error {
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".board-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func load(dir string) (*app, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	a := &app{path: filepath.Join(dir, "board.json")}
	data, err := os.ReadFile(a.path)
	if errors.Is(err, os.ErrNotExist) {
		a.board = Board{Version: 1, Phase: "write", Teacher: token(), Cards: []Card{}}
		return a, save(a.path, a.board)
	}
	if err != nil {
		return nil, err
	}
	if err = json.Unmarshal(data, &a.board); err != nil {
		return nil, fmt.Errorf("saved board is invalid; preserve it for recovery: %w", err)
	}
	if a.board.Version != 1 || !validToken(a.board.Teacher) || !strings.Contains("|write|discuss|vote|closed|", "|"+a.board.Phase+"|") {
		return nil, errors.New("unsupported or invalid saved board")
	}
	return a, nil
}
func (a *app) teacher(r *http.Request) bool {
	return subtle.ConstantTimeCompare([]byte(r.Header.Get("X-Teacher-Key")), []byte(a.board.Teacher)) == 1
}
func participant(w http.ResponseWriter, r *http.Request) string {
	if c, e := r.Cookie("brainstorm-participant"); e == nil && validToken(c.Value) {
		return c.Value
	}
	id := token()
	http.SetCookie(w, &http.Cookie{Name: "brainstorm-participant", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 86400 * 30})
	return id
}
func respond(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	if r.Method != "GET" && r.Method != "POST" {
		fail(w, 405, "Method not supported")
		return
	}
	if r.Method == "POST" {
		if origin := r.Header.Get("Origin"); origin != "" {
			u, e := url.Parse(origin)
			if e != nil || u.Host != r.Host {
				fail(w, 403, "Use the board's own page")
				return
			}
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			fail(w, 403, "Cross-site request refused")
			return
		}
	}
	if r.URL.Path == "/" && r.Method == "GET" {
		participant(w, r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
		return
	}
	if r.URL.Path == "/healthz" && r.Method == "GET" {
		respond(w, map[string]bool{"ok": true})
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	id := participant(w, r)
	if r.URL.Path == "/api/state" && r.Method == "GET" {
		respond(w, a.view(id, a.teacher(r)))
		return
	}
	if r.URL.Path == "/api/export" && r.Method == "GET" {
		if !a.teacher(r) {
			fail(w, 403, "Teacher link required")
			return
		}
		if a.board.Phase == "write" {
			fail(w, 409, "Reveal ideas before exporting")
			return
		}
		if r.URL.Query().Get("format") == "markdown" {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=brainstorm.md")
			fmt.Fprint(w, "# Community Challenge Brainstorm\n\n")
			for i, col := range columns {
				fmt.Fprintf(w, "## %s\n\n", col)
				for _, c := range a.board.Cards {
					if c.Column == i {
						fmt.Fprintf(w, "- %s — %s (%d votes)\n", strings.ReplaceAll(c.Text, "\n", " "), c.Name, len(c.Votes))
					}
				}
				fmt.Fprintln(w)
			}
		} else {
			w.Header().Set("Content-Disposition", "attachment; filename=brainstorm.json")
			respond(w, a.view("", false))
		}
		return
	}
	if r.URL.Path != "/api/action" || r.Method != "POST" {
		fail(w, 404, "Not found")
		return
	}
	var req struct {
		Action string `json:"action"`
		ID     string `json:"id"`
		Column int    `json:"column"`
		Text   string `json:"text"`
		Name   string `json:"name"`
		Phase  string `json:"phase"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		fail(w, 400, "Invalid request")
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		fail(w, 400, "Invalid request")
		return
	}
	// Work on a copy: a failed disk save must not acknowledge or retain a mutation.
	raw, _ := json.Marshal(a.board)
	var next Board
	json.Unmarshal(raw, &next)
	idx := -1
	for i, c := range next.Cards {
		if c.ID == req.ID {
			idx = i
			break
		}
	}
	switch req.Action {
	case "add", "edit", "delete":
		if next.Phase != "write" {
			fail(w, 409, "Writing has ended; your draft is still in this browser")
			return
		}
		if req.Action != "add" && (idx < 0 || next.Cards[idx].Owner != id) {
			fail(w, 403, "You can change only your own cards")
			return
		}
		if req.Action == "delete" {
			next.Cards = append(next.Cards[:idx], next.Cards[idx+1:]...)
			break
		}
		req.Text = strings.TrimSpace(req.Text)
		req.Name = strings.TrimSpace(req.Name)
		if req.Text == "" || len([]rune(req.Text)) > 1000 || len([]rune(req.Name)) > 40 || req.Column < 0 || req.Column >= len(columns) {
			fail(w, 400, "Use 1–1000 characters for an idea and at most 40 for a nickname")
			return
		}
		if req.Name == "" {
			req.Name = "Anonymous"
		}
		if req.Action == "add" {
			if len(next.Cards) >= 1000 {
				fail(w, 409, "This board has reached its 1,000-card limit")
				return
			}
			next.Cards = append(next.Cards, Card{ID: token(), Column: req.Column, Text: req.Text, Name: req.Name, Owner: id, Votes: map[string]bool{}})
		} else {
			next.Cards[idx].Text = req.Text
			next.Cards[idx].Column = req.Column
			next.Cards[idx].Name = req.Name
		}
	case "vote":
		if next.Phase != "vote" {
			fail(w, 409, "Voting is not open")
			return
		}
		if idx < 0 {
			fail(w, 404, "Card not found")
			return
		}
		c := &next.Cards[idx]
		if c.Votes == nil {
			c.Votes = map[string]bool{}
		}
		if c.Votes[id] {
			delete(c.Votes, id)
		} else {
			used := 0
			for _, card := range next.Cards {
				if card.Votes[id] {
					used++
				}
			}
			if used >= 3 {
				fail(w, 409, "You have used all three votes; remove one to change your choice")
				return
			}
			c.Votes[id] = true
		}
	case "phase":
		if !a.teacher(r) {
			fail(w, 403, "Teacher link required")
			return
		}
		allowed := map[string]string{"write": "discuss", "discuss": "vote", "vote": "closed"}
		if allowed[next.Phase] != req.Phase {
			fail(w, 409, "Follow the phases in order")
			return
		}
		next.Phase = req.Phase
	default:
		fail(w, 400, "Unknown action")
		return
	}
	next.Revision++
	if err := save(a.path, next); err != nil {
		log.Printf("save failed: %v", err)
		fail(w, 500, "Could not save. Your change was not accepted; keep your draft and ask the teacher.")
		return
	}
	a.board = next
	respond(w, a.view(id, a.teacher(r)))
}
func (a *app) view(id string, teacher bool) any {
	cards := []map[string]any{}
	used := 0
	for _, c := range a.board.Cards {
		if c.Votes[id] {
			used++
		}
		if a.board.Phase == "write" && c.Owner != id {
			continue
		}
		cards = append(cards, map[string]any{"id": c.ID, "column": c.Column, "text": c.Text, "name": c.Name, "mine": c.Owner == id, "votes": len(c.Votes), "voted": c.Votes[id]})
	}
	return map[string]any{"title": "Community Challenge Brainstorm", "phase": a.board.Phase, "revision": a.board.Revision, "columns": columns, "cards": cards, "total": len(a.board.Cards), "teacher": teacher, "votesLeft": 3 - used}
}
func main() {
	data := flag.String("data", "", "persistent data directory")
	addr := flag.String("addr", "0.0.0.0:18091", "listen address")
	flag.Parse()
	if *data == "" {
		log.Fatal("-data is required")
	}
	a, err := load(*data)
	if err != nil {
		log.Fatal(err)
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("Cannot listen on %s: %v. Set BRAINSTORM_ADDR to another port.", *addr, err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	host, _, _ := net.SplitHostPort(*addr)
	hosts := []string{"127.0.0.1"}
	if host != "" && host != "0.0.0.0" && host != "::" {
		hosts = []string{host}
	} else {
		addrs, _ := net.InterfaceAddrs()
		for _, n := range addrs {
			ip, _, e := net.ParseCIDR(n.String())
			if e == nil && ip.To4() != nil && ip.IsPrivate() && !ip.IsLoopback() {
				hosts = append(hosts, ip.String())
			}
		}
	}
	sort.Strings(hosts)
	fmt.Println("\nCommunity Challenge Brainstorm — keep this terminal running")
	for _, h := range hosts {
		fmt.Printf("Student: http://%s/\n", net.JoinHostPort(h, port))
	}
	teacherHost := "127.0.0.1"
	for _, h := range hosts {
		if h != "127.0.0.1" {
			teacherHost = h
			break
		}
	}
	fmt.Printf("Teacher (keep private): http://%s/#teacher=%s\n", net.JoinHostPort(teacherHost, port), a.board.Teacher)
	fmt.Printf("Saved board: %s\nCtrl-C stops the server; restart resumes this board.\n\n", a.path)
	srv := &http.Server{Handler: a, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 30 * time.Second}
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
