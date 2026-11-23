// Native poll and form blocks: an adapter over the form app's JSON API
// (POST /api/action add|answer|show), composed over HTTP, never imported.
//
// The page participant identity is the respondent. The page server holds a
// per-participant cookie jar towards form (item.Respondents: participant
// key -> the opaque form-participant token it presents), so each page
// identity is exactly one respondent there; a resubmit replaces it. Form's
// owner key for each block is held in the page store (item.Natives) and is
// never emitted by show, state, the page or export.
//
// Tallies are computed here from form's per-respondent results, counting
// only tokens that map to an active page participant: a kicked or merged
// identity, or anyone who reached form directly, is never counted.
// Open/closed and results visibility are page-side gates (form has no
// reopen and cannot hide results once shown).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	kit "github.com/qiangli/bashy/examples/desk/kit"
)

const formRespondentCookie = "form-participant"

// nativeBlock is the page's private record of a form item it owns a block
// over.
type nativeBlock struct {
	Kind           string   `json:"kind"` // poll | form
	Form           string   `json:"form"` // the form item id (the block's ref)
	Key            string   `json:"key"`  // form owner key; never leaves the store
	Question       string   `json:"question,omitempty"`
	Options        []string `json:"options,omitempty"`
	Votes          int      `json:"votes,omitempty"` // poll vote budget per identity
	Open           bool     `json:"open"`
	ResultsVisible bool     `json:"results_visible,omitempty"`
	Anonymous      bool     `json:"anonymous,omitempty"`
}

// formField is the part of form's definition the page renders.
type formField struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"`
	Required bool     `json:"required,omitempty"`
	Options  []string `json:"options,omitempty"`
	Help     string   `json:"help,omitempty"`
}

func (a *app) formBase() string {
	if a.formURL != "" {
		return strings.TrimRight(a.formURL, "/")
	}
	return strings.TrimRight(envOr("PAGE_FORM_URL", "http://127.0.0.1:18107"), "/")
}

// errForm carries form's refusal and its status through to the caller.
type errForm struct {
	status int
	msg    string
}

func (e *errForm) Error() string { return e.msg }

// formCall posts one action to form as its owner (key) or as one
// respondent (token), and decodes the answer into out.
func (a *app) formCall(key, token string, payload map[string]any, out any) error {
	raw, _ := json.Marshal(payload)
	req, err := http.NewRequest(http.MethodPost, a.formBase()+"/api/action", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Facilitator-Key", key)
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: formRespondentCookie, Value: token})
	}
	hc := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := hc.Do(req)
	if err != nil {
		return &errForm{http.StatusBadGateway, "the form app is not reachable; start it with form serve"}
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if resp.StatusCode != http.StatusOK {
		status := resp.StatusCode
		if status >= 500 {
			status = http.StatusBadGateway
		}
		return &errForm{status, "form: " + kit.ErrorMessage(buf)}
	}
	if out != nil && json.Unmarshal(buf, out) != nil {
		return &errForm{http.StatusBadGateway, "form: invalid response"}
	}
	return nil
}

func failForm(w http.ResponseWriter, err error) {
	var fe *errForm
	if errors.As(err, &fe) {
		kit.Fail(w, fe.status, fe.msg)
		return
	}
	kit.Fail(w, http.StatusBadGateway, err.Error())
}

// formSnapshot is form's owner view of a many-respondent item.
type formSnapshot struct {
	Status     string      `json:"status"`
	Definition []formField `json:"definition"`
	Results    struct {
		Respondents map[string]struct {
			Answers  map[string]any `json:"answers"`
			Complete bool           `json:"complete"`
		} `json:"respondents"`
	} `json:"results"`
}

// pollOption is one row of the results view: option -> votes (handles).
type pollOption struct {
	Option  string   `json:"option"`
	Votes   int      `json:"votes"`
	Handles []string `json:"handles,omitempty"`
}

// answerRow is one row of the results view: handle -> answer.
type answerRow struct {
	Handle  string         `json:"handle,omitempty"`
	Auth    string         `json:"auth,omitempty"`
	Answers map[string]any `json:"answers"`
}

// nativeResults is the organizer's results view of one block, and the
// source of every face and the record.
type nativeResults struct {
	Block          string       `json:"block"`
	Kind           string       `json:"kind"`
	Form           string       `json:"form"`
	Question       string       `json:"question,omitempty"`
	Status         string       `json:"status"` // open | closed
	ResultsVisible bool         `json:"results_visible"`
	Anonymous      bool         `json:"anonymous"`
	Votes          int          `json:"votes,omitempty"`
	Count          int          `json:"count"`
	Options        []pollOption `json:"options,omitempty"`
	Answers        []answerRow  `json:"answers"`
	Fields         []formField  `json:"fields"`

	mine map[string]any // the caller's own submitted answers
}

// nativeResultsLocked reads the block's form item as its owner and keys
// every response by page identity. caller is the requesting participant
// key ("" for none).
func (a *app) nativeResultsLocked(it *item, id string, nb *nativeBlock, caller string) (*nativeResults, error) {
	var snap formSnapshot
	if err := a.formCall(nb.Key, "", map[string]any{"action": "show", "id": nb.Form}, &snap); err != nil {
		return nil, err
	}
	status := "open"
	if !nb.Open || it.Status == kit.StatusClosed || snap.Status == kit.StatusClosed {
		status = "closed"
	}
	res := &nativeResults{Block: id, Kind: nb.Kind, Form: nb.Form, Question: nb.Question, Status: status,
		ResultsVisible: nb.ResultsVisible, Anonymous: nb.Anonymous, Votes: nb.Votes,
		Answers: []answerRow{}, Fields: snap.Definition}
	owners := map[string]string{} // form token -> active participant key
	for key, token := range it.Respondents {
		if p := it.Participants[key]; p != nil && !p.Kicked && token != "" {
			owners[token] = key
		}
	}
	counts := map[string]*pollOption{}
	for _, o := range nb.Options {
		counts[o] = &pollOption{Option: o}
	}
	for token, r := range snap.Results.Respondents {
		key, ok := owners[token]
		if !ok || !r.Complete {
			continue
		}
		if key == caller {
			res.mine = r.Answers
		}
		p := it.Participants[key]
		row := answerRow{Answers: r.Answers}
		if !nb.Anonymous {
			row.Handle, row.Auth = p.Handle, p.Auth
		}
		res.Answers = append(res.Answers, row)
		res.Count++
		if nb.Kind != "poll" {
			continue
		}
		for _, choice := range voteChoices(r.Answers["vote"]) {
			if o := counts[choice]; o != nil {
				o.Votes++
				if !nb.Anonymous {
					o.Handles = append(o.Handles, p.Handle)
				}
			}
		}
	}
	for _, o := range nb.Options {
		slices.Sort(counts[o].Handles)
		res.Options = append(res.Options, *counts[o])
	}
	slices.SortFunc(res.Answers, func(x, y answerRow) int {
		if c := strings.Compare(x.Handle, y.Handle); c != 0 {
			return c
		}
		xs, _ := json.Marshal(x.Answers)
		ys, _ := json.Marshal(y.Answers)
		return bytes.Compare(xs, ys)
	})
	return res, nil
}

// voteChoices reads a choice (string) or multi ([]string) answer.
func voteChoices(v any) []string {
	switch t := v.(type) {
	case string:
		if t != "" {
			return []string{t}
		}
	case []any:
		out := []string{}
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

// pollDefinition is the one-question form a poll block owns: a choice for
// one vote, a multi for a budget of several.
func pollDefinition(question string, options []string, votes int) ([]formField, error) {
	clean := []string{}
	for _, o := range options {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		if slices.Contains(clean, o) {
			return nil, fmt.Errorf("poll option %q is listed twice", o)
		}
		clean = append(clean, o)
	}
	if len(clean) < 2 {
		return nil, errors.New("a poll needs at least two options (--options a,b,c)")
	}
	if votes < 1 || votes > len(clean) {
		return nil, fmt.Errorf("--votes must be between 1 and the number of options (%d)", len(clean))
	}
	kind := "choice"
	if votes > 1 {
		kind = "multi"
	}
	return []formField{{ID: "vote", Label: question, Kind: kind, Required: true, Options: clean}}, nil
}

// formDefinition accepts form's definition shapes: an array of fields or
// {"title":..., "fields":[...]}. File fields stay on the form page.
func formDefinition(raw json.RawMessage) (string, json.RawMessage, error) {
	t := bytes.TrimSpace(raw)
	if len(t) == 0 {
		return "", nil, errors.New("a form block needs a definition (--def FILE, JSON)")
	}
	var fields json.RawMessage = t
	title := ""
	if t[0] == '{' {
		var wrapped struct {
			Title  string          `json:"title"`
			Fields json.RawMessage `json:"fields"`
		}
		if err := json.Unmarshal(t, &wrapped); err != nil {
			return "", nil, fmt.Errorf("invalid definition: %v", err)
		}
		title, fields = wrapped.Title, wrapped.Fields
	}
	var parsed []formField
	if err := json.Unmarshal(fields, &parsed); err != nil || len(parsed) == 0 {
		return "", nil, errors.New("invalid definition: want a non-empty array of fields")
	}
	for _, f := range parsed {
		if f.Kind == "file" {
			return "", nil, fmt.Errorf("field %q: file fields are not supported on a page", f.ID)
		}
	}
	return title, fields, nil
}

// nextBlockID returns "<kind>-N" not yet used by a block of body.
func nextBlockID(body, kind string) string {
	used := map[string]bool{}
	for _, b := range blocksOf(body) {
		used[b.ID] = true
	}
	for i := 1; ; i++ {
		if id := kind + "-" + strconv.Itoa(i); !used[id] {
			return id
		}
	}
}

// blockAdd creates the form item, keeps its owner key and appends the
// block fence to the page.
func (a *app) blockAdd(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
	if !a.isOwner(r, it) {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can add blocks")
		return
	}
	if it.Status == kit.StatusClosed {
		kit.Fail(w, http.StatusConflict, "This page is closed")
		return
	}
	id := req.Block
	if id == "" {
		id = nextBlockID(it.Body, req.Kind)
	}
	if !kit.ValidName(id) {
		kit.Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for a block id")
		return
	}
	for _, b := range blocksOf(it.Body) {
		if b.ID == id {
			kit.Fail(w, http.StatusConflict, "A block with that id already exists on this page")
			return
		}
	}
	nb := &nativeBlock{Kind: req.Kind, Open: !req.Closed, ResultsVisible: req.ResultsVisible, Anonymous: req.Anonymous}
	title := strings.TrimSpace(req.Title)
	var definition any
	switch req.Kind {
	case "poll":
		if title == "" {
			title = "Poll"
		}
		votes := req.Votes
		if votes == 0 {
			votes = 1
		}
		def, err := pollDefinition(title, req.Options, votes)
		if err != nil {
			kit.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		nb.Question, nb.Options, nb.Votes, definition = title, def[0].Options, votes, def
	case "form":
		defTitle, fields, err := formDefinition(req.Definition)
		if err != nil {
			kit.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
		if title == "" {
			title = defTitle
		}
		if title == "" {
			title = "Form"
		}
		nb.Question, definition = title, fields
	default:
		kit.Fail(w, http.StatusBadRequest, "Block kind must be poll or form")
		return
	}
	var created struct {
		ID       string `json:"id"`
		OwnerKey string `json:"ownerKey"`
	}
	if err := a.formCall("", "", map[string]any{"action": "add", "title": title, "definition": definition,
		"respondents": "many", "anonymous": req.Anonymous}, &created); err != nil {
		failForm(w, err)
		return
	}
	if !kit.ValidName(created.ID) || created.OwnerKey == "" {
		kit.Fail(w, http.StatusBadGateway, "form: invalid response")
		return
	}
	nb.Form, nb.Key = created.ID, created.OwnerKey
	next := it.clone()
	if next.Natives == nil {
		next.Natives = map[string]*nativeBlock{}
	}
	next.Natives[id] = nb
	body := strings.TrimRight(next.Body, "\n")
	if body != "" {
		body += "\n\n"
	}
	next.Body = body + "```block " + req.Kind + ":" + created.ID + " id=" + id + "\n```\n"
	next.Status = kit.StatusFilled
	if _, err := parseBody(next.Body); err != nil {
		kit.Fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.saveLocked(&next, ""); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	v := a.openView(r, &next)
	v["block"], v["kind"], v["form"] = id, req.Kind, created.ID
	kit.WriteJSON(w, v)
}

// blockSet is the organizer's open, close and results-visibility control.
func (a *app) blockSet(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
	if faceFor(r, a.canOrganize(r, it)) != organizerFace {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can change blocks")
		return
	}
	if it.Natives[req.Block] == nil {
		kit.Fail(w, http.StatusNotFound, "No poll or form block with that id on this page")
		return
	}
	next := it.clone()
	nb := next.Natives[req.Block]
	switch req.Kind {
	case "open":
		nb.Open = true
	case "close":
		nb.Open = false
	case "show-results":
		nb.ResultsVisible = true
	case "hide-results":
		nb.ResultsVisible = false
	default:
		kit.Fail(w, http.StatusBadRequest, "Use open, close, show-results or hide-results")
		return
	}
	if err := a.saveLocked(&next, req.Block); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
		return
	}
	log.Printf("page %q block %s %s", it.ID, req.Block, req.Kind)
	v := a.openView(r, &next)
	v["block"], v["open"], v["results_visible"] = req.Block, nb.Open, nb.ResultsVisible
	kit.WriteJSON(w, v)
}

// blockResults is the organizer's results view.
func (a *app) blockResults(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
	if faceFor(r, a.canOrganize(r, it)) != organizerFace {
		kit.Fail(w, http.StatusForbidden, "Only the organizer can read block results")
		return
	}
	nb := it.Natives[req.Block]
	if nb == nil {
		kit.Fail(w, http.StatusNotFound, "No poll or form block with that id on this page")
		return
	}
	res, err := a.nativeResultsLocked(it, req.Block, nb, "")
	if err != nil {
		failForm(w, err)
		return
	}
	kit.WriteJSON(w, res)
}

// answer forwards one participant's submission to form as that
// participant's respondent, then pushes the block's live frame.
func (a *app) answer(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
	p, ok := requestParticipant(r)
	if !ok {
		kit.Fail(w, http.StatusForbidden, "Join this page first, then answer")
		return
	}
	nb := it.Natives[req.Block]
	if nb == nil {
		kit.Fail(w, http.StatusNotFound, "No poll or form block with that id on this page")
		return
	}
	if !nb.Open || it.Status == kit.StatusClosed {
		kit.Fail(w, http.StatusConflict, "This "+nb.Kind+" is closed")
		return
	}
	var answers map[string]any
	if err := json.Unmarshal(req.Answers, &answers); err != nil || answers == nil {
		kit.Fail(w, http.StatusBadRequest, "Send answers as an object keyed by field id")
		return
	}
	if nb.Kind == "poll" {
		if err := checkBallot(nb, answers); err != nil {
			kit.Fail(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	if a.canOrganize(r, it) && faceFor(r, true) == participantFace {
		if a.previews == nil {
			a.previews = make(map[previewAnswerKey]map[string]any)
		}
		a.previews[previewAnswerKey{participant: p.Key, block: req.Block}] = answers
		kit.WriteJSON(w, a.openView(r, it))
		return
	}
	next := it.clone()
	if next.Respondents == nil {
		next.Respondents = map[string]string{}
	}
	token := next.Respondents[p.Key]
	if token == "" {
		token = kit.NewToken()
		next.Respondents[p.Key] = token
	}
	if err := a.formCall("", token, map[string]any{"action": "answer", "id": nb.Form,
		"answers": answers, "complete": true}, nil); err != nil {
		failForm(w, err)
		return
	}
	if err := a.saveLocked(&next, req.Block); err != nil {
		kit.Fail(w, http.StatusInternalServerError, "Could not save. Your answer was recorded but not linked; submit again.")
		return
	}
	v := a.openView(r, &next)
	v["block"] = req.Block
	kit.WriteJSON(w, v)
}

// checkBallot enforces the poll's vote budget: one option, or up to Votes
// distinct options.
func checkBallot(nb *nativeBlock, answers map[string]any) error {
	if len(answers) != 1 {
		return errors.New("a poll takes one answer: vote")
	}
	var picks []string
	switch v := answers["vote"].(type) {
	case string:
		picks = []string{v}
	case []any:
		for _, e := range v {
			s, ok := e.(string)
			if !ok {
				return errors.New("vote: want option names")
			}
			picks = append(picks, s)
		}
	default:
		return errors.New("vote: pick an option")
	}
	if nb.Votes <= 1 {
		if _, single := answers["vote"].(string); !single && len(picks) == 1 {
			answers["vote"] = picks[0]
		}
	}
	if len(picks) == 0 {
		return errors.New("vote: pick at least one option")
	}
	if len(picks) > max(nb.Votes, 1) {
		return fmt.Errorf("vote: at most %d", max(nb.Votes, 1))
	}
	seen := map[string]bool{}
	for _, s := range picks {
		if !slices.Contains(nb.Options, s) {
			return fmt.Errorf("vote: %q is not an option", s)
		}
		if seen[s] {
			return fmt.Errorf("vote: %q is picked twice", s)
		}
		seen[s] = true
	}
	if nb.Votes > 1 {
		answers["vote"] = picks
	}
	return nil
}

// nativeBlockLocked returns the page's record for b when it owns it.
func nativeBlockLocked(it *item, b *block) *nativeBlock {
	nb := it.Natives[b.ID]
	if nb == nil || nb.Kind != b.Kind || nb.Form != b.Ref {
		return nil
	}
	return nb
}

type previewAnswerKey struct {
	participant, block string
}

func (a *app) nativeFaces(r *http.Request, it *item, b *block, nb *nativeBlock, face pageFace) []blockFaceHTML {
	caller := ""
	if p, ok := requestParticipant(r); ok && face == participantFace {
		caller = p.Key
	}
	res, err := a.nativeResultsLocked(it, b.ID, nb, caller)
	if err != nil {
		return []blockFaceHTML{{face, nativeShell(face, html.EscapeString(nb.Question),
			kit.State("error", "This block is not available right now", "Check the connection to the form app."))}}
	}
	switch face {
	case organizerFace:
		faces := []blockFaceHTML{
			{organizerFace, nativeResultsHTML(b, res, organizerFace)},
			{participantFace, participantHTML(b, res, false)},
		}
		if b.Kind == "poll" {
			faces = append(faces, blockFaceHTML{displayFace, nativeDisplayHTML(res)})
		}
		return faces
	case recordFace:
		return []blockFaceHTML{{recordFace, nativeResultsHTML(b, res, recordFace)}}
	case displayFace:
		return []blockFaceHTML{{displayFace, nativeDisplayHTML(res)}}
	default:
		if a.canOrganize(r, it) {
			res.mine = a.previews[previewAnswerKey{participant: caller, block: b.ID}]
		}
		return []blockFaceHTML{{participantFace, participantHTML(b, res, caller != "")}}
	}
}

func nativeShell(face pageFace, title, inner string) string {
	return `<section class="page-native" data-face="` + string(face) + `"><h2 class="desk-h2">` + title +
		`</h2>` + inner + `</section>`
}

func nativeDisplayHTML(res *nativeResults) string {
	return nativeShell(displayFace, html.EscapeString(res.Question), pollTallyHTML(res))
}

// participantHTML is the respondent's face: the ballot or form with the
// caller's own answers, and counts (never handles) when results are
// visible.
func participantHTML(b *block, res *nativeResults, enrolled bool) string {
	var s strings.Builder
	closed := res.Status == "closed"
	disabled := ""
	if closed || !enrolled {
		disabled = " disabled"
	}
	s.WriteString(`<form class="page-answer" data-native-block="` + html.EscapeString(b.ID) + `">`)
	if res.Kind == "poll" && res.Votes > 1 {
		s.WriteString(`<p class="desk-muted">Pick up to ` + strconv.Itoa(res.Votes) + `.</p>`)
	}
	for _, f := range res.Fields {
		if res.Kind == "poll" {
			f.Label = ""
		}
		s.WriteString(answerFieldHTML(f, res.mine[f.ID], disabled))
	}
	label := "Submit"
	if res.Kind == "poll" {
		label = "Vote"
	}
	if !closed && enrolled {
		s.WriteString(`<div class="desk-row"><button type="submit" class="desk-copy">` + label + `</button></div>`)
	}
	s.WriteString(`</form>`)
	switch {
	case closed:
		s.WriteString(`<p class="desk-muted" data-native-status="closed">This ` + res.Kind + ` is closed.</p>`)
	case !enrolled:
		s.WriteString(`<p class="desk-muted">Join the page to answer.</p>`)
	case res.mine != nil:
		s.WriteString(`<p class="desk-muted" data-native-status="answered">Your answer is in; submit again to change it.</p>`)
	}
	if res.ResultsVisible && res.Kind == "poll" {
		s.WriteString(pollTallyHTML(res))
	}
	return nativeShell(participantFace, html.EscapeString(res.Question), s.String())
}

func pollTallyHTML(res *nativeResults) string {
	var s strings.Builder
	s.WriteString(`<ul class="page-tally" data-native-results>`)
	for _, o := range res.Options {
		s.WriteString(`<li>` + html.EscapeString(o.Option) + ` <strong>` + strconv.Itoa(o.Votes) + `</strong></li>`)
	}
	s.WriteString(`</ul><p class="desk-muted">` + strconv.Itoa(res.Count) + ` responses</p>`)
	return s.String()
}

// answerFieldHTML renders one field with the caller's saved answer.
func answerFieldHTML(f formField, saved any, disabled string) string {
	name := html.EscapeString(f.ID)
	label := ""
	if f.Label != "" {
		label = `<label>` + html.EscapeString(f.Label) + `</label>`
	}
	open := `<div class="desk-field" data-field="` + name + `" data-kind="` + html.EscapeString(f.Kind) + `">` + label
	savedStr, _ := saved.(string)
	option := func(typ, value string, checked bool) string {
		c := ""
		if checked {
			c = " checked"
		}
		return `<label class="desk-option"><input type="` + typ + `" name="` + name + `" value="` +
			html.EscapeString(value) + `"` + c + disabled + `> ` + html.EscapeString(value) + `</label>`
	}
	var s strings.Builder
	s.WriteString(open)
	switch f.Kind {
	case "choice":
		for _, o := range f.Options {
			s.WriteString(option("radio", o, o == savedStr))
		}
	case "yes-no":
		for _, o := range []string{"yes", "no"} {
			s.WriteString(option("radio", o, o == savedStr))
		}
	case "multi":
		chosen := voteChoices(saved)
		for _, o := range f.Options {
			s.WriteString(option("checkbox", o, slices.Contains(chosen, o)))
		}
	case "textarea":
		s.WriteString(`<textarea name="` + name + `"` + disabled + `>` + html.EscapeString(savedStr) + `</textarea>`)
	default:
		s.WriteString(`<input type="text" name="` + name + `" value="` + html.EscapeString(savedStr) + `"` + disabled + `>`)
	}
	if f.Help != "" {
		s.WriteString(`<p class="desk-muted">` + html.EscapeString(f.Help) + `</p>`)
	}
	s.WriteString(`</div>`)
	return s.String()
}

// nativeResultsHTML renders option -> votes (handles) and
// handle -> answer; handles are omitted for an anonymous block.
func nativeResultsHTML(b *block, res *nativeResults, face pageFace) string {
	var s strings.Builder
	visible := "hidden from participants"
	if res.ResultsVisible {
		visible = "visible to participants"
	}
	s.WriteString(`<p class="desk-muted" data-native-status="` + res.Status + `">` + res.Status + ` · results ` + visible)
	if res.Anonymous {
		s.WriteString(` · anonymous`)
	}
	s.WriteString(` · ` + strconv.Itoa(res.Count) + ` responses</p>`)
	if res.Kind == "poll" {
		s.WriteString(`<table class="page-results" data-native-results><thead><tr><th>Option</th><th>Votes</th>`)
		if !res.Anonymous {
			s.WriteString(`<th>Who</th>`)
		}
		s.WriteString(`</tr></thead><tbody>`)
		for _, o := range res.Options {
			s.WriteString(`<tr><td>` + html.EscapeString(o.Option) + `</td><td>` + strconv.Itoa(o.Votes) + `</td>`)
			if !res.Anonymous {
				s.WriteString(`<td>` + html.EscapeString(strings.Join(o.Handles, ", ")) + `</td>`)
			}
			s.WriteString(`</tr>`)
		}
		s.WriteString(`</tbody></table>`)
	}
	if len(res.Answers) > 0 && (res.Kind != "poll" || !res.Anonymous) {
		s.WriteString(`<table class="page-results" data-native-answers><thead><tr>`)
		if !res.Anonymous {
			s.WriteString(`<th>Participant</th>`)
		}
		for _, f := range res.Fields {
			s.WriteString(`<th>` + html.EscapeString(fieldTitle(f, res)) + `</th>`)
		}
		s.WriteString(`</tr></thead><tbody>`)
		for _, row := range res.Answers {
			s.WriteString(`<tr>`)
			if !res.Anonymous {
				s.WriteString(`<td>` + html.EscapeString(row.Handle) + `</td>`)
			}
			for _, f := range res.Fields {
				s.WriteString(`<td>` + html.EscapeString(answerText(row.Answers[f.ID])) + `</td>`)
			}
			s.WriteString(`</tr>`)
		}
		s.WriteString(`</tbody></table>`)
	}
	if face == organizerFace {
		stateAction, stateLabel := "open", "Open responses"
		if res.Status == "open" {
			stateAction, stateLabel = "close", "Close responses"
		}
		resultsAction, resultsLabel := "show-results", "Show results"
		if res.ResultsVisible {
			resultsAction, resultsLabel = "hide-results", "Hide results"
		}
		s.WriteString(`<div class="desk-row" data-native-control-block="` + html.EscapeString(b.ID) + `">` +
			`<button type="button" class="desk-copy" data-native-control="` + stateAction + `">` + stateLabel + `</button>` +
			`<button type="button" class="desk-copy" data-native-control="` + resultsAction + `">` + resultsLabel + `</button></div>`)
	}
	return nativeShell(face, html.EscapeString(res.Question), s.String())
}

func fieldTitle(f formField, res *nativeResults) string {
	if res.Kind == "poll" {
		return "Vote"
	}
	return f.Label
}

func answerText(v any) string {
	if picks := voteChoices(v); picks != nil {
		return strings.Join(picks, ", ")
	}
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		raw, _ := json.Marshal(t)
		return string(raw)
	}
}

// nativeMarkdown is the block's section of the record: tallies with
// handles and per-handle answers; no handles for an anonymous block.
func nativeMarkdown(b *block, res *nativeResults) string {
	cell := func(s string) string {
		return strings.ReplaceAll(escapeParticipantMarkdown(s), "|", "\\|")
	}
	var s strings.Builder
	s.WriteString("**" + cell(res.Question) + "** (" + res.Kind + " `" + b.ID + "`, " + res.Status + ", " +
		strconv.Itoa(res.Count) + " responses")
	if res.Anonymous {
		s.WriteString(", anonymous")
	}
	s.WriteString(")\n\n")
	if res.Kind == "poll" {
		if res.Anonymous {
			s.WriteString("| Option | Votes |\n|---|---|\n")
		} else {
			s.WriteString("| Option | Votes | Who |\n|---|---|---|\n")
		}
		for _, o := range res.Options {
			s.WriteString("| " + cell(o.Option) + " | " + strconv.Itoa(o.Votes))
			if !res.Anonymous {
				handles := make([]string, len(o.Handles))
				for i, h := range o.Handles {
					handles[i] = cell(h)
				}
				s.WriteString(" | " + strings.Join(handles, ", "))
			}
			s.WriteString(" |\n")
		}
		if res.Anonymous {
			return s.String()
		}
		s.WriteString("\n")
	}
	if len(res.Answers) == 0 {
		return s.String()
	}
	header, rule := "|", "|"
	if !res.Anonymous {
		header, rule = "| Participant |", "|---|"
	}
	for _, f := range res.Fields {
		header += " " + cell(fieldTitle(f, res)) + " |"
		rule += "---|"
	}
	s.WriteString(header + "\n" + rule + "\n")
	for _, row := range res.Answers {
		line := "|"
		if !res.Anonymous {
			line += " " + cell(row.Handle) + " |"
		}
		for _, f := range res.Fields {
			line += " " + cell(answerText(row.Answers[f.ID])) + " |"
		}
		s.WriteString(line + "\n")
	}
	return s.String()
}

// blockMarkdownLocked is a block's place in the record.
func (a *app) blockMarkdownLocked(it *item, b *block) string {
	switch b.Kind {
	case "timer":
		return timerMarkdown(it, b)
	case "book", "note":
		return contentMarkdown(b)
	case "embed":
		return embedMarkdown(b)
	case "here", "hands":
		return a.presenceMarkdown(it, b)
	}
	if b.Kind == "board" {
		return a.boardMarkdownLocked(b)
	}
	nb := nativeBlockLocked(it, b)
	if nb == nil {
		return blockMarkdown(b)
	}
	res, err := a.nativeResultsLocked(it, b.ID, nb, "")
	if err != nil {
		return blockMarkdown(b) + "\n_Results unavailable: the form app is not reachable._\n"
	}
	return nativeMarkdown(b, res)
}

// serveBlock answers GET /api/block?id=&block=: one block's HTML for the
// caller's face, re-fetched by the page when the block's live frame
// arrives.
func (a *app) serveBlock(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	defer a.mu.Unlock()
	it := a.loadOr404(w, r.URL.Query().Get("id"))
	if it == nil || !a.viewOr403(w, r, it) {
		return
	}
	r, done, ok := a.readFaceLocked(w, r, it)
	if !ok {
		return
	}
	defer done()
	it = a.items[it.ID]
	id := r.URL.Query().Get("block")
	for _, b := range blocksOf(it.Body) {
		if b.ID == id {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			io.WriteString(w, a.renderBlockFaces(r, it, &b, faceFor(r, a.canOrganize(r, it))))
			return
		}
	}
	kit.Fail(w, http.StatusNotFound, "No block with that id on this page")
}

// nativeScript submits answers and swaps a block in place when its live
// frame arrives.
func nativeScript(base, id string, face pageFace) string {
	cfg, _ := json.Marshal(map[string]string{"action": base + "/api/action?view=" + string(face), "id": id})
	return `<script>
(function(){
  const cfg = ` + string(cfg) + `;
  function alertMsg(text){ const el = document.querySelector('[data-desk-alert]'); if (el) { el.textContent = text; el.setAttribute('data-kind', text ? 'error' : ''); } }
  document.addEventListener('submit', async function(e){
    const form = e.target.closest('[data-native-block]');
    if (!form) return;
    e.preventDefault();
    const answers = {};
    form.querySelectorAll('[data-field]').forEach(function(wrap){
      const name = wrap.getAttribute('data-field'), kind = wrap.getAttribute('data-kind');
      if (kind === 'multi') {
        answers[name] = Array.from(wrap.querySelectorAll('input:checked')).map(function(i){ return i.value; });
      } else if (kind === 'choice' || kind === 'yes-no') {
        const picked = wrap.querySelector('input:checked'); answers[name] = picked ? picked.value : '';
      } else {
        const field = wrap.querySelector('input,textarea'); answers[name] = field ? field.value : '';
      }
    });
    const block = form.getAttribute('data-native-block');
    try {
      const r = await fetch(cfg.action, {method: 'POST', credentials: 'same-origin', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({action: 'answer', id: cfg.id, block: block, answers: answers})});
      const body = await r.json().catch(function(){ return {}; });
      if (!r.ok) { alertMsg((body.error && body.error.message) || 'Could not save your answer.'); return; }
      alertMsg('');
      if (window.deskToast) window.deskToast('Answer saved.', 'ok');
      document.dispatchEvent(new CustomEvent('page-block', {detail: {block}}));
    } catch (_) { alertMsg('Could not reach the page server; your answer is still on this page.'); }
  });
})();
</script>`
}

func nativeControlsScript(base, id string) string {
	cfg, _ := json.Marshal(map[string]string{"action": base + "/api/action?view=organizer", "id": id})
	return `<script>
(function () {
  const cfg = ` + string(cfg) + `;
  document.addEventListener('click', async e => {
    const button = e.target.closest('[data-native-control]');
    if (!button) return;
    const block = button.closest('[data-native-control-block]').dataset.nativeControlBlock;
    button.disabled = true;
    try {
      const response = await fetch(cfg.action, {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({action: 'block', id: cfg.id, block, kind: button.dataset.nativeControl})
      });
      if (!response.ok) throw new Error();
      document.dispatchEvent(new CustomEvent('page-block', {detail: {block}}));
    } catch (_) {
      if (window.deskToast) window.deskToast('Could not update the block.', 'error');
    } finally {
      button.disabled = false;
    }
  });
})();
</script>`
}
