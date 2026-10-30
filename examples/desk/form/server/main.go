// Command form is the desk form tool (F1): an agent defines a form in
// JSON or YAML and the human fills it on the loopback page; the agent
// reads answers back as JSON keyed by field id.
//
// Doors: `form add|open|show|set|list|share|close|wait [--json]` (agent)
// and the loopback page (human). Storage: $BASHY_HOME/desk/form/items/<id>.json.
//
// The definition schema and answer store in schema.go are the reusable
// engine shared with replyform (F2) and questionnaire (F3).
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

type item struct {
	Version        int                `json:"version"`
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	Body           string             `json:"body"`
	Fields         []kit.Field        `json:"fields"`
	Definition     Definition         `json:"definition,omitempty"`
	Answers        Answers            `json:"answers,omitempty"`
	Respondents    string             `json:"respondents,omitempty"`
	Responses      map[string]Answers `json:"responses,omitempty"`
	Completed      map[string]bool    `json:"completed,omitempty"`
	Anonymous      bool               `json:"anonymous,omitempty"`
	CloseAt        string             `json:"close_at,omitempty"`
	ResultsVisible bool               `json:"results_visible,omitempty"`
	ReplyRows      []ReplyRow         `json:"reply_rows,omitempty"`
	Mode           string             `json:"mode,omitempty"`
	Status         string             `json:"status"`
	Revision       int                `json:"revision"`
	Owner          string             `json:"owner"`
	Members        map[string]bool    `json:"members"`
	Invites        map[string]bool    `json:"invites"`
}

type app struct {
	mu      sync.Mutex
	dir     string
	prefix  string
	baseURL string // public origin, e.g. http://127.0.0.1:18107
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
		return nil, fmt.Errorf("saved form %q is invalid; preserve it for recovery", id)
	}
	if it.Version != 1 || it.ID != id || !kit.ValidStatus(it.Status) {
		return nil, fmt.Errorf("unsupported or invalid saved form %q", id)
	}
	if err := ValidateDefinition(it.Definition); err != nil {
		return nil, fmt.Errorf("saved form %q is invalid: %v", id, err)
	}
	if it.Members == nil {
		it.Members = map[string]bool{}
	}
	if it.Invites == nil {
		it.Invites = map[string]bool{}
	}
	if it.Answers == nil {
		it.Answers = Answers{}
	}
	if it.Respondents == "" {
		it.Respondents = "one"
	}
	if it.Respondents != "one" && it.Respondents != "many" {
		return nil, fmt.Errorf("invalid respondents mode")
	}
	if it.Responses == nil {
		it.Responses = map[string]Answers{}
	}
	if it.Completed == nil {
		it.Completed = map[string]bool{}
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
	if it.closed() {
		v["status"] = kit.StatusClosed
	}
	return v
}

func (it *item) closed() bool {
	if it.Status == kit.StatusClosed {
		return true
	}
	if it.CloseAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, it.CloseAt)
	return err == nil && !time.Now().Before(t)
}

func (it *item) results() map[string]any {
	per := map[string]any{}
	agg := map[string]any{}
	for _, f := range it.Definition {
		switch f.Kind {
		case KindChoice, KindMulti, KindYesNo:
			counts := map[string]int{}
			if f.Kind == KindYesNo {
				counts["yes"], counts["no"] = 0, 0
			} else {
				for _, option := range f.Options {
					counts[option] = 0
				}
			}
			for respondent, ans := range it.Responses {
				if !it.Completed[respondent] {
					continue
				}
				if f.Kind == KindMulti {
					if vals, ok := toStringSlice(ans[f.ID]); ok {
						for _, v := range vals {
							counts[v]++
						}
					}
				} else if v, ok := ans[f.ID].(string); ok && v != "" {
					counts[v]++
				}
			}
			agg[f.ID] = counts
		default:
			values := []any{}
			for respondent, ans := range it.Responses {
				if !it.Completed[respondent] {
					continue
				}
				if v, ok := ans[f.ID]; ok && !emptyAnswer(v) {
					values = append(values, v)
				}
			}
			agg[f.ID] = values
		}
	}
	count := 0
	for id, ans := range it.Responses {
		per[id] = map[string]any{"answers": ans, "complete": it.Completed[id]}
		if it.Completed[id] {
			count++
		}
	}
	return map[string]any{"respondents": per, "aggregate": agg, "count": count}
}

// request is the shared action envelope. Definition and Answers ride as
// raw messages so definition errors can name the offending field instead
// of failing as a generic "Invalid request".
type request struct {
	Extract        json.RawMessage `json:"extract"`
	Values         json.RawMessage `json:"values"`
	Refusals       json.RawMessage `json:"refusals"`
	ReplyRows      []ReplyRow      `json:"-"`
	Action         string          `json:"action"`
	ID             string          `json:"id"`
	Title          string          `json:"title"`
	Body           string          `json:"body"`
	Fields         []kit.Field     `json:"fields"`
	Definition     json.RawMessage `json:"definition"`
	Answers        json.RawMessage `json:"answers"`
	Complete       bool            `json:"complete"`
	Create         bool            `json:"create"`
	Invite         string          `json:"invite"`
	Respondents    string          `json:"respondents"`
	Anonymous      bool            `json:"anonymous"`
	CloseAt        string          `json:"close_at"`
	ResultsVisible bool            `json:"results_visible"`
}

const modeQuestionnaire = "questionnaire"

type questionAnswer struct {
	Status string `json:"status"`
	Answer any    `json:"answer,omitempty"`
}

func decodeDefinition(raw json.RawMessage) (Definition, bool, error) {
	t := strings.TrimSpace(string(raw))
	if len(raw) == 0 || t == "" || t == "null" {
		return nil, false, nil
	}
	var def Definition
	dec := json.NewDecoder(strings.NewReader(t))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&def); err != nil {
		return nil, true, fmt.Errorf("invalid definition: %v", err)
	}
	if err := ValidateDefinition(def); err != nil {
		return nil, true, err
	}
	return def, true, nil
}

func decodeAnswers(raw json.RawMessage) (Answers, bool, error) {
	t := strings.TrimSpace(string(raw))
	if len(raw) == 0 || t == "" || t == "null" {
		return nil, false, nil
	}
	var ans Answers
	if err := json.Unmarshal(raw, &ans); err != nil {
		return nil, true, fmt.Errorf("invalid answers: %v", err)
	}
	if ans == nil {
		ans = Answers{}
	}
	return ans, true, nil
}

func (a *app) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ContractHandler("form", http.HandlerFunc(a.serveHTTP)).ServeHTTP(w, r)
}

func (a *app) serveHTTP(w http.ResponseWriter, r *http.Request) {
	kit.ApplySecurityHeaders(w)
	if !kit.GuardPOST(w, r, "form") {
		return
	}
	path, ok := kit.StripPrefix(a.prefix, r.URL.Path)
	if !ok {
		kit.Fail(w, 404, "Not found")
		return
	}
	if path == "/" && r.Method == "GET" {
		kit.ParticipantCookie(w, r, "form-participant")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(a.pageWithWriter(w, r))
		return
	}
	if path == "/healthz" && r.Method == "GET" {
		kit.WriteJSON(w, map[string]bool{"ok": true})
		return
	}
	if path == "/api/state" && r.Method == "GET" {
		a.mu.Lock()
		defer a.mu.Unlock()
		kit.ParticipantCookie(w, r, "form-participant")
		id := r.URL.Query().Get("id")
		it, err := a.loadLocked(id)
		if err != nil {
			if os.IsNotExist(err) {
				kit.Fail(w, 404, "Form not found. Create it with form add first.")
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
	var req request
	if !kit.DecodeAction(w, r, 1<<20, &req) {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	me := kit.ParticipantCookie(w, r, "form-participant")
	switch req.Action {
	case "add":
		a.create(w, req, me)
	case "reply":
		a.reply(w, req, me)
	case "ask":
		a.ask(w, r, req, me)
	case "open":
		a.open(w, req)
	case "show":
		a.get(w, r, req, me)
	case "set":
		a.mutate(w, r, req, me, false)
	case "close":
		a.mutate(w, r, req, me, true)
	case "answer":
		a.answer(w, req, me)
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

func (a *app) create(w http.ResponseWriter, req request, me string) {
	a.createItem(w, req, me, "")
}

func (a *app) createItem(w http.ResponseWriter, req request, me, mode string) {
	if kit.SecretRefused(req.Fields) {
		kit.Fail(w, 400, kit.ErrSecret)
		return
	}
	def, _, err := decodeDefinition(req.Definition)
	if err != nil {
		kit.Fail(w, 400, err.Error())
		return
	}
	if req.Respondents == "" {
		req.Respondents = "one"
	}
	if req.Respondents != "one" && req.Respondents != "many" {
		kit.Fail(w, 400, "respondents must be one or many")
		return
	}
	if req.CloseAt != "" {
		if _, err := time.Parse(time.RFC3339, req.CloseAt); err != nil {
			kit.Fail(w, 400, "close_at must be RFC3339")
			return
		}
	}
	id := req.ID
	if id == "" {
		id = "form-" + kit.NewToken()[:12]
	}
	if !kit.ValidName(id) {
		kit.Fail(w, 400, "Use 1-64 letters, digits, - or _ for an id")
		return
	}
	if _, ok := a.items[id]; ok {
		kit.Fail(w, 409, "A form with that id already exists")
		return
	}
	if _, err := os.Stat(itemPath(a.dir, id)); err == nil {
		kit.Fail(w, 409, "A form with that id already exists")
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Untitled form"
	}
	it := &item{Version: 1, ID: id, Title: title, Body: req.Body,
		Fields: req.Fields, Definition: def, Answers: Answers{},
		Mode: mode, ReplyRows: req.ReplyRows, Respondents: req.Respondents,
		Responses: map[string]Answers{}, Completed: map[string]bool{}, Anonymous: req.Anonymous, CloseAt: req.CloseAt, ResultsVisible: req.ResultsVisible,
		Status: kit.StatusPending, Owner: kit.NewToken(),
		Members: map[string]bool{me: true}, Invites: map[string]bool{}}
	if err := a.saveLocked(it); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(it)
	v["ownerKey"] = it.Owner // revealed once, at creation, like a facilitator key
	kit.WriteJSON(w, v)
}

// ask creates a questionnaire or appends a follow-up round to one. Agent
// supplied field ids are validated (including secret-name refusal) and then
// replaced by stable q1..qn ids owned by the questionnaire.
func (a *app) ask(w http.ResponseWriter, r *http.Request, req request, me string) {
	def, hasDef, err := decodeDefinition(req.Definition)
	if err != nil {
		kit.Fail(w, 400, err.Error())
		return
	}
	if !hasDef || len(def) == 0 {
		kit.Fail(w, 400, "A questionnaire needs at least one question definition")
		return
	}
	if req.ID == "" || req.Create {
		for i := range def {
			def[i].ID = fmt.Sprintf("q%d", i+1)
		}
		raw, _ := json.Marshal(def)
		req.Definition = raw
		a.createItem(w, req, me, modeQuestionnaire)
		return
	}
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Questionnaire not found. Create it with form ask first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can append questionnaire questions")
		return
	}
	if it.Mode != modeQuestionnaire {
		kit.Fail(w, 409, "Follow-up rounds can only be appended to a questionnaire")
		return
	}
	if it.Status == kit.StatusClosed {
		kit.Fail(w, 409, "This questionnaire is closed")
		return
	}
	next := *it
	next.Definition = append(Definition(nil), it.Definition...)
	for i := range def {
		def[i].ID = fmt.Sprintf("q%d", len(next.Definition)+1)
		next.Definition = append(next.Definition, def[i])
	}
	if err := ValidateDefinition(next.Definition); err != nil {
		kit.Fail(w, 400, err.Error())
		return
	}
	next.Status = kit.StatusPending
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(&next))
}

func (a *app) open(w http.ResponseWriter, req request) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	kit.WriteJSON(w, a.openView(it))
}

func (a *app) get(w http.ResponseWriter, r *http.Request, req request, me string) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) && !it.Members[me] && it.Respondents != "many" {
		kit.Fail(w, 403, "Join this form first; ask the owner for an invite.")
		return
	}
	if it.Respondents == "many" {
		v := map[string]any{"id": it.ID, "url": a.urlFor(it.ID), "status": a.openView(it)["status"],
			"title": it.Title, "body": it.Body, "definition": it.Definition, "revision": it.Revision,
			"mode": it.Mode, "respondents": it.Respondents, "anonymous": it.Anonymous,
			"close_at": it.CloseAt, "results_visible": it.ResultsVisible, "answers": it.Responses[me]}
		if a.isOwner(r, it) {
			v["results"] = it.results()
		} else if it.ResultsVisible {
			v["results"] = map[string]any{"aggregate": it.results()["aggregate"], "count": it.results()["count"]}
		}
		kit.WriteJSON(w, v)
		return
	}
	answers := any(it.Answers)
	if it.Mode == modeReply {
		answers = replyAnswers(it)
	}
	if it.Mode == modeQuestionnaire {
		byQuestion := make(map[string]questionAnswer, len(it.Definition))
		for _, q := range it.Definition {
			answer, ok := it.Answers[q.ID]
			status := kit.StatusPending
			if ok && !emptyAnswer(answer) {
				status = kit.StatusAnswered
			}
			byQuestion[q.ID] = questionAnswer{Status: status, Answer: answer}
		}
		answers = byQuestion
	}
	kit.WriteJSON(w, map[string]any{"id": it.ID, "url": a.urlFor(it.ID),
		"status": it.Status, "title": it.Title, "body": it.Body,
		"fields": it.Fields, "definition": it.Definition,
		"answers": answers, "revision": it.Revision, "mode": it.Mode})
}

func (a *app) mutate(w http.ResponseWriter, r *http.Request, req request, me string, closing bool) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can change this form")
		return
	}
	if it.closed() {
		kit.Fail(w, 409, "This form is closed")
		return
	}
	if it.Mode == modeReply && !closing {
		kit.Fail(w, 409, "Replyform proposals are immutable; mark completion on the page")
		return
	}
	next := *it
	if closing {
		next.Status = kit.StatusClosed
	} else {
		if kit.SecretRefused(req.Fields) {
			kit.Fail(w, 400, kit.ErrSecret)
			return
		}
		def, hasDef, err := decodeDefinition(req.Definition)
		if err != nil {
			kit.Fail(w, 400, err.Error())
			return
		}
		ans, hasAns, err := decodeAnswers(req.Answers)
		if err != nil {
			kit.Fail(w, 400, err.Error())
			return
		}
		if req.Body != "" {
			next.Body = req.Body
		}
		if req.Fields != nil {
			next.Fields = req.Fields
		}
		if hasDef {
			next.Definition = def
		}
		if strings.TrimSpace(req.Title) != "" {
			next.Title = req.Title
		}
		if req.CloseAt != "" {
			if _, err := time.Parse(time.RFC3339, req.CloseAt); err != nil {
				kit.Fail(w, 400, "close_at must be RFC3339")
				return
			}
			next.CloseAt = req.CloseAt
		}
		if req.ResultsVisible {
			next.ResultsVisible = true
		}
		checkDef := next.Definition
		if hasAns {
			norm, err := NormalizeAnswers(checkDef, ans, false)
			if err != nil {
				kit.Fail(w, 400, err.Error())
				return
			}
			next.Answers = norm
		} else if hasDef {
			// A replaced definition re-validates the stored answers so
			// stale keys cannot linger under new field ids.
			norm, err := NormalizeAnswers(def, next.Answers, false)
			if err != nil {
				kit.Fail(w, 400, fmt.Sprintf("stored answers do not fit the new definition: %v", err))
				return
			}
			next.Answers = norm
		}
		next.Status = kit.StatusFilled
	}
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(&next))
}

// answer is the human door: the loopback page posts partial or complete
// answers under the participant cookie. Every save persists, so partial
// answers survive reload; a complete save with all required fields filled
// moves the form to answered.
func (a *app) answer(w http.ResponseWriter, req request, me string) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if it.closed() {
		kit.Fail(w, 409, "This form is closed")
		return
	}
	ans, _, err := decodeAnswers(req.Answers)
	if err != nil {
		kit.Fail(w, 400, err.Error())
		return
	}
	if ans == nil {
		ans = Answers{}
	}
	current := it.Answers
	if it.Respondents == "many" {
		current = it.Responses[me]
	}
	merged := make(Answers, len(current)+len(ans))
	for id, answer := range current {
		merged[id] = answer
	}
	for id, answer := range ans {
		merged[id] = answer
	}
	var norm Answers
	if it.Mode == modeReply {
		norm, err = normalizeReply(it.ReplyRows, merged, req.Complete)
	} else {
		norm, err = NormalizeAnswers(it.Definition, merged, req.Complete)
	}
	if err != nil {
		kit.Fail(w, 400, err.Error())
		return
	}
	next := *it
	if it.Respondents == "many" {
		next.Responses = make(map[string]Answers, len(it.Responses)+1)
		next.Completed = make(map[string]bool, len(it.Completed)+1)
		for id, response := range it.Responses {
			next.Responses[id] = response
		}
		for id, complete := range it.Completed {
			next.Completed[id] = complete
		}
		next.Responses[me] = norm
		if req.Complete {
			next.Completed[me] = true
		}
		// A survey remains open after each participant submits.
		next.Status = kit.StatusPending
	} else {
		next.Answers = norm
	}
	if req.Complete && it.Respondents != "many" {
		next.Status = kit.StatusAnswered
	} else if it.Respondents != "many" && next.Status != kit.StatusAnswered {
		next.Status = kit.StatusPending
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

func (a *app) share(w http.ResponseWriter, r *http.Request, req request) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !a.isOwner(r, it) {
		kit.Fail(w, 403, "Only the owner can share this form")
		return
	}
	if it.Respondents == "many" {
		// The form URL is reusable; the participant cookie separates responses.
		kit.WriteJSON(w, a.openView(it))
		return
	}
	next := *it
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

func (a *app) join(w http.ResponseWriter, req request, me string) {
	it, err := a.loadLocked(req.ID)
	if err != nil {
		if os.IsNotExist(err) {
			kit.Fail(w, 404, "Form not found. Create it with form add first.")
		} else {
			kit.Fail(w, 400, err.Error())
		}
		return
	}
	if !kit.ValidToken(req.Invite) || !it.Invites[req.Invite] {
		kit.Fail(w, 403, "This invite is invalid. Ask the owner for a new one.")
		return
	}
	next := *it
	next.Members[me] = true
	delete(next.Invites, req.Invite) // single use
	if err := a.saveLocked(&next); err != nil {
		kit.Fail(w, 500, "Could not save. Your change was not accepted.")
		return
	}
	kit.WriteJSON(w, a.openView(&next))
}

// fieldHTML renders one declared field with the kit field components: the
// recommended option of a choice/multi carries a mark, required fields are
// flagged, and saved answers are pre-filled so partial saves survive reload.
func fieldHTML(f FormField, saved any, questionnaire bool) string {
	var b strings.Builder
	required := ""
	if f.Required {
		required = ` <span aria-hidden="true">*</span>`
	}
	label := html.EscapeString(f.Label) + required
	savedStr, _ := saved.(string)
	open := `<div class="desk-field" data-field="` + html.EscapeString(f.ID) +
		`" data-kind="` + html.EscapeString(f.Kind) + `"`
	if questionnaire {
		open += ` data-question-id="` + html.EscapeString(f.ID) + `"`
	}
	open += `><label>` + label + `</label>`
	help := ""
	if f.Help != "" {
		help = `<p class="desk-muted desk-content" data-desk-render="inline">` + html.EscapeString(f.Help) + `</p>`
	}
	req := ""
	if f.Required {
		req = " required"
	}
	switch f.Kind {
	case KindTextarea:
		b.WriteString(open + `<textarea name="` + html.EscapeString(f.ID) + `"` + req + `>` +
			html.EscapeString(savedStr) + `</textarea>` + help + `</div>`)
	case KindChoice:
		b.WriteString(open + `<div role="radiogroup">`)
		for _, opt := range f.Options {
			mark := ""
			if opt == f.Recommended {
				mark = `<span class="desk-recommended">recommended</span>`
			}
			checked := ""
			if opt == savedStr {
				checked = " checked"
			}
			b.WriteString(`<label class="desk-option"><input type="radio" name="` +
				html.EscapeString(f.ID) + `" value="` + html.EscapeString(opt) + `"` + checked + `>` +
				html.EscapeString(opt) + mark + `</label>`)
		}
		b.WriteString(`</div>` + help + `</div>`)
	case KindMulti:
		var chosen []string
		switch t := saved.(type) {
		case []string:
			chosen = t
		case []any:
			for _, e := range t {
				if s, ok := e.(string); ok {
					chosen = append(chosen, s)
				}
			}
		}
		b.WriteString(open)
		for _, opt := range f.Options {
			mark := ""
			if opt == f.Recommended {
				mark = `<span class="desk-recommended">recommended</span>`
			}
			checked := ""
			if contains(chosen, opt) {
				checked = " checked"
			}
			b.WriteString(`<label class="desk-option"><input type="checkbox" name="` +
				html.EscapeString(f.ID) + `" value="` + html.EscapeString(opt) + `"` + checked + `>` +
				html.EscapeString(opt) + mark + `</label>`)
		}
		b.WriteString(help + `</div>`)
	case KindYesNo:
		b.WriteString(open + `<div role="radiogroup">`)
		for _, opt := range []string{"yes", "no"} {
			checked := ""
			if opt == savedStr {
				checked = " checked"
			}
			b.WriteString(`<label class="desk-option"><input type="radio" name="` +
				html.EscapeString(f.ID) + `" value="` + opt + `"` + checked + `>` + opt + `</label>`)
		}
		b.WriteString(`</div>` + help + `</div>`)
	case KindFile:
		current := ""
		switch t := saved.(type) {
		case string:
			if t != "" {
				current = `<p class="desk-muted" data-current-file="` + html.EscapeString(t) + `">Currently: ` + html.EscapeString(t) + `</p>`
			}
		case map[string]any:
			if name, _ := t["name"].(string); name != "" {
				current = `<p class="desk-muted" data-current-file="` + html.EscapeString(name) + `">Currently: ` + html.EscapeString(name) + `</p>`
			}
		}
		b.WriteString(open + current + `<input type="file" name="` + html.EscapeString(f.ID) + `">` + help + `</div>`)
	default: // text
		b.WriteString(open + `<input type="text" name="` + html.EscapeString(f.ID) + `" value="` +
			html.EscapeString(savedStr) + `"` + req + `>` + help + `</div>`)
	}
	return b.String()
}

// page renders the human door: item header, status badges, copy buttons,
// the kit field components and a theme toggle, all from the kit snippets.
func (a *app) page(r *http.Request) []byte { return a.pageWithWriter(nil, r) }

func (a *app) pageWithWriter(w http.ResponseWriter, r *http.Request) []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	id := r.URL.Query().Get("id")
	header := kit.EmptyItemHeader("no form open")
	formBody := `<div class="desk-field">` + kit.State("empty", "No form open", "Open a form link (form open ID) to fill it here.") + `</div>`
	if id != "" {
		if it, err := a.loadLocked(id); err == nil {
			header = kit.ItemHeader(it.ID, a.openView(it)["status"].(string), a.urlFor(it.ID))
			var fb strings.Builder
			saved := it.Answers
			if it.Respondents == "many" {
				me := ""
				if c, err := r.Cookie("form-participant"); err == nil {
					me = c.Value
				}
				if me == "" && w != nil {
					me = kit.ParticipantCookie(w, r, "form-participant")
				}
				saved = it.Responses[me]
			}
			for i, f := range it.Definition {
				if it.Mode == modeQuestionnaire {
					f.Label = fmt.Sprintf("Q%d. %s", i+1, f.Label)
				}
				fb.WriteString(fieldHTML(f, saved[f.ID], it.Mode == modeQuestionnaire))
			}
			if it.Mode == modeReply {
				for _, row := range it.ReplyRows {
					row.Done, _ = it.Answers[row.StableID].(bool)
					fb.WriteString(replyHTML(row))
				}
			}
			if len(it.Definition) == 0 && it.Mode != modeReply {
				fb.WriteString(`<div class="desk-field">` + kit.State("empty", "No fields yet", "This form has no fields yet.") + `</div>`)
			}
			formBody = `<form id="desk-form" data-form-id="` + html.EscapeString(it.ID) + `">` + fb.String() +
				`<div class="desk-row">` +
				`<button type="button" class="desk-copy secondary" data-desk-save data-desk-shortcut="save" title="Save (⌘/Ctrl+S)">Save (partial)</button>` +
				`<button type="button" class="desk-copy" data-desk-submit data-desk-shortcut="submit" title="Submit (⌘/Ctrl+Enter)">Submit answers</button></div></form>`
			if it.closed() {
				formBody = `<div class="desk-field">` + kit.State("empty", "Form closed", "Responses are closed.") + `</div>`
			}
			if it.Respondents == "many" && it.ResultsVisible {
				resultJSON, _ := json.Marshal(map[string]any{"aggregate": it.results()["aggregate"], "count": it.results()["count"]})
				formBody += `<section class="desk-panel"><h2>Live results</h2><pre>` + html.EscapeString(string(resultJSON)) + `</pre></section>`
			}
		} else {
			formBody = kit.State("error", "Form not found", "Create it with form add first.")
		}
	}
	body := kit.AppBar("form", a.baseURL+"/", header, "") + `<main class="desk-main desk-narrow">` +
		`<div data-desk-alert class="desk-alert"></div>` +
		`<section class="desk-panel">` + formBody + `</section>` +
		`<p class="desk-footer">Local-first form · loopback only · partial saves persist · re-surface with <code>form open ID</code>.</p></main>` +
		`<script>` + formPageJS + `</script>`
	return kit.Page("form", "Form · bashy desk", body)
}

// formPageJS collects answers from the kit field components and posts
// partial (Save) or complete (Submit) saves. File answers send name+size;
// bytes never leave the machine through this page.
const formPageJS = `(function () {
  'use strict';
  function currentForm() { return document.getElementById('desk-form'); }
  function collect() {
    var form = currentForm();
    if (!form) return null;
    var answers = Object.create(null);
    form.querySelectorAll('[data-field]').forEach(function (wrap) {
      var id = wrap.getAttribute('data-field');
      var kind = wrap.getAttribute('data-kind');
      if (kind === 'reply') {
        answers[id] = wrap.querySelector('input[type=checkbox]').checked;
      } else if (kind === 'multi') {
        var vals = [];
        wrap.querySelectorAll('input[type=checkbox]:checked').forEach(function (el) { vals.push(el.value); });
        answers[id] = vals;
      } else if (kind === 'choice' || kind === 'yes-no') {
        var picked = wrap.querySelector('input[type=radio]:checked');
        answers[id] = picked ? picked.value : '';
      } else if (kind === 'file') {
        var input = wrap.querySelector('input[type=file]');
        if (input && input.files && input.files.length > 0) {
          answers[id] = { name: input.files[0].name, size: input.files[0].size };
        } else {
          var cur = wrap.querySelector('[data-current-file]');
          answers[id] = cur ? cur.getAttribute('data-current-file') : '';
        }
      } else {
        var field = wrap.querySelector('input,textarea,select');
        answers[id] = field ? field.value : '';
      }
    });
    return { id: form.getAttribute('data-form-id'), answers: answers };
  }
  async function send(complete) {
    var msg = document.querySelector('[data-desk-alert]');
    var payload = collect();
    if (!payload) return;
    payload.action = 'answer';
    payload.complete = !!complete;
    try {
      var r = await fetch('api/action', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(payload)
      });
      var body = await r.json();
      if (!r.ok) {
        if (msg) { msg.textContent = (body && body.error && body.error.message) || 'Could not save.'; msg.setAttribute('data-kind', 'error'); }
        return;
      }
      if (msg) msg.textContent = '';
      if (window.deskToast) window.deskToast(complete ? 'Answers submitted.' : 'Saved — partial answers persist.', 'ok');
      var badge = document.querySelector('.desk-item-header .desk-badge');
      if (badge && body.status) {
        badge.setAttribute('data-status', body.status);
        badge.textContent = body.status;
      }
    } catch (e) {
      if (msg) msg.textContent = 'Could not save. The server may be down; your answers are still on this page.';
    }
  }
  document.addEventListener('click', function (ev) {
    if (ev.target.closest('[data-desk-save]')) send(false);
    if (ev.target.closest('[data-desk-submit]')) send(true);
  });
})();`

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
	fmt.Printf("\nDesk form — keep this terminal running\nParticipant: %s/\nSaved forms: %s\nCtrl-C stops the server.\n\n",
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
