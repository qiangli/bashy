// Board blocks: board:<session> renders a brainstorm session inline and
// lets page participants write cards and vote, composed over brainstorm's
// HTTP API (GET /api/state, POST /api/action), never imported.
//
// Each page identity is exactly one brainstorm participant. The page server
// holds a per-participant cookie jar towards brainstorm (item.BoardVoters:
// participant key -> the opaque brainstorm-participant token it presents),
// so a refresh, a lost page cookie reclaimed by handle or a second tab never
// yields a second vote. Cards carry the page handle as their name. Rounds,
// phases and the vote budget stay brainstorm's: the organizer face links out
// to brainstorm for them. For a board the page created, brainstorm's
// facilitator key is held in item.Boards and appears only in that organizer
// link-out, never in participant HTML, state, show or export.
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	kit "desk-kit"
	"page/blocks"
)

const boardVoterCookie = "brainstorm-participant"

// boardBlock is the page's private record of a brainstorm session it
// created for a block.
type boardBlock struct {
	Session string `json:"session"`
	Key     string `json:"key"` // brainstorm facilitator key; organizer link-out only
}

// boardState is the part of brainstorm's session view the page renders.
type boardState struct {
	Title      string      `json:"title"`
	Phase      string      `json:"phase"`
	Round      int         `json:"round"`
	Columns    []string    `json:"columns"`
	Prompts    []string    `json:"prompts"`
	VoteBudget int         `json:"voteBudget"`
	VotesLeft  int         `json:"votesLeft"`
	Total      int         `json:"total"`
	Cards      []boardCard `json:"cards"`
}

type boardCard struct {
	ID     string `json:"id"`
	Column int    `json:"column"`
	Text   string `json:"text"`
	Name   string `json:"name"`
	Votes  int    `json:"votes"`
	Mine   bool   `json:"mine"`
	Voted  bool   `json:"voted"`
}

// brainstormBase is brainstorm's origin: PAGE_BRAINSTORM_URL, else
// BRAINSTORM_ADDR (its listen address, reached over loopback when it binds
// every interface), else the default port.
func (a *app) brainstormBase() string {
	if a.brainstormURL != "" {
		return strings.TrimRight(a.brainstormURL, "/")
	}
	if u := os.Getenv("PAGE_BRAINSTORM_URL"); u != "" {
		return strings.TrimRight(u, "/")
	}
	if host, port, err := net.SplitHostPort(os.Getenv("BRAINSTORM_ADDR")); err == nil && port != "" {
		if host == "" || host == "0.0.0.0" || host == "::" {
			host = "127.0.0.1"
		}
		return "http://" + net.JoinHostPort(host, port)
	}
	return "http://127.0.0.1:18091"
}

// brainstormCall sends one request to brainstorm as one participant (token)
// and decodes the answer into out. Refusals keep brainstorm's status.
func (a *app) brainstormCall(method, path, token string, payload, out any) error {
	var body io.Reader
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.brainstormBase()+path, body)
	if err != nil {
		return err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.AddCookie(&http.Cookie{Name: boardVoterCookie, Value: token})
	}
	hc := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{Proxy: nil}}
	resp, err := hc.Do(req)
	if err != nil {
		return &errForm{http.StatusBadGateway, "the brainstorm app is not reachable; start brainstorm first"}
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if resp.StatusCode != http.StatusOK {
		status := resp.StatusCode
		if status >= 500 {
			status = http.StatusBadGateway
		}
		return &errForm{status, "brainstorm: " + kit.ErrorMessage(buf)}
	}
	if out != nil && json.Unmarshal(buf, out) != nil {
		return &errForm{http.StatusBadGateway, "brainstorm: invalid response"}
	}
	return nil
}

// boardStateLocked reads the session as the participant token presents it
// ("" reads as a stranger: own cards and votes are none).
func (a *app) boardStateLocked(session, token string) (*boardState, error) {
	var st boardState
	if err := a.brainstormCall(http.MethodGet, "/api/state?session="+url.QueryEscape(session), token, nil, &st); err != nil {
		return nil, err
	}
	return &st, nil
}

// boardOf returns the page's board block named id.
func boardOf(it *item, id string) *block {
	for _, b := range blocksOf(it.Body) {
		if b.ID == id && b.Kind == "board" {
			return &b
		}
	}
	return nil
}

// boardName is the card name brainstorm stores: the page handle, cut to
// brainstorm's 40-character limit.
func boardName(handle string) string {
	if r := []rune(handle); len(r) > 40 {
		return string(r[:40])
	}
	return handle
}

// boardAdd appends a board block: over a named existing session, or over a
// new session the page creates and keeps the facilitator key for.
func (a *app) boardAdd(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
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
		id = nextBlockID(it.Body, "board")
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
	session := strings.TrimSpace(req.Session)
	var owned *boardBlock
	if session != "" {
		if !kit.ValidName(session) {
			kit.Fail(w, http.StatusBadRequest, "Use 1-64 letters, digits, - or _ for a session name")
			return
		}
		if _, err := a.boardStateLocked(session, ""); err != nil {
			failForm(w, err)
			return
		}
	} else {
		session = "page-" + kit.NewToken()[:12]
		title := strings.TrimSpace(req.Title)
		if title == "" {
			title = it.Title
		}
		if t := []rune(title); len(t) > 80 {
			title = string(t[:80])
		}
		var created struct {
			FacilitatorKey string `json:"facilitatorKey"`
		}
		if err := a.brainstormCall(http.MethodPost, "/api/action", "",
			map[string]any{"action": "create", "session": session, "title": title}, &created); err != nil {
			failForm(w, err)
			return
		}
		if created.FacilitatorKey == "" {
			kit.Fail(w, http.StatusBadGateway, "brainstorm: invalid response")
			return
		}
		owned = &boardBlock{Session: session, Key: created.FacilitatorKey}
	}
	next := it.clone()
	if owned != nil {
		if next.Boards == nil {
			next.Boards = map[string]*boardBlock{}
		}
		next.Boards[id] = owned
	}
	body := strings.TrimRight(next.Body, "\n")
	if body != "" {
		body += "\n\n"
	}
	next.Body = body + "```block board:" + session + " id=" + id + "\n```\n"
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
	v["block"], v["kind"], v["session"] = id, "board", session
	kit.WriteJSON(w, v)
}

// boardAction forwards one participant's card or vote to brainstorm as
// that participant, then pushes the block's live frame. The organizer's
// participant preview acts on a page-local overlay instead.
func (a *app) boardAction(w http.ResponseWriter, r *http.Request, it *item, req actionRequest) {
	p, ok := requestParticipant(r)
	if !ok {
		kit.Fail(w, http.StatusForbidden, "Join this page first, then take part")
		return
	}
	b := boardOf(it, req.Block)
	if b == nil {
		kit.Fail(w, http.StatusNotFound, "No board block with that id on this page")
		return
	}
	if it.Status == kit.StatusClosed {
		kit.Fail(w, http.StatusConflict, "This page is closed")
		return
	}
	payload := map[string]any{"action": req.Kind, "session": b.Ref}
	switch req.Kind {
	case "add":
		payload["text"], payload["column"], payload["name"] = req.Text, req.Column, boardName(p.Handle)
	case "edit":
		payload["id"], payload["text"], payload["column"], payload["name"] = req.Card, req.Text, req.Column, boardName(p.Handle)
	case "delete", "vote":
		payload["id"] = req.Card
	default:
		kit.Fail(w, http.StatusBadRequest, "Use add, edit, delete or vote")
		return
	}
	if req.Kind != "add" && req.Card == "" {
		kit.Fail(w, http.StatusBadRequest, "Name the card")
		return
	}
	if a.canOrganize(r, it) && faceFor(r, true) == participantFace {
		st, err := a.boardStateLocked(b.Ref, "")
		if err == nil {
			err = a.boardPreviewLocked(p.Key, b.ID, true).act(st, req, boardName(p.Handle))
		}
		if err != nil {
			failForm(w, err)
			return
		}
		v := a.openView(r, it)
		v["block"] = b.ID
		kit.WriteJSON(w, v)
		return
	}
	token := it.BoardVoters[p.Key]
	if token == "" {
		// Bind the identity before brainstorm sees it, so an accepted card
		// or vote is never left under an unrecorded token.
		next := it.clone()
		if next.BoardVoters == nil {
			next.BoardVoters = map[string]string{}
		}
		token = kit.NewToken()
		next.BoardVoters[p.Key] = token
		if err := a.persistLocked(&next); err != nil {
			kit.Fail(w, http.StatusInternalServerError, "Could not save. Your change was not accepted.")
			return
		}
		it = &next
	}
	if err := a.brainstormCall(http.MethodPost, "/api/action", token, payload, nil); err != nil {
		failForm(w, err)
		return
	}
	if breakMode != "events-no-publish" {
		a.events.Publish(it.ID, b.ID, it.Revision)
	}
	v := a.openView(r, it)
	v["block"] = b.ID
	kit.WriteJSON(w, v)
}

// boardPreview is the organizer's "view as participant" overlay: cards and
// votes that live only in this page server and never reach brainstorm.
type boardPreview struct {
	cards []boardCard
	votes map[string]bool
	next  int
}

func (a *app) boardPreviewLocked(participant, blockID string, create bool) *boardPreview {
	k := previewAnswerKey{participant: participant, block: blockID}
	if pv, ok := a.previews[k]["board"].(*boardPreview); ok || !create {
		return pv
	}
	if a.previews == nil {
		a.previews = make(map[previewAnswerKey]map[string]any)
	}
	pv := &boardPreview{votes: map[string]bool{}}
	a.previews[k] = map[string]any{"board": pv}
	return pv
}

// act applies brainstorm's own rules to the overlay.
func (pv *boardPreview) act(st *boardState, req actionRequest, name string) error {
	find := func() int {
		for i, c := range pv.cards {
			if c.ID == req.Card {
				return i
			}
		}
		return -1
	}
	switch req.Kind {
	case "add", "edit", "delete":
		if st.Phase != "write" {
			return &errForm{http.StatusConflict, "brainstorm: Writing has ended; your draft is still in this browser"}
		}
		i := find()
		if req.Kind != "add" && i < 0 {
			return &errForm{http.StatusForbidden, "brainstorm: You can change only your own cards"}
		}
		if req.Kind == "delete" {
			pv.cards = append(pv.cards[:i], pv.cards[i+1:]...)
			return nil
		}
		text := strings.TrimSpace(req.Text)
		if text == "" || len([]rune(text)) > 1000 || req.Column < 0 || req.Column >= len(st.Columns) {
			return &errForm{http.StatusBadRequest, "brainstorm: Use 1–1000 characters for an idea"}
		}
		if req.Kind == "edit" {
			pv.cards[i].Text, pv.cards[i].Column = text, req.Column
			return nil
		}
		pv.next++
		pv.cards = append(pv.cards, boardCard{ID: "preview-" + strconv.Itoa(pv.next), Column: req.Column,
			Text: text, Name: name, Mine: true})
	case "vote":
		if st.Phase != "vote" {
			return &errForm{http.StatusConflict, "brainstorm: Voting is not open"}
		}
		known := find() >= 0
		for _, c := range st.Cards {
			known = known || c.ID == req.Card
		}
		if !known {
			return &errForm{http.StatusNotFound, "brainstorm: Card not found"}
		}
		if pv.votes[req.Card] {
			delete(pv.votes, req.Card)
		} else if len(pv.votes) >= st.VoteBudget {
			return &errForm{http.StatusConflict, "brainstorm: You have used all " + strconv.Itoa(st.VoteBudget) + " votes"}
		} else {
			pv.votes[req.Card] = true
		}
	}
	return nil
}

// apply lays the overlay over a stranger's read of the session.
func (pv *boardPreview) apply(st *boardState) {
	st.Cards = append(st.Cards, pv.cards...)
	st.Total += len(pv.cards)
	used := 0
	for i := range st.Cards {
		if pv.votes[st.Cards[i].ID] {
			st.Cards[i].Voted = true
			st.Cards[i].Votes++
			used++
		}
	}
	st.VotesLeft = st.VoteBudget - used
}

// boardLink is the organizer's link-out to brainstorm, with the
// facilitator key when the page created the session.
func (a *app) boardLink(it *item, b *block) (string, string) {
	u := a.brainstormBase() + "/?session=" + url.QueryEscape(b.Ref)
	if bb := it.Boards[b.ID]; bb != nil && bb.Session == b.Ref && bb.Key != "" {
		return u + "#facilitator=" + url.QueryEscape(bb.Key), "Open phase and round controls in brainstorm"
	}
	return u, "Open in brainstorm (phase and round controls need your facilitator link)"
}

// boardFaces renders the faces of a board block for view.
func (a *app) boardFaces(r *http.Request, it *item, b *block, view pageFace) []blockFaceHTML {
	switch view {
	case displayFace:
		return nil
	case organizerFace:
		st, err := a.boardStateLocked(b.Ref, "")
		link, label := a.boardLink(it, b)
		return []blockFaceHTML{
			{organizerFace, boardHTML(b, st, err, false, false, link, label)},
			{participantFace, boardHTML(b, st, err, true, false, "", "")},
		}
	case recordFace:
		st, err := a.boardStateLocked(b.Ref, "")
		return []blockFaceHTML{{recordFace, boardHTML(b, st, err, false, false, "", "")}}
	}
	p, enrolled := requestParticipant(r)
	token := ""
	var pv *boardPreview
	if enrolled && a.canOrganize(r, it) {
		pv = a.boardPreviewLocked(p.Key, b.ID, false)
	} else if enrolled {
		token = it.BoardVoters[p.Key]
	}
	st, err := a.boardStateLocked(b.Ref, token)
	if err == nil && pv != nil {
		pv.apply(st)
	}
	return []blockFaceHTML{{participantFace, boardHTML(b, st, err, true, enrolled, "", "")}}
}

// boardHTML shapes one read for a face: participant faces get the write
// and vote controls of the current phase; enrolled ones may use them.
func boardHTML(b *block, st *boardState, err error, participant, enrolled bool, link, label string) string {
	d := blocks.BoardData{ID: b.ID, Title: b.ID, Enrolled: enrolled, LinkURL: link, LinkLabel: label}
	if err != nil {
		d.Err = "This board is not available right now. Check the connection to the brainstorm app."
		return blocks.Board(d)
	}
	if st.Title != "" {
		d.Title = st.Title
	}
	d.Phase, d.Round, d.Total, d.VotesLeft, d.VoteBudget = st.Phase, st.Round, st.Total, st.VotesLeft, st.VoteBudget
	d.Compose, d.Vote = participant && st.Phase == "write", participant && st.Phase == "vote"
	for i, name := range st.Columns {
		col := blocks.BoardColumn{Index: i, Name: name}
		if i < len(st.Prompts) {
			col.Prompt = st.Prompts[i]
		}
		for _, c := range st.Cards {
			if c.Column == i {
				col.Cards = append(col.Cards, blocks.BoardCard{ID: c.ID, Text: c.Text, Name: c.Name,
					Votes: c.Votes, Mine: c.Mine, Voted: c.Voted})
			}
		}
		d.Columns = append(d.Columns, col)
	}
	return blocks.Board(d)
}

// boardMarkdownLocked is the board's section of the record: the cards
// visible at export time, by column, with names and votes.
func (a *app) boardMarkdownLocked(b *block) string {
	st, err := a.boardStateLocked(b.Ref, "")
	if err != nil {
		return blockMarkdown(b) + "\n_Board unavailable: the brainstorm app is not reachable._\n"
	}
	line := func(s string) string {
		return escapeParticipantMarkdown(strings.Join(strings.Fields(s), " "))
	}
	var s strings.Builder
	s.WriteString("**" + line(st.Title) + "** (board `" + b.ID + "`, round " + strconv.Itoa(st.Round) + ", " +
		st.Phase + ", " + strconv.Itoa(st.Total) + " cards)\n\n")
	if len(st.Cards) == 0 {
		if st.Phase == "write" {
			s.WriteString("_Cards stay private until writing ends._\n")
		} else {
			s.WriteString("_No cards._\n")
		}
		return s.String()
	}
	for i, name := range st.Columns {
		var rows strings.Builder
		for _, c := range st.Cards {
			if c.Column == i {
				rows.WriteString("- " + line(c.Text) + " — " + line(c.Name) + " (" + strconv.Itoa(c.Votes) + " votes)\n")
			}
		}
		if rows.Len() > 0 {
			s.WriteString("_" + line(name) + "_\n\n" + rows.String() + "\n")
		}
	}
	return s.String()
}

const boardStyle = `<style>
.page-board-columns{display:grid;grid-template-columns:repeat(auto-fit,minmax(14rem,1fr));gap:var(--desk-space-3)}
.page-board-column ul{list-style:none;padding:0;margin:0;display:grid;gap:var(--desk-space-2)}
.page-board-card{border:1px solid var(--desk-line);border-radius:var(--desk-radius-md);padding:var(--desk-space-2);background:var(--desk-panel)}
.page-board-card[data-mine]{border-color:var(--desk-line-strong)}
.page-board-compose textarea{width:100%;min-height:4rem}
</style>`

// boardScript posts cards and votes, keeps a draft across live re-renders
// and re-reads each board every few seconds, because phase and round
// changes happen in brainstorm.
func boardScript(base, id string, face pageFace) string {
	cfg, _ := json.Marshal(map[string]string{"action": base + "/api/action?view=" + string(face), "id": id})
	return boardStyle + `<script>
(function(){
  const cfg = ` + string(cfg) + `;
  const drafts = {};
  function alertMsg(text){ const el = document.querySelector('[data-desk-alert]'); if (el) { el.textContent = text; el.setAttribute('data-kind', text ? 'error' : ''); } }
  async function send(block, body){
    try {
      const r = await fetch(cfg.action, {method: 'POST', credentials: 'same-origin', headers: {'Content-Type': 'application/json'},
        body: JSON.stringify(Object.assign({action: 'board', id: cfg.id, block: block}, body))});
      const out = await r.json().catch(function(){ return {}; });
      if (!r.ok) { alertMsg((out.error && out.error.message) || 'Could not update the board.'); return false; }
      alertMsg('');
      document.dispatchEvent(new CustomEvent('page-block', {detail: {block}}));
      return true;
    } catch (_) { alertMsg('Could not reach the page server; your draft is still on this page.'); return false; }
  }
  document.addEventListener('input', function(e){
    const form = e.target.closest('[data-board-compose]');
    if (form) drafts[form.dataset.boardCompose] = {text: form.elements.text.value, column: form.elements.column.value, edit: form.dataset.boardEdit || ''};
  });
  document.addEventListener('submit', async function(e){
    const form = e.target.closest('[data-board-compose]');
    if (!form) return;
    e.preventDefault();
    const block = form.dataset.boardCompose;
    const edit = form.dataset.boardEdit;
    const body = {kind: edit ? 'edit' : 'add', card: edit || '', text: form.elements.text.value, column: Number(form.elements.column.value)};
    if (await send(block, body)) delete drafts[block];
  });
  document.addEventListener('click', function(e){
    const button = e.target.closest('[data-board-action]');
    if (!button) return;
    const board = button.closest('[data-board]'), card = button.closest('[data-board-card]');
    const action = button.dataset.boardAction;
    if (action === 'edit') {
      const form = board.querySelector('[data-board-compose]');
      if (!form) return;
      form.dataset.boardEdit = card.dataset.boardCard;
      form.elements.text.value = card.querySelector('p').textContent;
      form.elements.column.value = card.closest('[data-board-column]').dataset.boardColumn;
      drafts[board.dataset.board] = {text: form.elements.text.value, column: form.elements.column.value, edit: form.dataset.boardEdit};
      form.elements.text.focus();
      return;
    }
    send(board.dataset.board, {kind: action, card: card.dataset.boardCard});
  });
  new MutationObserver(function(){
    document.querySelectorAll('[data-board-compose]').forEach(function(form){
      const d = drafts[form.dataset.boardCompose];
      if (d && !form.elements.text.value) { form.elements.text.value = d.text; form.elements.column.value = d.column; if (d.edit) form.dataset.boardEdit = d.edit; }
    });
  }).observe(document.body, {childList: true, subtree: true});
  setInterval(function(){
    if (document.hidden) return;
    document.querySelectorAll('[data-board]').forEach(function(board){
      if (board.contains(document.activeElement)) return;
      document.dispatchEvent(new CustomEvent('page-block', {detail: {block: board.dataset.board}}));
    });
  }, 4000);
})();
</script>`
}

// hasBoard reports whether the page body has a board block.
func hasBoard(it *item) bool {
	for _, b := range blocksOf(it.Body) {
		if b.Kind == "board" {
			return true
		}
	}
	return false
}
