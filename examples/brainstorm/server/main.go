package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
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

	"desk-chart/fence"
	kit "desk-kit"
	"desk-kit/render"
)

// defaultAddr is the recorded LAN-default exception to the desk loopback rule.
const defaultAddr = "0.0.0.0:18091"

func init() {
	// Markdown ```chart fences in cards reuse the C1 renderer.
	render.ChartSVG = fence.RenderFence
}

// renderMarkdown serves POST /api/render: Markdown in, sanitized HTML out.
// Cards are short plain text with no item assets, so local embeds degrade
// to links and remote embeds stay off.
func renderMarkdown(src []byte) ([]byte, error) {
	return render.Markdown(src, render.Options{})
}

//go:embed index.html
var indexHTML []byte

// page is the session page with the desk UI kit CSS/JS inlined.
var page = buildPage(indexHTML)

// appBar is the kit top bar; the item header carries the session id,
// phase status, connection pill and summary the page script updates.
var appBar = kit.AppBar("brainstorm", "./",
	`<div class="desk-item-header"><span class="desk-id" id="sessionId">main</span>`+
		`<span class="desk-badge" id="statusBadge" data-status="pending">pending</span>`+
		`<span class="desk-pill" id="connection">Connecting…</span><span class="muted" id="summary"></span></div>`, "")

func buildPage(src []byte) []byte {
	p := bytes.Replace(src, []byte("<style>"), []byte("<style>"+kit.CSS()+"\n"), 1)
	p = bytes.Replace(p, []byte("<!--desk-appbar-->"), []byte(appBar), 1)
	return bytes.Replace(p, []byte("</main><script>"), []byte("</main><script>"+kit.JS()+"</script><script>"), 1)
}

var defaultColumns = []string{"Problems we notice", "Who needs help?", "Possible solutions", "What could we build?", "How would we test it?"}
var defaultPrompts = []string{
	"What is difficult in your school, team or community?",
	"Who experiences this problem? Give an example.",
	"How could we help? What would it do?",
	"What small prototype can our team make?",
	"How would we know the solution helps?",
}

const defaultVoteBudget = 3

var sessionNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

type Card struct {
	ID          string          `json:"id"`
	Column      int             `json:"column"`
	Text        string          `json:"text"`
	Name        string          `json:"name"`
	Owner       string          `json:"owner"`
	Votes       map[string]bool `json:"votes"`
	Preset      bool            `json:"preset,omitempty"`
	PriorVotes  int             `json:"priorVotes,omitempty"`
	OriginRound int             `json:"originRound,omitempty"`
}

type PresetCardInput struct {
	ID          string `json:"id,omitempty"`
	Column      int    `json:"column,omitempty"`
	Text        string `json:"text"`
	Name        string `json:"name,omitempty"`
	Votes       int    `json:"votes,omitempty"`
	PriorVotes  int    `json:"priorVotes,omitempty"`
	OriginRound int    `json:"originRound,omitempty"`
}

type RoundFilter struct {
	SelectedIDs  []string `json:"selectedIds,omitempty"`
	MinVotes     int      `json:"minVotes,omitempty"`
	TopN         int      `json:"topN,omitempty"`
	FilterColumn *int     `json:"filterColumn,omitempty"`
	FilterAuthor string   `json:"filterAuthor,omitempty"`
	FilterQuery  string   `json:"filterQuery,omitempty"`
	KeepVotes    bool     `json:"keepVotes,omitempty"`
	StartPhase   string   `json:"startPhase,omitempty"`
	Summary      string   `json:"summary,omitempty"`
}

type RoundCard struct {
	ID          string `json:"id"`
	Column      int    `json:"column"`
	Text        string `json:"text"`
	Name        string `json:"name"`
	Votes       int    `json:"votes"`
	PriorVotes  int    `json:"priorVotes,omitempty"`
	Preset      bool   `json:"preset,omitempty"`
	OriginRound int    `json:"originRound,omitempty"`
	Advanced    bool   `json:"advanced"`
}

type RoundSnapshot struct {
	Round         int         `json:"round"`
	EndedAt       string      `json:"endedAt,omitempty"`
	Phase         string      `json:"phase"`
	Cards         []RoundCard `json:"cards"`
	RawCards      []Card      `json:"rawCards,omitempty"`
	Filter        RoundFilter `json:"filter"`
	TotalCount    int         `json:"totalCount"`
	AdvancedCount int         `json:"advancedCount"`
}

type Session struct {
	Version     int             `json:"version"`
	Name        string          `json:"name"`
	Title       string          `json:"title"`
	Columns     []string        `json:"columns"`
	Prompts     []string        `json:"prompts"`
	VoteBudget  int             `json:"voteBudget"`
	Facilitator string          `json:"facilitator"`
	Phase       string          `json:"phase"`
	Round       int             `json:"round,omitempty"`
	Rounds      []RoundSnapshot `json:"rounds,omitempty"`
	FromSession string          `json:"fromSession,omitempty"`
	FromRound   int             `json:"fromRound,omitempty"`
	Cards       []Card          `json:"cards"`
	Revision    int             `json:"revision"`
	// Private sessions admit only the facilitator and invited members.
	// Sessions saved before this field existed decode as open (false).
	Private bool              `json:"private,omitempty"`
	Members map[string]Member `json:"members,omitempty"` // participant cookie -> member
	Invites map[string]string `json:"invites,omitempty"` // single-use invite code -> label
}

// Member is an invited participant; Handle is the public, non-secret
// identifier the facilitator uses to revoke them.
type Member struct {
	Handle string `json:"handle"`
	Label  string `json:"label"`
}
type app struct {
	mu       sync.Mutex
	dir      string
	prefix   string
	sessions map[string]*Session
}

func token() string            { return kit.NewToken() }
func validToken(s string) bool { return kit.ValidToken(s) }
func sessionPath(dir, name string) string {
	return filepath.Join(dir, "sessions", name+".json")
}
func saveSession(path string, s Session) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return kit.AtomicWrite(path, data)
}
func validPhase(p string) bool {
	return p == "write" || p == "discuss" || p == "vote" || p == "closed"
}
func normalizeSession(s *Session) error {
	if s.Version != 1 && s.Version != 2 {
		return errors.New("unsupported saved session version")
	}
	if s.Name == "" || !sessionNameRe.MatchString(s.Name) {
		return errors.New("unsupported or invalid saved session")
	}
	if !validToken(s.Facilitator) || !validPhase(s.Phase) {
		return errors.New("unsupported or invalid saved session")
	}
	if len(s.Columns) == 0 || len(s.Columns) > 8 {
		return errors.New("unsupported or invalid saved session")
	}
	if s.VoteBudget < 1 || s.VoteBudget > 10 {
		return errors.New("unsupported or invalid saved session")
	}
	if s.Title == "" {
		s.Title = "Team Brainstorm"
	}
	if s.Prompts == nil {
		s.Prompts = make([]string, len(s.Columns))
	}
	if s.Cards == nil {
		s.Cards = []Card{}
	}
	if s.Round < 1 {
		s.Round = len(s.Rounds) + 1
	}
	for i := range s.Cards {
		if s.Cards[i].Votes == nil {
			s.Cards[i].Votes = map[string]bool{}
		}
		if s.Cards[i].OriginRound < 1 {
			s.Cards[i].OriginRound = 1
		}
	}
	for i := range s.Rounds {
		if s.Rounds[i].Cards == nil {
			s.Rounds[i].Cards = []RoundCard{}
		}
		for j := range s.Rounds[i].RawCards {
			if s.Rounds[i].RawCards[j].Votes == nil {
				s.Rounds[i].RawCards[j].Votes = map[string]bool{}
			}
		}
	}
	s.Version = 2
	return nil
}

var errSessionNotFound = errors.New("session not found")
var errSessionExists = errors.New("session already exists")

// loadLocked returns the named session without creating it. Callers must
// hold a.mu. Only the explicit create action mints a new session (see
// createLocked); reads of unknown names return errSessionNotFound so guests
// cannot reserve names or fill disk. Saved sessions on disk still load on
// first read.
func (a *app) loadLocked(name string) (*Session, error) {
	if !sessionNameRe.MatchString(name) {
		return nil, errors.New("use 1-64 letters, digits, - or _ for a session name")
	}
	if s, ok := a.sessions[name]; ok {
		return s, nil
	}
	raw, err := os.ReadFile(sessionPath(a.dir, name))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errSessionNotFound
		}
		return nil, err
	}
	s := &Session{}
	if err = json.Unmarshal(raw, s); err != nil {
		return nil, fmt.Errorf("saved session %q is invalid; preserve it for recovery: %w", name, err)
	}
	if err = normalizeSession(s); err != nil {
		return nil, fmt.Errorf("unsupported or invalid saved session %q", name)
	}
	a.sessions[name] = s
	return s, nil
}

// createLocked persists a prepared new session. Callers must hold a.mu and
// have validated the session fields. The minted facilitator capability is
// revealed only in the create-action response, never in state or exports.
func (a *app) createLocked(s *Session) (*Session, error) {
	if !sessionNameRe.MatchString(s.Name) {
		return nil, errors.New("use 1-64 letters, digits, - or _ for a session name")
	}
	if _, ok := a.sessions[s.Name]; ok {
		return nil, errSessionExists
	}
	if _, err := os.Stat(sessionPath(a.dir, s.Name)); err == nil {
		return nil, errSessionExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if s.Name == "main" {
		if _, err := os.Stat(filepath.Join(a.dir, "board.json")); err == nil {
			return nil, errSessionExists
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	if err := saveSession(sessionPath(a.dir, s.Name), *s); err != nil {
		return nil, err
	}
	a.sessions[s.Name] = s
	return s, nil
}

// ensureMainLocked returns the default session, minting it at startup when
// absent. A legacy classroom board.json (version 1, no session fields) is
// honored as session "main" without touching the old file; new saves always
// go to sessions/main.json. Callers must hold a.mu.
func (a *app) ensureMainLocked() (*Session, error) {
	if s, err := a.loadLocked("main"); err == nil {
		return s, nil
	} else if !errors.Is(err, errSessionNotFound) {
		return nil, err
	}
	s := &Session{Version: 2, Name: "main", Title: "Team Brainstorm",
		Columns:    append([]string{}, defaultColumns...),
		Prompts:    append([]string{}, defaultPrompts...),
		VoteBudget: defaultVoteBudget, Facilitator: token(), Phase: "write", Round: 1, Cards: []Card{}}
	if legacy, lerr := os.ReadFile(filepath.Join(a.dir, "board.json")); lerr == nil {
		var old struct {
			Version  int    `json:"version"`
			Phase    string `json:"phase"`
			Teacher  string `json:"teacher"`
			Cards    []Card `json:"cards"`
			Revision int    `json:"revision"`
		}
		if jerr := json.Unmarshal(legacy, &old); jerr != nil {
			return nil, fmt.Errorf("saved board is invalid; preserve it for recovery: %w", jerr)
		}
		if old.Version != 1 || !validToken(old.Teacher) || !validPhase(old.Phase) {
			return nil, errors.New("unsupported or invalid saved board")
		}
		s.Facilitator = old.Teacher
		s.Phase = old.Phase
		s.Cards = old.Cards
		if s.Cards == nil {
			s.Cards = []Card{}
		}
		s.Revision = old.Revision
		s.Title = "Community Challenge Brainstorm"
	} else if !errors.Is(lerr, os.ErrNotExist) {
		return nil, lerr
	}
	if err := saveSession(sessionPath(a.dir, s.Name), *s); err != nil {
		return nil, err
	}
	a.sessions[s.Name] = s
	return s, nil
}
func handleOf(id string) string {
	h := sha256.Sum256([]byte("brainstorm-member:" + id))
	return hex.EncodeToString(h[:6])
}

// allowed reports whether the caller may touch the session at all.
func (s *Session) allowed(id string, fac bool) bool {
	if !s.Private || fac {
		return true
	}
	_, ok := s.Members[id]
	return ok
}
func sessionNotFound(w http.ResponseWriter) {
	fail(w, 404, "Session not found. Ask the facilitator to create it.")
}
func (a *app) facilitator(r *http.Request, s *Session) bool {
	got := kit.FacilitatorKey(r)
	if got == "" {
		got = r.Header.Get("X-Teacher-Key")
	}
	return kit.Grant(got, s.Facilitator)
}
func participant(w http.ResponseWriter, r *http.Request) string {
	return kit.ParticipantCookie(w, r, "brainstorm-participant")
}
func respond(w http.ResponseWriter, v any)             { kit.WriteJSON(w, v) }
func fail(w http.ResponseWriter, code int, msg string) { kit.Fail(w, code, msg) }
func (a *app) stripPrefix(path string) (string, bool) {
	return kit.StripPrefix(a.prefix, path)
}

func sameOrigin(r *http.Request, u *url.URL) bool { return kit.SameOrigin(r, u) }

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ContractHandler("brainstorm", http.HandlerFunc(a.serveHTTP)).ServeHTTP(w, r)
}

func (a *app) serveHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ApplySecurityHeaders(w)
	if !kit.GuardPOST(w, r, "session's own") {
		return
	}
	path, ok := a.stripPrefix(r.URL.Path)
	if !ok {
		fail(w, 404, "Not found")
		return
	}
	if path == "/" && r.Method == "GET" {
		participant(w, r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(page)
		return
	}
	if path == "/healthz" && r.Method == "GET" {
		respond(w, map[string]bool{"ok": true})
		return
	}
	if path == "/api/render" {
		kit.ServeRender(w, r, renderMarkdown)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	id := participant(w, r)
	sessionName := r.URL.Query().Get("session")
	if sessionName == "" {
		sessionName = "main"
	}
	if path == "/api/state" && r.Method == "GET" {
		s, err := a.loadLocked(sessionName)
		if err != nil {
			if errors.Is(err, errSessionNotFound) {
				sessionNotFound(w)
			} else {
				fail(w, 400, err.Error())
			}
			return
		}
		isFac := a.facilitator(r, s)
		if !s.allowed(id, isFac) {
			sessionNotFound(w)
			return
		}
		roundNum, ok := parseRoundParam(w, r, s)
		if !ok {
			return
		}
		respond(w, a.viewForRound(id, s, isFac, roundNum))
		return
	}
	if path == "/api/export" && r.Method == "GET" {
		s, err := a.loadLocked(sessionName)
		if err != nil {
			if errors.Is(err, errSessionNotFound) {
				sessionNotFound(w)
			} else {
				fail(w, 400, err.Error())
			}
			return
		}
		if !s.allowed(id, a.facilitator(r, s)) {
			sessionNotFound(w)
			return
		}
		if !a.facilitator(r, s) {
			fail(w, 403, "Facilitator link required")
			return
		}
		roundNum, ok := parseRoundParam(w, r, s)
		if !ok {
			return
		}
		if (roundNum == 0 || roundNum == s.Round) && s.Phase == "write" {
			fail(w, 409, "Reveal ideas before exporting")
			return
		}
		if r.URL.Query().Get("format") == "markdown" {
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.Header().Set("Content-Disposition", "attachment; filename=brainstorm-"+s.Name+".md")
			writeMarkdownExport(w, s, roundNum)
		} else {
			w.Header().Set("Content-Disposition", "attachment; filename=brainstorm-"+s.Name+".json")
			respond(w, a.viewForRound("", s, false, roundNum))
		}
		return
	}
	if path != "/api/action" || r.Method != "POST" {
		fail(w, 404, "Not found")
		return
	}
	var req actionReq
	if !kit.DecodeAction(w, r, 16384, &req) {
		return
	}
	// Card submissions already use add; their text distinguishes them from
	// the desk add action, which creates a session.
	if deskActions[req.Action] && !(req.Action == "add" && req.ID == "" && req.Title == "" && req.Body == "" && len(req.Fields) == 0 && len(req.Presets) == 0 && req.FromSession == "") {
		a.desk(w, r, id, &req)
		return
	}
	if req.Session == "" {
		req.Session = "main"
	}
	if !sessionNameRe.MatchString(req.Session) {
		fail(w, 400, "Use 1-64 letters, digits, - or _ for a session name")
		return
	}
	if req.Action == "create" {
		if _, exists := a.sessions[req.Session]; exists {
			fail(w, 409, "A session with that name already exists")
			return
		}
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = "Team Brainstorm"
		}
		if len([]rune(title)) > 80 {
			fail(w, 400, "Keep the title within 80 characters")
			return
		}
		columns := req.Columns
		if columns == nil {
			columns = append([]string{}, defaultColumns...)
		}
		if len(columns) < 1 || len(columns) > 8 {
			fail(w, 400, "Use 1-8 columns")
			return
		}
		for _, c := range columns {
			if strings.TrimSpace(c) == "" || len([]rune(c)) > 40 {
				fail(w, 400, "Keep each column within 1-40 characters")
				return
			}
		}
		prompts := req.Prompts
		if prompts == nil {
			if req.Columns == nil {
				prompts = append([]string{}, defaultPrompts...)
			} else {
				prompts = make([]string, len(columns))
			}
		}
		if len(prompts) != len(columns) {
			fail(w, 400, "Give one prompt per column")
			return
		}
		for _, p := range prompts {
			if len([]rune(p)) > 140 {
				fail(w, 400, "Keep each prompt within 140 characters")
				return
			}
		}
		budget := req.VoteBudget
		if budget == 0 {
			budget = defaultVoteBudget
		}
		if budget < 1 || budget > 10 {
			fail(w, 400, "Use a vote budget of 1-10")
			return
		}
		startPhase := "write"
		if req.StartPhase != "" {
			if req.StartPhase != "write" && req.StartPhase != "discuss" && req.StartPhase != "vote" {
				fail(w, 400, "Start a session in write, discuss, or vote phase")
				return
			}
			startPhase = req.StartPhase
		}
		s := &Session{Version: 2, Name: req.Session, Title: title, Columns: columns,
			Prompts: prompts, VoteBudget: budget, Facilitator: token(), Phase: startPhase, Round: 1, Cards: []Card{}}
		if req.Private {
			s.Private = true
			s.Members = map[string]Member{}
			s.Invites = map[string]string{}
		}
		if req.FromSession != "" || len(req.Presets) > 0 {
			if code, msg, ok := a.seedSessionLocked(r, id, s, &req); !ok {
				fail(w, code, msg)
				return
			}
		}
		if _, err := a.createLocked(s); err != nil {
			if errors.Is(err, errSessionExists) {
				fail(w, 409, "A session with that name already exists")
			} else if !sessionNameRe.MatchString(req.Session) {
				fail(w, 400, err.Error())
			} else {
				log.Printf("create failed: %v", err)
				fail(w, 500, "Could not save. Try again.")
			}
			return
		}
		v := a.view(id, s, true)
		v["facilitatorKey"] = s.Facilitator
		respond(w, v)
		return
	}
	s, err := a.loadLocked(req.Session)
	if err != nil {
		if errors.Is(err, errSessionNotFound) {
			sessionNotFound(w)
		} else {
			fail(w, 400, err.Error())
		}
		return
	}
	isFac := a.facilitator(r, s)
	if req.Action == "join" {
		if !s.Private {
			respond(w, a.view(id, s, isFac))
			return
		}
		if s.allowed(id, isFac) {
			respond(w, a.view(id, s, isFac))
			return
		}
		label, ok := s.Invites[req.Invite]
		if req.Invite == "" || !ok {
			sessionNotFound(w)
			return
		}
		next := *s
		next.Members = map[string]Member{}
		for k, v := range s.Members {
			next.Members[k] = v
		}
		next.Invites = map[string]string{}
		for k, v := range s.Invites {
			if k != req.Invite {
				next.Invites[k] = v
			}
		}
		next.Members[id] = Member{Handle: handleOf(id), Label: label}
		next.Revision++
		if err := saveSession(sessionPath(a.dir, next.Name), next); err != nil {
			log.Printf("save failed: %v", err)
			fail(w, 500, "Could not save. Try again.")
			return
		}
		*s = next
		respond(w, a.view(id, s, false))
		return
	}
	if !s.allowed(id, isFac) {
		sessionNotFound(w)
		return
	}
	// Work on a copy: a failed disk save must not acknowledge or retain a mutation.
	raw, _ := json.Marshal(s)
	var next Session
	json.Unmarshal(raw, &next)
	if next.Round < 1 {
		next.Round = len(next.Rounds) + 1
	}
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
		canModify := idx >= 0 && (next.Cards[idx].Owner == id || (isFac && next.Cards[idx].Preset))
		if req.Action != "add" && !canModify {
			fail(w, 403, "You can change only your own cards")
			return
		}
		if req.Action == "delete" {
			next.Cards = append(next.Cards[:idx], next.Cards[idx+1:]...)
			break
		}
		req.Text = strings.TrimSpace(req.Text)
		req.Name = strings.TrimSpace(req.Name)
		if req.Text == "" || len([]rune(req.Text)) > 1000 || len([]rune(req.Name)) > 40 || req.Column < 0 || req.Column >= len(next.Columns) {
			fail(w, 400, "Use 1–1000 characters for an idea and at most 40 for a nickname")
			return
		}
		if req.Name == "" {
			req.Name = "Anonymous"
		}
		if req.Action == "add" {
			if len(next.Cards) >= 1000 {
				fail(w, 409, "This session has reached its 1,000-card limit")
				return
			}
			next.Cards = append(next.Cards, Card{ID: token(), Column: req.Column, Text: req.Text, Name: req.Name, Owner: id, Votes: map[string]bool{}, Preset: isFac && req.Preset, PriorVotes: max(0, req.PriorVotes), OriginRound: next.Round})
		} else {
			next.Cards[idx].Text = req.Text
			next.Cards[idx].Column = req.Column
			next.Cards[idx].Name = req.Name
		}
	case "preset":
		if !isFac {
			fail(w, 403, "Facilitator link required")
			return
		}
		if next.Phase == "closed" {
			fail(w, 409, "Start a new round before adding preset cards")
			return
		}
		if code, msg, ok := a.seedSessionLocked(r, id, &next, &req); !ok {
			fail(w, code, msg)
			return
		}
	case "next-round":
		if !isFac {
			fail(w, 403, "Facilitator link required")
			return
		}
		if code, msg, ok := advanceToNextRound(&next, &req); !ok {
			fail(w, code, msg)
			return
		}
	case "reopen-round":
		if !isFac {
			fail(w, 403, "Facilitator link required")
			return
		}
		if code, msg, ok := reopenRoundTo(&next, req.Round); !ok {
			fail(w, code, msg)
			return
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
			if used >= next.VoteBudget {
				fail(w, 409, fmt.Sprintf("You have used all %d votes; remove one to change your choice", next.VoteBudget))
				return
			}
			c.Votes[id] = true
		}
	case "invite", "revoke":
		if !isFac {
			fail(w, 403, "Facilitator link required")
			return
		}
		if !next.Private {
			fail(w, 409, "This session is open to everyone with the link")
			return
		}
		if next.Members == nil {
			next.Members = map[string]Member{}
		}
		if next.Invites == nil {
			next.Invites = map[string]string{}
		}
		if req.Action == "invite" {
			label := strings.TrimSpace(req.Name)
			if len([]rune(label)) > 40 {
				fail(w, 400, "Keep the invitee label within 40 characters")
				return
			}
			if len(next.Invites)+len(next.Members) >= 500 {
				fail(w, 409, "This session has reached its participant limit")
				return
			}
			code := token()
			next.Invites[code] = label
			next.Revision++
			if err := saveSession(sessionPath(a.dir, next.Name), next); err != nil {
				log.Printf("save failed: %v", err)
				fail(w, 500, "Could not save. Try again.")
				return
			}
			*s = next
			v := a.view(id, s, true)
			v["inviteCode"] = code
			respond(w, v)
			return
		}
		found := false
		for k, m := range next.Members {
			if m.Handle == req.ID {
				delete(next.Members, k)
				found = true
			}
		}
		if _, ok := next.Invites[req.ID]; ok {
			delete(next.Invites, req.ID)
			found = true
		}
		if !found {
			fail(w, 404, "No such participant or invitation")
			return
		}
	case "phase":
		if !isFac {
			fail(w, 403, "Facilitator link required")
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
	if err := saveSession(sessionPath(a.dir, next.Name), next); err != nil {
		log.Printf("save failed: %v", err)
		fail(w, 500, "Could not save. Your change was not accepted; keep your draft and ask the facilitator.")
		return
	}
	*s = next
	respond(w, a.view(id, s, isFac))
}

func parseRoundParam(w http.ResponseWriter, r *http.Request, s *Session) (int, bool) {
	q := strings.TrimSpace(r.URL.Query().Get("round"))
	if q == "" {
		return 0, true
	}
	n, err := strconv.Atoi(q)
	maxRound := s.Round
	if maxRound < 1 {
		maxRound = len(s.Rounds) + 1
	}
	if err != nil || n < 1 || n > maxRound {
		fail(w, 404, "Round not found")
		return 0, false
	}
	return n, true
}

func writeMarkdownExport(w http.ResponseWriter, s *Session, roundNum int) {
	curRound := s.Round
	if curRound < 1 {
		curRound = len(s.Rounds) + 1
	}
	if roundNum > 0 && roundNum < curRound && roundNum <= len(s.Rounds) {
		snap := s.Rounds[roundNum-1]
		fmt.Fprintf(w, "# %s (Round %d)\n\n", s.Title, snap.Round)
		if snap.Filter.Summary != "" {
			fmt.Fprintf(w, "Filter: %s (%d of %d advanced)\n\n", snap.Filter.Summary, snap.AdvancedCount, snap.TotalCount)
		}
		for i, col := range s.Columns {
			fmt.Fprintf(w, "## %s\n\n", col)
			for _, c := range snap.Cards {
				if c.Column == i {
					status := "archived"
					if c.Advanced {
						status = "advanced"
					}
					fmt.Fprintf(w, "- %s — %s (%d votes, %s)\n", strings.ReplaceAll(c.Text, "\n", " "), c.Name, c.Votes, status)
				}
			}
			fmt.Fprintln(w)
		}
		return
	}
	fmt.Fprintf(w, "# %s\n\n", s.Title)
	for i, col := range s.Columns {
		fmt.Fprintf(w, "## %s\n\n", col)
		for _, c := range s.Cards {
			if c.Column == i {
				fmt.Fprintf(w, "- %s — %s (%d votes)\n", strings.ReplaceAll(c.Text, "\n", " "), c.Name, len(c.Votes))
			}
		}
		fmt.Fprintln(w)
	}
	if len(s.Rounds) > 0 {
		fmt.Fprintf(w, "## Round History\n\n")
		for _, snap := range s.Rounds {
			fmt.Fprintf(w, "### Round %d (%d of %d advanced — %s)\n\n", snap.Round, snap.AdvancedCount, snap.TotalCount, snap.Filter.Summary)
			for _, c := range snap.Cards {
				status := "archived"
				if c.Advanced {
					status = "advanced"
				}
				fmt.Fprintf(w, "- %s — %s (%d votes, %s)\n", strings.ReplaceAll(c.Text, "\n", " "), c.Name, c.Votes, status)
			}
			fmt.Fprintln(w)
		}
	}
}

func publicRoundSnapshots(rounds []RoundSnapshot) []RoundSnapshot {
	if len(rounds) == 0 {
		return []RoundSnapshot{}
	}
	out := make([]RoundSnapshot, len(rounds))
	for i, r := range rounds {
		cards := make([]RoundCard, len(r.Cards))
		copy(cards, r.Cards)
		f := r.Filter
		if len(r.Filter.SelectedIDs) > 0 {
			f.SelectedIDs = append([]string{}, r.Filter.SelectedIDs...)
		}
		out[i] = RoundSnapshot{
			Round:         r.Round,
			EndedAt:       r.EndedAt,
			Phase:         r.Phase,
			Cards:         cards,
			Filter:        f,
			TotalCount:    r.TotalCount,
			AdvancedCount: r.AdvancedCount,
		}
	}
	return out
}

func filterCards(cards []Card, f RoundFilter, cols []string) ([]Card, map[string]bool, string) {
	anyCurrentVotes := false
	for _, c := range cards {
		if len(c.Votes) > 0 {
			anyCurrentVotes = true
			break
		}
	}
	cardVotes := func(c Card) int {
		if anyCurrentVotes {
			return len(c.Votes)
		}
		if len(c.Votes) > 0 {
			return len(c.Votes)
		}
		return c.PriorVotes
	}
	sel := map[string]bool{}
	for _, id := range f.SelectedIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			sel[id] = true
		}
	}
	authorFilter := strings.ToLower(strings.TrimSpace(f.FilterAuthor))
	queryFilter := strings.ToLower(strings.TrimSpace(f.FilterQuery))
	type indexedCard struct {
		idx  int
		card Card
	}
	var matched []indexedCard
	for i, c := range cards {
		if len(sel) > 0 && !sel[c.ID] {
			continue
		}
		if f.MinVotes > 0 && cardVotes(c) < f.MinVotes {
			continue
		}
		if f.FilterColumn != nil && *f.FilterColumn >= 0 && c.Column != *f.FilterColumn {
			continue
		}
		if authorFilter != "" && !strings.Contains(strings.ToLower(c.Name), authorFilter) {
			continue
		}
		if queryFilter != "" && !strings.Contains(strings.ToLower(c.Text+" "+c.Name), queryFilter) {
			continue
		}
		matched = append(matched, indexedCard{idx: i, card: c})
	}
	if f.TopN > 0 && len(matched) > f.TopN {
		ranked := append([]indexedCard{}, matched...)
		sort.SliceStable(ranked, func(i, j int) bool {
			vi, vj := cardVotes(ranked[i].card), cardVotes(ranked[j].card)
			if vi != vj {
				return vi > vj
			}
			if ranked[i].card.PriorVotes != ranked[j].card.PriorVotes {
				return ranked[i].card.PriorVotes > ranked[j].card.PriorVotes
			}
			return ranked[i].idx < ranked[j].idx
		})
		ranked = ranked[:f.TopN]
		keepIdx := map[int]bool{}
		for _, r := range ranked {
			keepIdx[r.idx] = true
		}
		var topMatched []indexedCard
		for _, m := range matched {
			if keepIdx[m.idx] {
				topMatched = append(topMatched, m)
			}
		}
		matched = topMatched
	}
	advancedIDs := map[string]bool{}
	kept := make([]Card, 0, len(matched))
	for _, m := range matched {
		advancedIDs[m.card.ID] = true
		c := m.card
		votes := map[string]bool{}
		for k, v := range m.card.Votes {
			votes[k] = v
		}
		c.Votes = votes
		kept = append(kept, c)
	}
	var parts []string
	if len(sel) > 0 {
		parts = append(parts, fmt.Sprintf("manual selection (%d)", len(sel)))
	}
	if f.MinVotes > 0 {
		parts = append(parts, fmt.Sprintf("min %d vote(s)", f.MinVotes))
	}
	if f.TopN > 0 {
		parts = append(parts, fmt.Sprintf("top %d", f.TopN))
	}
	if f.FilterColumn != nil && *f.FilterColumn >= 0 {
		colLabel := fmt.Sprintf("column %d", *f.FilterColumn)
		if *f.FilterColumn < len(cols) {
			colLabel = fmt.Sprintf("column %q", cols[*f.FilterColumn])
		}
		parts = append(parts, colLabel)
	}
	if f.FilterAuthor != "" {
		parts = append(parts, fmt.Sprintf("author %q", strings.TrimSpace(f.FilterAuthor)))
	}
	if f.FilterQuery != "" {
		parts = append(parts, fmt.Sprintf("query %q", strings.TrimSpace(f.FilterQuery)))
	}
	summary := "All cards"
	if len(parts) > 0 {
		summary = strings.Join(parts, ", ")
	}
	return kept, advancedIDs, summary
}

func advanceToNextRound(s *Session, req *actionReq) (int, string, bool) {
	if len(s.Rounds) >= 50 {
		return 409, "This session has reached its 50-round limit", false
	}
	startPhase := strings.TrimSpace(req.StartPhase)
	if startPhase == "" {
		startPhase = strings.TrimSpace(req.Phase)
	}
	if startPhase == "" {
		startPhase = "write"
	}
	if startPhase != "write" && startPhase != "discuss" && startPhase != "vote" {
		return 400, "Start the next round in write, discuss, or vote phase", false
	}
	if req.MinVotes < 0 || req.TopN < 0 {
		return 400, "Vote filters cannot be negative", false
	}
	if req.FilterColumn != nil && (*req.FilterColumn < -1 || *req.FilterColumn >= len(s.Columns)) {
		return 400, "Invalid column filter", false
	}
	f := RoundFilter{
		SelectedIDs:  req.SelectedIDs,
		MinVotes:     req.MinVotes,
		TopN:         req.TopN,
		FilterColumn: req.FilterColumn,
		FilterAuthor: strings.TrimSpace(req.FilterAuthor),
		FilterQuery:  strings.TrimSpace(req.FilterQuery),
		KeepVotes:    req.KeepVotes,
		StartPhase:   startPhase,
	}
	kept, advancedIDs, summary := filterCards(s.Cards, f, s.Columns)
	f.Summary = summary
	if s.Round < 1 {
		s.Round = len(s.Rounds) + 1
	}
	roundCards := make([]RoundCard, 0, len(s.Cards))
	rawCards := make([]Card, 0, len(s.Cards))
	for _, c := range s.Cards {
		orig := c.OriginRound
		if orig < 1 {
			orig = 1
		}
		roundCards = append(roundCards, RoundCard{
			ID:          c.ID,
			Column:      c.Column,
			Text:        c.Text,
			Name:        c.Name,
			Votes:       len(c.Votes),
			PriorVotes:  c.PriorVotes,
			Preset:      c.Preset,
			OriginRound: orig,
			Advanced:    advancedIDs[c.ID],
		})
		votesCopy := map[string]bool{}
		for k, v := range c.Votes {
			votesCopy[k] = v
		}
		rawCopy := c
		rawCopy.OriginRound = orig
		rawCopy.Votes = votesCopy
		rawCards = append(rawCards, rawCopy)
	}
	snap := RoundSnapshot{
		Round:         s.Round,
		EndedAt:       time.Now().UTC().Format(time.RFC3339),
		Phase:         s.Phase,
		Cards:         roundCards,
		RawCards:      rawCards,
		Filter:        f,
		TotalCount:    len(s.Cards),
		AdvancedCount: len(kept),
	}
	s.Rounds = append(s.Rounds, snap)
	for i := range kept {
		if kept[i].OriginRound < 1 {
			kept[i].OriginRound = s.Round
		}
		if len(kept[i].Votes) > 0 || s.Phase == "vote" || s.Phase == "closed" {
			kept[i].PriorVotes = len(kept[i].Votes)
		}
		kept[i].Preset = true
		if !req.KeepVotes {
			kept[i].Votes = map[string]bool{}
		}
	}
	s.Cards = kept
	s.Round++
	s.Phase = startPhase
	return 200, "", true
}

func reopenRoundTo(s *Session, targetRound int) (int, string, bool) {
	if s.Round < 1 {
		s.Round = len(s.Rounds) + 1
	}
	if targetRound < 1 || targetRound >= s.Round || targetRound > len(s.Rounds) {
		return 400, "Choose a completed earlier round to reopen", false
	}
	snap := s.Rounds[targetRound-1]
	if len(snap.RawCards) > 0 {
		restored := make([]Card, len(snap.RawCards))
		for i, rc := range snap.RawCards {
			votes := map[string]bool{}
			for k, v := range rc.Votes {
				votes[k] = v
			}
			restored[i] = rc
			restored[i].Votes = votes
		}
		s.Cards = restored
	} else {
		restored := make([]Card, len(snap.Cards))
		for i, rc := range snap.Cards {
			votes := map[string]bool{}
			for v := 0; v < rc.Votes; v++ {
				votes[fmt.Sprintf("prior-%d", v)] = true
			}
			restored[i] = Card{
				ID:          rc.ID,
				Column:      rc.Column,
				Text:        rc.Text,
				Name:        rc.Name,
				Owner:       deskOwner,
				Votes:       votes,
				Preset:      rc.Preset,
				PriorVotes:  rc.PriorVotes,
				OriginRound: rc.OriginRound,
			}
		}
		s.Cards = restored
	}
	phase := snap.Phase
	if !validPhase(phase) {
		phase = "vote"
	}
	s.Phase = phase
	s.Round = targetRound
	s.Rounds = s.Rounds[:targetRound-1]
	return 200, "", true
}

// seedSessionLocked imports cards from req.FromSession (optional round + filters)
// and/or appends preset cards from req.Presets (or req.Text when action == "preset").
// Callers must hold a.mu.
func (a *app) seedSessionLocked(r *http.Request, callerID string, dst *Session, req *actionReq) (int, string, bool) {
	if dst.Round < 1 {
		dst.Round = len(dst.Rounds) + 1
	}
	var toAdd []Card
	if srcName := strings.TrimSpace(req.FromSession); srcName != "" {
		if !sessionNameRe.MatchString(srcName) {
			return 400, "Use 1-64 letters, digits, - or _ for a source session name", false
		}
		src, err := a.loadLocked(srcName)
		if err != nil {
			if errors.Is(err, errSessionNotFound) {
				return 404, "Source session not found", false
			}
			return 400, err.Error(), false
		}
		isSrcFac := a.facilitator(r, src) || kit.Grant(strings.TrimSpace(req.FromKey), src.Facilitator)
		if !src.allowed(callerID, isSrcFac) {
			return 404, "Source session not found", false
		}
		srcRound := src.Round
		if srcRound < 1 {
			srcRound = len(src.Rounds) + 1
		}
		var sourceCards []Card
		if req.FromRound > 0 && req.FromRound != srcRound {
			if req.FromRound < 1 || req.FromRound > len(src.Rounds) {
				return 404, "Source round not found", false
			}
			snap := src.Rounds[req.FromRound-1]
			srcRound = snap.Round
			if len(snap.RawCards) > 0 {
				for _, c := range snap.RawCards {
					sourceCards = append(sourceCards, c)
				}
			} else {
				for _, rc := range snap.Cards {
					votes := map[string]bool{}
					for v := 0; v < rc.Votes; v++ {
						votes[fmt.Sprintf("seed-%d", v)] = true
					}
					sourceCards = append(sourceCards, Card{
						ID:          rc.ID,
						Column:      rc.Column,
						Text:        rc.Text,
						Name:        rc.Name,
						Owner:       deskOwner,
						Votes:       votes,
						Preset:      true,
						PriorVotes:  rc.PriorVotes,
						OriginRound: rc.OriginRound,
					})
				}
			}
		} else {
			for _, c := range src.Cards {
				if src.Phase == "write" && !isSrcFac && c.Owner != callerID && !c.Preset {
					continue
				}
				sourceCards = append(sourceCards, c)
			}
		}
		f := RoundFilter{
			SelectedIDs:  req.SelectedIDs,
			MinVotes:     req.MinVotes,
			TopN:         req.TopN,
			FilterColumn: req.FilterColumn,
			FilterAuthor: strings.TrimSpace(req.FilterAuthor),
			FilterQuery:  strings.TrimSpace(req.FilterQuery),
			KeepVotes:    req.KeepVotes,
		}
		kept, _, _ := filterCards(sourceCards, f, src.Columns)
		for _, c := range kept {
			col := c.Column
			if col < 0 || col >= len(dst.Columns) {
				col = 0
			}
			prior := len(c.Votes)
			if prior == 0 {
				prior = c.PriorVotes
			}
			votes := map[string]bool{}
			if req.KeepVotes {
				for k, v := range c.Votes {
					votes[k] = v
				}
			}
			orig := c.OriginRound
			if orig < 1 {
				orig = srcRound
			}
			toAdd = append(toAdd, Card{
				ID:          token(),
				Column:      col,
				Text:        c.Text,
				Name:        c.Name,
				Owner:       deskOwner,
				Votes:       votes,
				Preset:      true,
				PriorVotes:  prior,
				OriginRound: orig,
			})
		}
		dst.FromSession = srcName
		dst.FromRound = srcRound
	}
	presets := req.Presets
	if len(presets) == 0 && req.Action == "preset" && strings.TrimSpace(req.Text) != "" {
		presets = []PresetCardInput{{
			Column:      req.Column,
			Text:        req.Text,
			Name:        req.Name,
			PriorVotes:  req.PriorVotes,
			OriginRound: dst.Round,
		}}
	}
	for _, p := range presets {
		text := strings.TrimSpace(p.Text)
		name := strings.TrimSpace(p.Name)
		if text == "" || len([]rune(text)) > 1000 || len([]rune(name)) > 40 {
			return 400, "Use 1–1000 characters for a preset idea and at most 40 for a nickname", false
		}
		if name == "" {
			name = "Preset"
		}
		col := p.Column
		if col < 0 || col >= len(dst.Columns) {
			return 400, "Invalid column for preset card", false
		}
		prior := p.PriorVotes
		if prior == 0 && p.Votes > 0 {
			prior = p.Votes
		}
		if prior < 0 {
			prior = 0
		}
		orig := p.OriginRound
		if orig < 1 {
			orig = dst.Round
		}
		toAdd = append(toAdd, Card{
			ID:          token(),
			Column:      col,
			Text:        text,
			Name:        name,
			Owner:       deskOwner,
			Votes:       map[string]bool{},
			Preset:      true,
			PriorVotes:  prior,
			OriginRound: orig,
		})
	}
	if req.Action == "preset" && len(toAdd) == 0 && strings.TrimSpace(req.FromSession) == "" {
		return 400, "Provide preset cards or a source session", false
	}
	if len(dst.Cards)+len(toAdd) > 1000 {
		return 409, "This session has reached its 1,000-card limit", false
	}
	dst.Cards = append(dst.Cards, toAdd...)
	return 200, "", true
}

func (a *app) view(id string, s *Session, facilitator bool) map[string]any {
	return a.viewForRound(id, s, facilitator, 0)
}

func (a *app) viewForRound(id string, s *Session, facilitator bool, roundNum int) map[string]any {
	curRound := s.Round
	if curRound < 1 {
		curRound = len(s.Rounds) + 1
	}
	if roundNum > 0 && roundNum < curRound && roundNum <= len(s.Rounds) {
		snap := s.Rounds[roundNum-1]
		cards := make([]map[string]any, 0, len(snap.Cards))
		for _, c := range snap.Cards {
			cm := map[string]any{
				"id":       c.ID,
				"column":   c.Column,
				"text":     c.Text,
				"name":     c.Name,
				"mine":     false,
				"votes":    c.Votes,
				"voted":    false,
				"advanced": c.Advanced,
			}
			if c.Preset {
				cm["preset"] = true
			}
			if c.PriorVotes > 0 {
				cm["priorVotes"] = c.PriorVotes
			}
			if c.OriginRound > 0 {
				cm["originRound"] = c.OriginRound
			}
			cards = append(cards, cm)
		}
		v := a.baseView(id, s, facilitator, cards, 0)
		v["phase"] = snap.Phase
		v["total"] = len(snap.Cards)
		v["viewingRound"] = snap.Round
		v["roundFilter"] = snap.Filter
		v["advancedCount"] = snap.AdvancedCount
		return v
	}
	cards := []map[string]any{}
	used := 0
	for _, c := range s.Cards {
		if c.Votes[id] {
			used++
		}
		if s.Phase == "write" && c.Owner != id && !c.Preset {
			continue
		}
		cm := map[string]any{
			"id":     c.ID,
			"column": c.Column,
			"text":   c.Text,
			"name":   c.Name,
			"mine":   c.Owner == id || (facilitator && c.Preset),
			"votes":  len(c.Votes),
			"voted":  c.Votes[id],
		}
		if c.Preset {
			cm["preset"] = true
		}
		if c.PriorVotes > 0 {
			cm["priorVotes"] = c.PriorVotes
		}
		if c.OriginRound > 0 {
			cm["originRound"] = c.OriginRound
		}
		cards = append(cards, cm)
	}
	v := a.baseView(id, s, facilitator, cards, used)
	if facilitator && s.Private {
		members, invites := []map[string]string{}, []map[string]string{}
		for _, m := range s.Members {
			members = append(members, map[string]string{"handle": m.Handle, "label": m.Label})
		}
		for code, label := range s.Invites {
			invites = append(invites, map[string]string{"code": code, "label": label})
		}
		sort.Slice(members, func(i, j int) bool { return members[i]["handle"] < members[j]["handle"] })
		sort.Slice(invites, func(i, j int) bool { return invites[i]["code"] < invites[j]["code"] })
		v["members"] = members
		v["invites"] = invites
	}
	return v
}
func (a *app) baseView(id string, s *Session, facilitator bool, cards []map[string]any, used int) map[string]any {
	curRound := s.Round
	if curRound < 1 {
		curRound = len(s.Rounds) + 1
	}
	v := map[string]any{"session": s.Name, "private": s.Private, "title": s.Title, "phase": s.Phase, "revision": s.Revision,
		"round": curRound, "rounds": publicRoundSnapshots(s.Rounds),
		"columns": s.Columns, "prompts": s.Prompts, "voteBudget": s.VoteBudget,
		"cards": cards, "total": len(s.Cards), "facilitator": facilitator, "teacher": facilitator,
		"votesLeft": s.VoteBudget - used}
	if s.FromSession != "" {
		v["fromSession"] = s.FromSession
		if s.FromRound > 0 {
			v["fromRound"] = s.FromRound
		}
	}
	return v
}
func main() {
	if len(os.Args) > 1 && os.Args[1] == "cli" {
		os.Exit(runCLI(os.Args[2:], os.Stdout))
	}
	data := flag.String("data", "", "persistent data directory")
	addr := flag.String("addr", defaultAddr, "listen address")
	prefix := flag.String("prefix", "", "URL prefix when served behind a sub-path (e.g. /brainstorm)")
	flag.Parse()
	if *data == "" {
		log.Fatal("-data is required")
	}
	*prefix = strings.TrimSuffix(*prefix, "/")
	if *prefix != "" && !strings.HasPrefix(*prefix, "/") {
		log.Fatal("-prefix must start with /")
	}
	a := &app{dir: *data, prefix: *prefix, sessions: map[string]*Session{}}
	a.mu.Lock()
	mainSession, err := a.ensureMainLocked()
	a.mu.Unlock()
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
	base := func(h string) string { return "http://" + net.JoinHostPort(h, port) + *prefix + "/?session=main" }
	fmt.Println("\nTeam brainstorming — keep this terminal running")
	for _, h := range hosts {
		fmt.Printf("Participant: %s\n", base(h))
	}
	facHost := "127.0.0.1"
	for _, h := range hosts {
		if h != "127.0.0.1" {
			facHost = h
			break
		}
	}
	fmt.Printf("Facilitator (keep private): %s#facilitator=%s\n", base(facHost), mainSession.Facilitator)
	fmt.Printf("Saved sessions: %s\nCtrl-C stops the server; restart resumes these sessions.\n\n", filepath.Join(*data, "sessions"))
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
