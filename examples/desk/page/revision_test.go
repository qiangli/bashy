package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	kit "github.com/qiangli/bashy/examples/desk/kit"
	"github.com/qiangli/bashy/examples/desk/kit/contracttest"
)

func outpostOwnerHeaders(secret, prefix, user, role string, now time.Time) map[string]string {
	ts := strconv.FormatInt(now.Unix(), 10)
	h := http.Header{}
	h.Set("X-Forwarded-Prefix", prefix)
	h.Set("X-Outpost-Identity-Ts", ts)
	h.Set("X-Outpost-Relationship", role)
	h.Set("Remote-User", user)
	fields := []string{"v2", role, "", "", "", user, prefix, ts}
	r := &http.Request{Header: h}
	signJoinHeaders(r, secret, fields, "X-Outpost-Relationship-Sig")
	out := map[string]string{}
	for k := range r.Header {
		out[k] = r.Header.Get(k)
	}
	return out
}

func TestContentSaveRecordedWithAuthorAndBaseRev(t *testing.T) {
	base, a := startServer(t, "")
	const secret = "test-outpost-secret"
	a.joinOptions.outpostSecret = secret
	run := cli(base, a)

	now := time.Now()
	headers := outpostOwnerHeaders(secret, "/matrix/h/host/app/page", "ana@example.test", "owner", now)
	headers["X-Forwarded-For"] = "198.51.100.10, 127.0.0.1"

	// Create via outpost-stamped organizer request: records r1 with member author.
	w := do(a, http.MethodPost, "/api/action",
		`{"action":"add","id":"agenda","title":"Agenda v1","body":"# Agenda v1\n\nInitial.","audience":"lan","assistant":"genie"}`,
		"127.0.0.1:5000", headers)
	if w.Code != http.StatusOK {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	var created struct {
		OwnerKey string `json:"ownerKey"`
		Active   string `json:"active"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil || created.OwnerKey == "" || created.Active != "r1" {
		t.Fatalf("add response = %s (%v)", w.Body.String(), err)
	}

	// Recorded overwrite without base_rev from outpost owner identity.
	headers["X-Facilitator-Key"] = created.OwnerKey
	w = do(a, http.MethodPost, "/api/action",
		`{"action":"set","id":"agenda","title":"Agenda v2","body":"# Agenda v2\n\nSecond."}`,
		"127.0.0.1:5000", headers)
	if w.Code != http.StatusOK {
		t.Fatalf("set overwrite: %d %s", w.Code, w.Body)
	}

	// CLI set with matching --base-rev r2 and BASHY_AGENT assistant.
	t.Setenv("BASHY_AGENT", "claude")
	code, out := run("set", "agenda", "--base-rev", "r2", "--title", "Agenda v3", "--body", "# Agenda v3\n\nThird.", "--json")
	if code != 0 {
		t.Fatalf("cli set --base-rev r2: %d %s", code, out)
	}
	var setResp map[string]any
	if err := json.Unmarshal([]byte(out), &setResp); err != nil || setResp["active"] != "r3" || setResp["previous"] != "r2" {
		t.Fatalf("cli set response = %s (%v)", out, err)
	}

	// History via CLI --json lists r3, r2, r1 newest-first with author provenance.
	code, out = run("history", "agenda", "--json")
	if code != 0 {
		t.Fatalf("cli history: %d %s", code, out)
	}
	var hist kit.RevisionHistory
	if err := json.Unmarshal([]byte(out), &hist); err != nil {
		t.Fatalf("decode history: %v (%s)", err, out)
	}
	if hist.Active != "r3" || hist.Previous != "r2" || len(hist.Versions) != 3 {
		t.Fatalf("history = %+v", hist)
	}
	if hist.Versions[0].Revision != "r3" || hist.Versions[0].Author != "organizer" || hist.Versions[0].Assistant != "claude" {
		t.Errorf("r3 metadata = %+v", hist.Versions[0])
	}
	if hist.Versions[1].Revision != "r2" || hist.Versions[1].Author != "ana@example.test" || hist.Versions[1].Kind != "member" {
		t.Errorf("r2 metadata = %+v", hist.Versions[1])
	}
	if hist.Versions[2].Revision != "r1" || hist.Versions[2].Author != "ana@example.test" || hist.Versions[2].Kind != "member" || hist.Versions[2].Assistant != "genie" {
		t.Errorf("r1 metadata = %+v", hist.Versions[2])
	}

	// Plain-text CLI history never emits body content and prints revisions.
	code, plain := run("history", "agenda")
	if code != 0 || !strings.Contains(plain, "r3 (active)") || !strings.Contains(plain, "ana@example.test (member)") || strings.Contains(plain, "Third.") {
		t.Fatalf("plain history = %d %q", code, plain)
	}

	// Console principal also populates organizer author when no participant row exists yet.
	_, consoleApp := startServer(t, "")
	consoleApp.joinOptions.consolePrincipal = "os:alice"
	w = do(consoleApp, http.MethodPost, "/api/action",
		`{"action":"add","id":"console-p","body":"# Console\n\nBody"}`, "127.0.0.1:5000", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("console add: %d %s", w.Code, w.Body)
	}
	var cAdd struct{ OwnerKey string }
	_ = json.Unmarshal(w.Body.Bytes(), &cAdd)
	hw := do(consoleApp, http.MethodPost, "/api/action",
		`{"action":"history","id":"console-p"}`, "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": cAdd.OwnerKey})
	var cHist kit.RevisionHistory
	if hw.Code != http.StatusOK || json.Unmarshal(hw.Body.Bytes(), &cHist) != nil || len(cHist.Versions) != 1 ||
		cHist.Versions[0].Author != "alice" || cHist.Versions[0].Kind != "member" {
		t.Fatalf("console history = %d %+v (%s)", hw.Code, cHist, hw.Body.String())
	}
}

func TestStaleBaseReturns409WithBothRevisions(t *testing.T) {
	base, a := startServer(t, "")
	run := cli(base, a)

	if code, out := run("add", "--id", "doc", "--title", "V1", "--body", "Body 1"); code != 0 {
		t.Fatalf("add: %d %s", code, out)
	}
	if code, out := run("set", "doc", "--base-rev", "r1", "--body", "Body 2"); code != 0 {
		t.Fatalf("set r2: %d %s", code, out)
	}

	// HTTP 409 carries both base_rev and active and leaves content/frame intact.
	owner := map[string]string{"X-Facilitator-Key": a.items["doc"].Owner}
	w := do(a, http.MethodPost, "/api/action",
		`{"action":"set","id":"doc","base_rev":"r1","body":"Stale body"}`, "127.0.0.1:5000", owner)
	if w.Code != http.StatusConflict {
		t.Fatalf("stale HTTP status = %d, want 409: %s", w.Code, w.Body.String())
	}
	var errResp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("decode 409: %v", err)
	}
	if errResp["base_rev"] != "r1" || errResp["active"] != "r2" {
		t.Fatalf("409 response must carry both base_rev and active: %v", errResp)
	}

	// CLI --base-rev r1 fails with exit 1 and names both revisions.
	code, out := run("set", "doc", "--base-rev", "r1", "--body", "Stale body", "--json")
	if code != 1 || !strings.Contains(out, `"base_rev":"r1"`) || !strings.Contains(out, `"active":"r2"`) {
		t.Fatalf("cli stale set = %d %s", code, out)
	}

	// Content and live frame counter are untouched.
	a.mu.Lock()
	it := a.items["doc"]
	a.mu.Unlock()
	if it.Body != "Body 2" || it.Revision != 2 {
		t.Fatalf("stale save mutated item: body=%q revision=%d", it.Body, it.Revision)
	}
}

func TestUndoAndRestoreBringBodyBackLeavingLiveStateIntact(t *testing.T) {
	base, a := startServer(t, "")
	run := cli(base, a)

	if code, out := run("add", "--id", "live", "--title", "Title 1", "--body", "# Title 1\n\nBody 1\n\n```block timer:break duration=60s\n```\n\n```block here\n```\n\n```block hands\n```\n"); code != 0 {
		t.Fatalf("add: %d %s", code, out)
	}

	// Seed live & private non-content state: votes, timers, hands, participants, block owner keys.
	a.mu.Lock()
	next := a.items["live"].clone()
	next.Votes = map[string]string{"ada@192.0.2.1": "yes"}
	next.Timers = map[string]*TimerState{"timer-1": {Duration: 60, Remaining: 42, Running: false}}
	next.Participants = participantMap{
		"ada@192.0.2.1": {Handle: "Ada", Address: "192.0.2.1", Fingerprint: "fp1", Cookie: "c1", Auth: "anonymous"},
	}
	next.Hands = []HandEntry{{ParticipantKey: "ada@192.0.2.1"}}
	next.Natives = map[string]*nativeBlock{
		"poll-1": {Kind: "poll", Form: "form-1", Key: "secret-form-owner-key", Question: "Q", Options: []string{"yes", "no"}, Open: true},
	}
	next.Respondents = map[string]string{"ada@192.0.2.1": "resp-token-1"}
	next.Boards = map[string]*boardBlock{
		"board-1": {Session: "s-1", Key: "secret-board-owner-key"},
	}
	next.BoardVoters = map[string]string{"ada@192.0.2.1": "voter-token-1"}
	next.BlockFaces = map[string]storedFaces{
		"timer-1": {faceSettings: faceSettings{Project: true}, Kind: "timer", Ref: "break"},
	}
	if err := a.saveLocked(&next, ""); err != nil {
		t.Fatal(err)
	}
	a.mu.Unlock()

	if code, out := run("set", "live", "--title", "Title 2", "--body", "# Title 2\n\nBody 2\n\n```block timer:break duration=60s\n```\n"); code != 0 {
		t.Fatalf("set r2: %d %s", code, out)
	}
	if code, out := run("set", "live", "--title", "Title 3", "--body", "# Title 3\n\nBody 3\n\n```block timer:break duration=60s\n```\n"); code != 0 {
		t.Fatalf("set r3: %d %s", code, out)
	}
	revBeforeUndo := a.items["live"].Revision
	statusBeforeUndo := a.items["live"].Status

	// CLI undo brings back r2 (Title 2 / Body 2) while leaving the live frame counter and status untouched.
	code, out := run("undo", "live", "--json")
	if code != 0 {
		t.Fatalf("cli undo: %d %s", code, out)
	}
	var undone kit.RevisionState
	if err := json.Unmarshal([]byte(out), &undone); err != nil || undone.Active != "r2" || undone.Previous != "r3" {
		t.Fatalf("undo response = %s (%v)", out, err)
	}

	checkIntact := func(wantTitle, wantBody string, wantRev int, wantStatus string) {
		t.Helper()
		a.mu.Lock()
		defer a.mu.Unlock()
		cur := a.items["live"]
		if cur.Title != wantTitle || cur.Body != wantBody {
			t.Fatalf("content = (%q, %q), want (%q, %q)", cur.Title, cur.Body, wantTitle, wantBody)
		}
		if cur.Revision != wantRev {
			t.Fatalf("live frame counter = %d, want exact %d", cur.Revision, wantRev)
		}
		if cur.Status != wantStatus {
			t.Fatalf("status = %q, want exact %q", cur.Status, wantStatus)
		}
		if cur.Votes["ada@192.0.2.1"] != "yes" ||
			cur.Timers["timer-1"] == nil || cur.Timers["timer-1"].Remaining != 42 ||
			len(cur.Hands) != 1 || cur.Hands[0].ParticipantKey != "ada@192.0.2.1" ||
			cur.Participants["ada@192.0.2.1"] == nil || cur.Participants["ada@192.0.2.1"].Handle != "Ada" ||
			cur.Natives["poll-1"] == nil || cur.Natives["poll-1"].Key != "secret-form-owner-key" ||
			cur.Respondents["ada@192.0.2.1"] != "resp-token-1" ||
			cur.Boards["board-1"] == nil || cur.Boards["board-1"].Key != "secret-board-owner-key" ||
			cur.BoardVoters["ada@192.0.2.1"] != "voter-token-1" ||
			!cur.BlockFaces["timer-1"].Project {
			t.Fatalf("live/private state mutated after undo/restore: %+v", cur)
		}
	}
	checkIntact("Title 2", "# Title 2\n\nBody 2\n\n```block timer:break duration=60s\n```\n", revBeforeUndo, statusBeforeUndo)

	// CLI restore brings back r1 (Title 1 / Body 1 with blocks) and still leaves live frame counter and state intact.
	code, out = run("restore", "live", "r1", "--json")
	if code != 0 {
		t.Fatalf("cli restore r1: %d %s", code, out)
	}
	var restored kit.RevisionState
	if err := json.Unmarshal([]byte(out), &restored); err != nil || restored.Active != "r1" || restored.Previous != "r2" {
		t.Fatalf("restore response = %s (%v)", out, err)
	}
	checkIntact("Title 1", "# Title 1\n\nBody 1\n\n```block timer:break duration=60s\n```\n\n```block here\n```\n\n```block hands\n```\n", revBeforeUndo, statusBeforeUndo)

	// Reload from disk into a fresh app to verify persisted state.
	reloaded := &app{dir: a.dir, items: map[string]*item{}}
	reloaded.mu.Lock()
	loaded, err := reloaded.loadLocked("live")
	reloaded.mu.Unlock()
	if err != nil || loaded.Title != "Title 1" || loaded.Votes["ada@192.0.2.1"] != "yes" || loaded.Natives["poll-1"].Key != "secret-form-owner-key" {
		t.Fatalf("reloaded item mismatch: %+v (%v)", loaded, err)
	}
}

func TestOrganizerFaceListsRevisionsAndRestoresViaCookie(t *testing.T) {
	_, a := startServer(t, "")
	const peer = "192.0.2.50:5000"
	key := addPage(t, a, "workshop", "lan")
	owner := map[string]string{"X-Facilitator-Key": key}

	if w := do(a, http.MethodPost, "/api/action",
		`{"action":"set","id":"workshop","title":"Draft 2","body":"# Draft 2\n\nRevised."}`,
		peer, owner); w.Code != http.StatusOK {
		t.Fatalf("set r2: %d %s", w.Code, w.Body)
	}

	// Sign in to organizer browser session (cookie only, no X-Facilitator-Key on subsequent requests).
	session := do(a, http.MethodPost, "/api/action", `{"action":"organizer-session","id":"workshop"}`, peer, owner)
	if session.Code != http.StatusOK || len(session.Result().Cookies()) != 1 {
		t.Fatalf("organizer session: %d", session.Code)
	}
	cookie := session.Result().Cookies()[0]
	browser := map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}

	// Organizer face renders the revision list with a restore button and conforms to kit CSS/JS rules.
	orgPage := do(a, http.MethodGet, "/?id=workshop&view=organizer", "", peer, browser)
	if orgPage.Code != http.StatusOK {
		t.Fatalf("organizer page: %d", orgPage.Code)
	}
	htmlOut := orgPage.Body.String()
	for _, want := range []string{`data-page-revisions`, `data-revision="r2"`, `data-revision="r1"`, `data-page-restore="r1"`, `>Restore</button>`} {
		if !strings.Contains(htmlOut, want) {
			t.Errorf("organizer face missing %q", want)
		}
	}
	if violations := contracttest.StyleViolations(htmlOut, nil); len(violations) > 0 {
		t.Errorf("organizer face kit conformance violations: %v", violations)
	}

	// Clicking restore on the organizer face uses the organizer cookie.
	res := do(a, http.MethodPost, "/api/action?view=organizer",
		`{"action":"restore","id":"workshop","revision":"r1"}`, peer, browser)
	if res.Code != http.StatusOK {
		t.Fatalf("organizer cookie restore: %d %s", res.Code, res.Body)
	}
	if a.items["workshop"].Body != "# P\n\nsecret-plan" {
		t.Fatalf("body after browser restore = %q", a.items["workshop"].Body)
	}

	// Participant, preview, and display faces must never render the revision list or restore button.
	for _, tc := range []struct {
		name, path string
		headers    map[string]string
	}{
		{"preview", "/?id=workshop&view=participant", browser},
		{"display", "/?id=workshop&view=display", nil},
	} {
		w := do(a, http.MethodGet, tc.path, "", peer, tc.headers)
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "data-page-revisions") || strings.Contains(w.Body.String(), "data-page-restore") {
			t.Errorf("%s face leaked revision controls (%d)", tc.name, w.Code)
		}
	}
}

func TestParticipantActionsCreateNoRevision(t *testing.T) {
	_, a, key := classPageWithForm(t)
	if w := ownerAction(t, a, key, `{"action":"block","id":"class","kind":"poll","title":"Vote","options":["yes","no"]}`); w.Code != http.StatusOK {
		t.Fatalf("block add poll: %d %s", w.Code, w.Body)
	}
	setPayload, _ := json.Marshal(map[string]any{
		"action": "set",
		"id":     "class",
		"body":   a.items["class"].Body + "\n```block hands\n```\n",
	})
	if w := ownerAction(t, a, key, string(setPayload)); w.Code != http.StatusOK {
		t.Fatalf("add hands block: %d %s", w.Code, w.Body)
	}

	beforeHist, err := a.revisions().History("class", "")
	if err != nil {
		t.Fatal(err)
	}

	// Participant joins, raises hand, votes, and attempts forbidden organizer actions.
	p := &door{t: t, a: a, peer: "192.0.2.88:5000"}
	if w := p.join("River"); w.Code != http.StatusSeeOther {
		t.Fatalf("join: %d", w.Code)
	}
	if w := p.answer("poll-1", map[string]any{"vote": "yes"}); w.Code != http.StatusOK {
		t.Fatalf("answer: %d %s", w.Code, w.Body)
	}
	postAsParticipant := func(target, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		return p.send(r)
	}
	if w := postAsParticipant("/api/action?view=participant", `{"action":"hands","id":"class","kind":"raise"}`); w.Code != http.StatusOK {
		t.Fatalf("hands raise: %d %s", w.Code, w.Body)
	}
	for _, forbidden := range []string{
		`{"action":"set","id":"class","body":"hijack"}`,
		`{"action":"undo","id":"class"}`,
		`{"action":"restore","id":"class","revision":"r1"}`,
		`{"action":"history","id":"class"}`,
	} {
		if w := postAsParticipant("/api/action", forbidden); w.Code != http.StatusForbidden {
			t.Errorf("participant %s: want 403, got %d", forbidden, w.Code)
		}
	}

	// Organizer non-content actions (share, block open/close, close) create no content revision.
	if w := ownerAction(t, a, key, `{"action":"share","id":"class"}`); w.Code != http.StatusOK {
		t.Fatalf("share: %d", w.Code)
	}
	if w := ownerAction(t, a, key, `{"action":"block","id":"class","block":"poll-1","kind":"close"}`); w.Code != http.StatusOK {
		t.Fatalf("block close: %d", w.Code)
	}

	afterHist, err := a.revisions().History("class", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(afterHist.Versions) != len(beforeHist.Versions) || afterHist.Active != beforeHist.Active || afterHist.Previous != beforeHist.Previous {
		t.Fatalf("non-content actions created a revision: before=%+v after=%+v", beforeHist, afterHist)
	}

	// blockAdd recorded r2 and set recorded r3; undo reverts to r2 (without hands), restore r1 removes the
	// poll fence while keeping poll owner key and live counter intact, and restore r3 brings everything back.
	origBody := a.items["class"].Body
	pollKey := a.items["class"].Natives["poll-1"].Key
	liveRev := a.items["class"].Revision
	if w := ownerAction(t, a, key, `{"action":"undo","id":"class"}`); w.Code != http.StatusOK {
		t.Fatalf("undo to r2: %d %s", w.Code, w.Body)
	}
	if strings.Contains(a.items["class"].Body, "```block hands") || !strings.Contains(a.items["class"].Body, "```block poll:") || a.items["class"].Revision != liveRev {
		t.Fatalf("undo to r2 mismatch: body=%q rev=%d", a.items["class"].Body, a.items["class"].Revision)
	}
	if w := ownerAction(t, a, key, `{"action":"restore","id":"class","revision":"r1"}`); w.Code != http.StatusOK {
		t.Fatalf("restore r1: %d %s", w.Code, w.Body)
	}
	if strings.Contains(a.items["class"].Body, "```block poll:") || a.items["class"].Natives["poll-1"].Key != pollKey || a.items["class"].Revision != liveRev {
		t.Fatalf("restore r1 failed to revert body or lost poll key/rev: body=%q rev=%d", a.items["class"].Body, a.items["class"].Revision)
	}
	if w := ownerAction(t, a, key, `{"action":"restore","id":"class","revision":"r3"}`); w.Code != http.StatusOK {
		t.Fatalf("restore r3: %d %s", w.Code, w.Body)
	}
	if a.items["class"].Body != origBody || a.items["class"].Revision != liveRev {
		t.Fatalf("restore r3 body/rev mismatch: body=%q want=%q rev=%d", a.items["class"].Body, origBody, a.items["class"].Revision)
	}
}

func TestPreS1SavedPagePreservedAndPartialWriteSafe(t *testing.T) {
	_, a := startServer(t, "")
	// Write a pre-S1 saved page directly without any revisions/ directory.
	legacy := item{
		Version:  1,
		ID:       "legacy",
		Title:    "Pre-S1 Title",
		Body:     "# Pre-S1 Title\n\nLegacy content.",
		Audience: audienceHost,
		Votes:    map[string]string{"ada@127.0.0.1": "a"},
		Status:   kit.StatusFilled,
		Revision: 4,
		Owner:    kit.NewToken(),
		Members:  map[string]bool{},
		Invites:  map[string]bool{},
	}
	raw, err := kit.MarshalIndent(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := kit.AtomicWrite(itemPath(a.dir, "legacy"), raw); err != nil {
		t.Fatal(err)
	}
	owner := map[string]string{"X-Facilitator-Key": legacy.Owner}

	// Pre-S1 page renders cleanly on organizer face and returns empty history.
	if w := do(a, http.MethodGet, "/?id=legacy", "", "127.0.0.1:5000", owner); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "No revisions recorded yet.") {
		t.Fatalf("pre-S1 organizer page: %d %s", w.Code, w.Body.String())
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"undo","id":"legacy"}`, "127.0.0.1:5000", owner); w.Code != http.StatusConflict {
		t.Fatalf("pre-S1 undo: want 409, got %d", w.Code)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","base_rev":"r1","body":"x"}`, "127.0.0.1:5000", owner); w.Code != http.StatusConflict ||
		!strings.Contains(w.Body.String(), `"base_rev":"r1"`) || !strings.Contains(w.Body.String(), `"active":""`) {
		t.Fatalf("pre-S1 stale base: want 409 with both revisions, got %d %s", w.Code, w.Body.String())
	}

	// Overwrite without base_rev seeds legacy pre-S1 authored bytes as r1 and saves r2,
	// so undo brings back the prior pre-S1 title and body while preserving votes and counters.
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","body":"Migrated body"}`, "127.0.0.1:5000", owner); w.Code != http.StatusOK {
		t.Fatalf("pre-S1 set: %d %s", w.Code, w.Body)
	}
	if a.items["legacy"].Votes["ada@127.0.0.1"] != "a" || a.items["legacy"].Revision != 5 {
		t.Fatalf("pre-S1 votes/revision lost: %+v", a.items["legacy"])
	}
	if h, err := a.revisions().History("legacy", ""); err != nil || h.Active != "r2" || h.Previous != "r1" || len(h.Versions) != 2 {
		t.Fatalf("legacy first edit history = %+v (%v)", h, err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"undo","id":"legacy"}`, "127.0.0.1:5000", owner); w.Code != http.StatusOK {
		t.Fatalf("legacy first-edit undo: %d %s", w.Code, w.Body)
	}
	if a.items["legacy"].Title != "Pre-S1 Title" || a.items["legacy"].Body != "# Pre-S1 Title\n\nLegacy content." || a.items["legacy"].Revision != 5 {
		t.Fatalf("legacy undo did not restore pre-S1 content cleanly: %+v", a.items["legacy"])
	}

	// Partial write error path: simulate item write failure on set, undo and restore by replacing items/legacy.json with a non-empty dir.
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","body":"Second body"}`, "127.0.0.1:5000", owner); w.Code != http.StatusOK {
		t.Fatalf("set r3: %d", w.Code)
	}
	origItemRaw, err := os.ReadFile(itemPath(a.dir, "legacy"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(itemPath(a.dir, "legacy")); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(itemPath(a.dir, "legacy"), "hold")
	if err := os.MkdirAll(blocker, 0o700); err != nil {
		t.Fatal(err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","body":"Failed write"}`, "127.0.0.1:5000", owner); w.Code != http.StatusInternalServerError {
		t.Fatalf("partial set failure: want 500, got %d %s", w.Code, w.Body.String())
	}
	if h, err := a.revisions().History("legacy", ""); err != nil || h.Active != "r3" || h.Previous != "r1" || len(h.Versions) != 3 {
		t.Fatalf("partial set left revision pointer dirty: %+v (%v)", h, err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"undo","id":"legacy"}`, "127.0.0.1:5000", owner); w.Code != http.StatusInternalServerError {
		t.Fatalf("partial undo failure: want 500, got %d %s", w.Code, w.Body.String())
	}
	if h, err := a.revisions().History("legacy", ""); err != nil || h.Active != "r3" || h.Previous != "r1" {
		t.Fatalf("partial undo left revision pointer dirty: %+v (%v)", h, err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"restore","id":"legacy","revision":"r1"}`, "127.0.0.1:5000", owner); w.Code != http.StatusInternalServerError {
		t.Fatalf("partial restore failure: want 500, got %d %s", w.Code, w.Body.String())
	}
	if h, err := a.revisions().History("legacy", ""); err != nil || h.Active != "r3" || h.Previous != "r1" {
		t.Fatalf("partial restore left revision pointer dirty: %+v (%v)", h, err)
	}
	if a.items["legacy"].Body != "Second body" || a.items["legacy"].Revision != 6 {
		t.Fatalf("in-memory item mutated on failed write: %+v", a.items["legacy"])
	}
	if err := os.RemoveAll(itemPath(a.dir, "legacy")); err != nil {
		t.Fatal(err)
	}
	if err := kit.AtomicWrite(itemPath(a.dir, "legacy"), origItemRaw); err != nil {
		t.Fatal(err)
	}

	// Partial write error path during set when revisions/ snapshots dir is blocked:
	// item file and in-memory item must roll back to match prior history.
	snapDir := filepath.Join(a.dir, "revisions", "legacy", "_", "snapshots")
	if err := os.MkdirAll(filepath.Join(snapDir, "r4", "hold"), 0o700); err != nil {
		t.Fatal(err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","body":"Rollback body"}`, "127.0.0.1:5000", owner); w.Code != http.StatusInternalServerError {
		t.Fatalf("set with blocked snapshot dir: want 500, got %d %s", w.Code, w.Body.String())
	}
	_ = os.RemoveAll(filepath.Join(snapDir, "r4"))
	if a.items["legacy"].Body != "Second body" || a.items["legacy"].Revision != 6 {
		t.Fatalf("failed revision save did not roll back in-memory item: %+v", a.items["legacy"])
	}
	diskCheck := &app{dir: a.dir, items: map[string]*item{}}
	diskCheck.mu.Lock()
	diskItem, err := diskCheck.loadLocked("legacy")
	diskCheck.mu.Unlock()
	if err != nil || diskItem.Body != "Second body" || diskItem.Revision != 6 {
		t.Fatalf("failed revision save left dirty item on disk: %+v (%v)", diskItem, err)
	}

	// Partial write error path during add when revisions/ is blocked.
	if err := os.WriteFile(filepath.Join(a.dir, "revisions", "blocked"), []byte("not-a-dir"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"add","id":"blocked","body":"x"}`, "127.0.0.1:5000", nil); w.Code != http.StatusInternalServerError {
		t.Fatalf("add with blocked revisions: want 500, got %d", w.Code)
	}
	if _, err := os.Stat(itemPath(a.dir, "blocked")); !os.IsNotExist(err) {
		t.Fatalf("failed add left item file on disk: %v", err)
	}
	if _, ok := a.items["blocked"]; ok {
		t.Fatal("failed add left item in memory")
	}
}
