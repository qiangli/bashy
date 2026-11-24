package main

import (
	"bufio"
	"context"
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

func openPageEventStream(t *testing.T, base, id string) (<-chan kit.Frame, func()) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/events?id="+id, nil)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		cancel()
		t.Fatalf("open event stream: status %d", resp.StatusCode)
	}
	frames := make(chan kit.Frame, 32)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer close(frames)
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if raw, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				var f kit.Frame
				if json.Unmarshal([]byte(raw), &f) == nil {
					frames <- f
				}
			}
		}
	}()
	// Consume the initial open frame emitted on subscription.
	select {
	case _, ok := <-frames:
		if !ok {
			resp.Body.Close()
			cancel()
			t.Fatal("event stream closed before initial frame")
		}
	case <-time.After(3 * time.Second):
		resp.Body.Close()
		cancel()
		t.Fatal("timed out waiting for initial SSE frame")
	}
	return frames, func() {
		cancel()
		resp.Body.Close()
		<-done
	}
}

func expectOnePageFrame(t *testing.T, frames <-chan kit.Frame, id string, wantRev int) {
	t.Helper()
	var f kit.Frame
	select {
	case got, ok := <-frames:
		if !ok {
			t.Fatalf("event stream closed waiting for rev %d", wantRev)
		}
		f = got
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for SSE frame rev=%d", wantRev)
	}
	if f.Item != id || f.Block != "" || f.Revision != wantRev || f.Resync {
		t.Fatalf("unexpected SSE frame %+v, want {Item:%q Block:\"\" Revision:%d Resync:false}", f, id, wantRev)
	}
	select {
	case extra, ok := <-frames:
		if ok {
			t.Fatalf("expected exactly one SSE frame for rev %d, got extra %+v", wantRev, extra)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

func expectNoPageFrame(t *testing.T, frames <-chan kit.Frame) {
	t.Helper()
	select {
	case f, ok := <-frames:
		if ok {
			t.Fatalf("expected no SSE frame, got %+v", f)
		}
	case <-time.After(50 * time.Millisecond):
	}
}

// nonContentBytes serializes every field of it except Title, Body, and Revision
// so callers can prove undo/restore leave all non-content state byte-identical.
func nonContentBytes(t *testing.T, it *item) []byte {
	t.Helper()
	c := it.clone()
	c.Title = ""
	c.Body = ""
	c.Revision = 0
	raw, err := kit.MarshalIndent(c)
	if err != nil {
		t.Fatal(err)
	}
	return raw
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
	if a.items["agenda"].Revision != 1 {
		t.Fatalf("add revision = %d, want 1", a.items["agenda"].Revision)
	}
	frames, closeStream := openPageEventStream(t, base, "agenda")
	defer closeStream()

	// Recorded overwrite without base_rev from outpost owner identity.
	headers["X-Facilitator-Key"] = created.OwnerKey
	w = do(a, http.MethodPost, "/api/action",
		`{"action":"set","id":"agenda","title":"Agenda v2","body":"# Agenda v2\n\nSecond."}`,
		"127.0.0.1:5000", headers)
	if w.Code != http.StatusOK {
		t.Fatalf("set overwrite: %d %s", w.Code, w.Body)
	}
	if a.items["agenda"].Revision != 2 {
		t.Fatalf("set r2 revision = %d, want 2", a.items["agenda"].Revision)
	}
	expectOnePageFrame(t, frames, "agenda", 2)

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
	if a.items["agenda"].Revision != 3 {
		t.Fatalf("set r3 revision = %d, want 3", a.items["agenda"].Revision)
	}
	expectOnePageFrame(t, frames, "agenda", 3)

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

	frames, closeStream := openPageEventStream(t, base, "doc")
	defer closeStream()

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
	expectNoPageFrame(t, frames)

	// CLI --base-rev r1 fails with exit 1 and names both revisions in JSON and plain text.
	code, out := run("set", "doc", "--base-rev", "r1", "--body", "Stale body", "--json")
	if code != 1 || !strings.Contains(out, `"base_rev":"r1"`) || !strings.Contains(out, `"active":"r2"`) {
		t.Fatalf("cli stale set --json = %d %s", code, out)
	}
	code, plain := run("set", "doc", "--base-rev", "r1", "--body", "Stale body")
	if code != 1 || !strings.Contains(plain, `stale base revision "r1"`) || !strings.Contains(plain, `active revision is "r2"`) {
		t.Fatalf("cli stale set plain = %d %q", code, plain)
	}
	expectNoPageFrame(t, frames)

	// Content and live frame counter are untouched.
	a.mu.Lock()
	it := a.items["doc"]
	a.mu.Unlock()
	if it.Body != "Body 2" || it.Revision != 2 {
		t.Fatalf("stale save mutated item: body=%q revision=%d", it.Body, it.Revision)
	}
}

func TestUndoAndRestoreBringBodyBackIncrementingLiveFrameWithByteIdenticalNonContentState(t *testing.T) {
	base, a := startServer(t, "")
	run := cli(base, a)

	if code, out := run("add", "--id", "live", "--title", "Title 1", "--body", "# Title 1\n\nBody 1\n\n```block timer:break duration=60s\n```\n\n```block here\n```\n\n```block hands\n```\n"); code != 0 {
		t.Fatalf("add: %d %s", code, out)
	}
	if a.items["live"].Revision != 1 {
		t.Fatalf("add revision = %d, want 1", a.items["live"].Revision)
	}

	frames, closeStream := openPageEventStream(t, base, "live")
	defer closeStream()

	// Seed live & private non-content state: votes, timers, hands, participants, block owner keys, faces.
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
	expectOnePageFrame(t, frames, "live", 2)

	if code, out := run("set", "live", "--title", "Title 2", "--body", "# Title 2\n\nBody 2\n\n```block timer:break duration=60s\n```\n"); code != 0 {
		t.Fatalf("set r2: %d %s", code, out)
	}
	expectOnePageFrame(t, frames, "live", 3)
	if code, out := run("set", "live", "--title", "Title 3", "--body", "# Title 3\n\nBody 3\n\n```block timer:break duration=60s\n```\n"); code != 0 {
		t.Fatalf("set r3: %d %s", code, out)
	}
	expectOnePageFrame(t, frames, "live", 4)

	a.mu.Lock()
	revBeforeUndo := a.items["live"].Revision
	wantNonContent := nonContentBytes(t, a.items["live"])
	a.mu.Unlock()

	checkIntact := func(wantTitle, wantBody string, wantRev int) {
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
		gotNonContent := nonContentBytes(t, cur)
		if string(gotNonContent) != string(wantNonContent) {
			t.Fatalf("non-content state drifted:\ngot:\n%s\nwant:\n%s", gotNonContent, wantNonContent)
		}
	}

	// CLI undo brings back r2 (Title 2 / Body 2), increments live Revision by +1, and publishes 1 frame.
	code, out := run("undo", "live", "--json")
	if code != 0 {
		t.Fatalf("cli undo: %d %s", code, out)
	}
	var undone kit.RevisionState
	if err := json.Unmarshal([]byte(out), &undone); err != nil || undone.Active != "r2" || undone.Previous != "r3" {
		t.Fatalf("undo response = %s (%v)", out, err)
	}
	expectOnePageFrame(t, frames, "live", revBeforeUndo+1)
	checkIntact("Title 2", "# Title 2\n\nBody 2\n\n```block timer:break duration=60s\n```\n", revBeforeUndo+1)

	// CLI restore brings back r1 (Title 1 / Body 1 with blocks), increments live Revision by +1, and publishes 1 frame.
	code, out = run("restore", "live", "r1", "--json")
	if code != 0 {
		t.Fatalf("cli restore r1: %d %s", code, out)
	}
	var restored kit.RevisionState
	if err := json.Unmarshal([]byte(out), &restored); err != nil || restored.Active != "r1" || restored.Previous != "r2" {
		t.Fatalf("restore response = %s (%v)", out, err)
	}
	expectOnePageFrame(t, frames, "live", revBeforeUndo+2)
	checkIntact("Title 1", "# Title 1\n\nBody 1\n\n```block timer:break duration=60s\n```\n\n```block here\n```\n\n```block hands\n```\n", revBeforeUndo+2)

	// Reload from disk into a fresh app to verify persisted state matches byte-identically.
	reloaded := &app{dir: a.dir, items: map[string]*item{}}
	reloaded.mu.Lock()
	loaded, err := reloaded.loadLocked("live")
	reloaded.mu.Unlock()
	if err != nil || loaded.Title != "Title 1" || loaded.Revision != revBeforeUndo+2 {
		t.Fatalf("reloaded item mismatch: %+v (%v)", loaded, err)
	}
	if got := nonContentBytes(t, loaded); string(got) != string(wantNonContent) {
		t.Fatalf("reloaded non-content state drifted:\ngot:\n%s\nwant:\n%s", got, wantNonContent)
	}
}

func TestOrganizerFaceListsRevisionsAndRestoresViaCookie(t *testing.T) {
	base, a := startServer(t, "")
	const peer = "192.0.2.50:5000"
	key := addPage(t, a, "workshop", "lan")
	owner := map[string]string{"X-Facilitator-Key": key}

	frames, closeStream := openPageEventStream(t, base, "workshop")
	defer closeStream()

	if w := do(a, http.MethodPost, "/api/action",
		`{"action":"set","id":"workshop","title":"Draft 2","body":"# Draft 2\n\nRevised."}`,
		peer, owner); w.Code != http.StatusOK {
		t.Fatalf("set r2: %d %s", w.Code, w.Body)
	}
	expectOnePageFrame(t, frames, "workshop", 2)

	// Sign in to organizer browser session (cookie only, no X-Facilitator-Key on subsequent requests).
	session := do(a, http.MethodPost, "/api/action", `{"action":"organizer-session","id":"workshop"}`, peer, owner)
	if session.Code != http.StatusOK || len(session.Result().Cookies()) != 1 {
		t.Fatalf("organizer session: %d", session.Code)
	}
	cookie := session.Result().Cookies()[0]
	browser := map[string]string{"Cookie": cookie.Name + "=" + cookie.Value}

	// Organizer face renders the revision list, error surfacing script, and conforms to kit CSS/JS rules.
	orgPage := do(a, http.MethodGet, "/?id=workshop&view=organizer", "", peer, browser)
	if orgPage.Code != http.StatusOK {
		t.Fatalf("organizer page: %d", orgPage.Code)
	}
	htmlOut := orgPage.Body.String()
	for _, want := range []string{
		`data-page-revisions`, `data-revision="r2"`, `data-revision="r1"`,
		`data-page-restore="r1"`, `>Restore</button>`,
		`out.error && out.error.message`, `data-desk-alert`,
	} {
		if !strings.Contains(htmlOut, want) {
			t.Errorf("organizer face missing %q", want)
		}
	}
	if violations := contracttest.StyleViolations(htmlOut, nil); len(violations) > 0 {
		t.Errorf("organizer face kit conformance violations: %v", violations)
	}

	// Clicking restore on the organizer face uses the organizer cookie, bumps Revision +1, and publishes 1 frame.
	res := do(a, http.MethodPost, "/api/action?view=organizer",
		`{"action":"restore","id":"workshop","revision":"r1"}`, peer, browser)
	if res.Code != http.StatusOK {
		t.Fatalf("organizer cookie restore: %d %s", res.Code, res.Body)
	}
	if a.items["workshop"].Body != "# P\n\nsecret-plan" || a.items["workshop"].Revision != 3 {
		t.Fatalf("after browser restore = body:%q rev:%d", a.items["workshop"].Body, a.items["workshop"].Revision)
	}
	expectOnePageFrame(t, frames, "workshop", 3)

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

func TestBlockAddAndBoardAddContentHistory(t *testing.T) {
	base, a, key := classPageWithForm(t)
	a.brainstormURL = startBrainstorm(t)
	run := cli(base, a)
	if a.items["class"].Revision != 1 {
		t.Fatalf("initial revision = %d, want 1", a.items["class"].Revision)
	}

	frames, closeStream := openPageEventStream(t, base, "class")
	defer closeStream()

	// 1. blockAdd (poll) records r2, bumps live Revision by +1, publishes 1 frame.
	t.Setenv("BASHY_AGENT", "genie")
	code, out := run("block", "add", "poll", "class", "--options", "yes,no", "--title", "Vote")
	if code != 0 {
		t.Fatalf("block add poll: %d %s", code, out)
	}
	if a.items["class"].Revision != 2 {
		t.Fatalf("after poll blockAdd revision = %d, want 2", a.items["class"].Revision)
	}
	expectOnePageFrame(t, frames, "class", 2)

	// 2. blockAdd (form) records r3, bumps live Revision by +1, publishes 1 frame.
	defPath := filepath.Join(t.TempDir(), "feedback.json")
	if err := os.WriteFile(defPath, []byte(`[{"id":"note","label":"Note","kind":"text"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	code, out = run("block", "add", "form", "class", "--def", defPath, "--title", "Feedback")
	if code != 0 {
		t.Fatalf("block add form: %d %s", code, out)
	}
	if a.items["class"].Revision != 3 {
		t.Fatalf("after form blockAdd revision = %d, want 3", a.items["class"].Revision)
	}
	expectOnePageFrame(t, frames, "class", 3)

	// 3. boardAdd records r4, bumps live Revision by +1, publishes 1 frame.
	t.Setenv("BASHY_AGENT", "claude")
	code, out = run("block", "add", "board", "class", "--title", "Ideas")
	if code != 0 {
		t.Fatalf("block add board: %d %s", code, out)
	}
	if a.items["class"].Revision != 4 {
		t.Fatalf("after boardAdd revision = %d, want 4", a.items["class"].Revision)
	}
	expectOnePageFrame(t, frames, "class", 4)

	// Verify content history captures all 4 revisions and assistant metadata.
	histResp := ownerAction(t, a, key, `{"action":"history","id":"class"}`)
	if histResp.Code != http.StatusOK {
		t.Fatalf("history: %d %s", histResp.Code, histResp.Body)
	}
	var hist kit.RevisionHistory
	if err := json.Unmarshal(histResp.Body.Bytes(), &hist); err != nil {
		t.Fatal(err)
	}
	if hist.Active != "r4" || hist.Previous != "r3" || len(hist.Versions) != 4 {
		t.Fatalf("history = %+v", hist)
	}
	if hist.Versions[0].Assistant != "claude" || hist.Versions[1].Assistant != "genie" || hist.Versions[2].Assistant != "genie" {
		t.Fatalf("assistant metadata mismatch: %+v", hist.Versions)
	}

	pollKey := a.items["class"].Natives["poll-1"].Key
	formKey := a.items["class"].Natives["form-1"].Key
	boardKey := a.items["class"].Boards["board-1"].Key
	bodyR4 := a.items["class"].Body
	wantNonContent := nonContentBytes(t, a.items["class"])

	// 4. Undo steps back from r4 (board) -> r3 (poll+form), increments Revision +1, publishes 1 frame, keeps all keys.
	if w := ownerAction(t, a, key, `{"action":"undo","id":"class"}`); w.Code != http.StatusOK {
		t.Fatalf("undo r4->r3: %d %s", w.Code, w.Body)
	}
	expectOnePageFrame(t, frames, "class", 5)
	if a.items["class"].Revision != 5 || strings.Contains(a.items["class"].Body, "```block board:") || !strings.Contains(a.items["class"].Body, "```block form:") {
		t.Fatalf("undo to r3 mismatch: rev=%d body=%q", a.items["class"].Revision, a.items["class"].Body)
	}
	if string(nonContentBytes(t, a.items["class"])) != string(wantNonContent) {
		t.Fatal("undo mutated non-content state (native/board keys)")
	}

	// 5. Restore r1 removes all added blocks from body, increments Revision +1, publishes 1 frame, keeps all keys.
	if w := ownerAction(t, a, key, `{"action":"restore","id":"class","revision":"r1"}`); w.Code != http.StatusOK {
		t.Fatalf("restore r1: %d %s", w.Code, w.Body)
	}
	expectOnePageFrame(t, frames, "class", 6)
	if a.items["class"].Revision != 6 || strings.Contains(a.items["class"].Body, "```block ") {
		t.Fatalf("restore r1 mismatch: rev=%d body=%q", a.items["class"].Revision, a.items["class"].Body)
	}
	if a.items["class"].Natives["poll-1"].Key != pollKey || a.items["class"].Natives["form-1"].Key != formKey || a.items["class"].Boards["board-1"].Key != boardKey {
		t.Fatal("restore r1 lost native or board owner keys")
	}
	if string(nonContentBytes(t, a.items["class"])) != string(wantNonContent) {
		t.Fatal("restore r1 mutated non-content state")
	}

	// 6. Restore r4 brings back poll, form, and board fences, increments Revision +1, publishes 1 frame.
	if w := ownerAction(t, a, key, `{"action":"restore","id":"class","revision":"r4"}`); w.Code != http.StatusOK {
		t.Fatalf("restore r4: %d %s", w.Code, w.Body)
	}
	expectOnePageFrame(t, frames, "class", 7)
	if a.items["class"].Revision != 7 || a.items["class"].Body != bodyR4 {
		t.Fatalf("restore r4 mismatch: rev=%d body=%q want=%q", a.items["class"].Revision, a.items["class"].Body, bodyR4)
	}
	if string(nonContentBytes(t, a.items["class"])) != string(wantNonContent) {
		t.Fatal("restore r4 mutated non-content state")
	}

	// 7. Partial write failure during blockAdd and boardAdd rolls back item body, Natives/Boards, and Revision.
	snapDir := filepath.Join(a.dir, "revisions", "class", "_", "snapshots")
	if err := os.MkdirAll(filepath.Join(snapDir, "r5", "hold"), 0o700); err != nil {
		t.Fatal(err)
	}
	if w := ownerAction(t, a, key, `{"action":"block","id":"class","kind":"poll","options":["x","y"]}`); w.Code != http.StatusInternalServerError {
		t.Fatalf("failed blockAdd: want 500, got %d %s", w.Code, w.Body.String())
	}
	if w := ownerAction(t, a, key, `{"action":"block","id":"class","kind":"board"}`); w.Code != http.StatusInternalServerError {
		t.Fatalf("failed boardAdd: want 500, got %d %s", w.Code, w.Body.String())
	}
	_ = os.RemoveAll(filepath.Join(snapDir, "r5"))
	expectNoPageFrame(t, frames)
	if a.items["class"].Revision != 7 || a.items["class"].Body != bodyR4 || string(nonContentBytes(t, a.items["class"])) != string(wantNonContent) {
		t.Fatalf("failed blockAdd/boardAdd left dirty item state: rev=%d body=%q", a.items["class"].Revision, a.items["class"].Body)
	}
}

func TestParticipantActionsCreateNoRevision(t *testing.T) {
	base, a, key := classPageWithForm(t)
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

	// Organizer non-content actions (share, block open/close) create no content revision.
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

	frames, closeStream := openPageEventStream(t, base, "class")
	defer closeStream()

	// blockAdd recorded r2 and set recorded r3; undo/restore each increment live Revision by +1,
	// publish 1 whole-page frame, and leave all non-content state byte-identical.
	origBody := a.items["class"].Body
	pollKey := a.items["class"].Natives["poll-1"].Key
	liveRev := a.items["class"].Revision
	wantNonContent := nonContentBytes(t, a.items["class"])

	if w := ownerAction(t, a, key, `{"action":"undo","id":"class"}`); w.Code != http.StatusOK {
		t.Fatalf("undo to r2: %d %s", w.Code, w.Body)
	}
	expectOnePageFrame(t, frames, "class", liveRev+1)
	if strings.Contains(a.items["class"].Body, "```block hands") || !strings.Contains(a.items["class"].Body, "```block poll:") || a.items["class"].Revision != liveRev+1 {
		t.Fatalf("undo to r2 mismatch: body=%q rev=%d", a.items["class"].Body, a.items["class"].Revision)
	}
	if string(nonContentBytes(t, a.items["class"])) != string(wantNonContent) {
		t.Fatal("undo mutated non-content state")
	}

	if w := ownerAction(t, a, key, `{"action":"restore","id":"class","revision":"r1"}`); w.Code != http.StatusOK {
		t.Fatalf("restore r1: %d %s", w.Code, w.Body)
	}
	expectOnePageFrame(t, frames, "class", liveRev+2)
	if strings.Contains(a.items["class"].Body, "```block poll:") || a.items["class"].Natives["poll-1"].Key != pollKey || a.items["class"].Revision != liveRev+2 {
		t.Fatalf("restore r1 failed to revert body or lost poll key/rev: body=%q rev=%d", a.items["class"].Body, a.items["class"].Revision)
	}
	if string(nonContentBytes(t, a.items["class"])) != string(wantNonContent) {
		t.Fatal("restore r1 mutated non-content state")
	}

	if w := ownerAction(t, a, key, `{"action":"restore","id":"class","revision":"r3"}`); w.Code != http.StatusOK {
		t.Fatalf("restore r3: %d %s", w.Code, w.Body)
	}
	expectOnePageFrame(t, frames, "class", liveRev+3)
	if a.items["class"].Body != origBody || a.items["class"].Revision != liveRev+3 {
		t.Fatalf("restore r3 body/rev mismatch: body=%q want=%q rev=%d", a.items["class"].Body, origBody, a.items["class"].Revision)
	}
	if string(nonContentBytes(t, a.items["class"])) != string(wantNonContent) {
		t.Fatal("restore r3 mutated non-content state")
	}
}

func TestClosedPageWriteProtectionAndErrors(t *testing.T) {
	base, a, key := classPageWithForm(t)
	a.brainstormURL = startBrainstorm(t)
	run := cli(base, a)

	if w := ownerAction(t, a, key, `{"action":"set","id":"class","title":"Final Title","body":"# Final\n\nLocked."}`); w.Code != http.StatusOK {
		t.Fatalf("set r2: %d %s", w.Code, w.Body)
	}
	if w := ownerAction(t, a, key, `{"action":"close","id":"class"}`); w.Code != http.StatusOK {
		t.Fatalf("close: %d %s", w.Code, w.Body)
	}
	revAtClose := a.items["class"].Revision
	wantNonContent := nonContentBytes(t, a.items["class"])

	frames, closeStream := openPageEventStream(t, base, "class")
	defer closeStream()

	// Read-only history remains allowed on a closed page.
	if w := ownerAction(t, a, key, `{"action":"history","id":"class"}`); w.Code != http.StatusOK {
		t.Fatalf("history on closed page: %d %s", w.Code, w.Body)
	}
	if code, out := run("history", "class", "--json"); code != 0 || !strings.Contains(out, `"active":"r2"`) {
		t.Fatalf("cli history on closed page: %d %s", code, out)
	}

	// Organizer HTML on closed page disables Restore buttons.
	orgHTML := do(a, http.MethodGet, "/?id=class", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key})
	if orgHTML.Code != http.StatusOK || !strings.Contains(orgHTML.Body.String(), `data-page-restore="r1" disabled`) {
		t.Fatalf("closed organizer page did not disable restore button: %d %s", orgHTML.Code, orgHTML.Body.String())
	}

	// All write operations are refused with 409 "This page is closed" and emit no SSE frame.
	for _, payload := range []string{
		`{"action":"set","id":"class","body":"new"}`,
		`{"action":"set","id":"class","base_rev":"r2","body":"new"}`,
		`{"action":"undo","id":"class"}`,
		`{"action":"restore","id":"class","revision":"r1"}`,
		`{"action":"block","id":"class","kind":"poll","options":["a","b"]}`,
		`{"action":"block","id":"class","kind":"board"}`,
	} {
		w := ownerAction(t, a, key, payload)
		if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "This page is closed") {
			t.Errorf("closed write %s: want 409, got %d %s", payload, w.Code, w.Body.String())
		}
	}

	// CLI undo, restore, set, and block add on closed page fail with exit 1 and surface "This page is closed".
	for _, args := range [][]string{
		{"undo", "class"},
		{"undo", "class", "--json"},
		{"restore", "class", "r1"},
		{"restore", "class", "r1", "--json"},
		{"set", "class", "--body", "nope"},
		{"block", "add", "poll", "class", "--options", "a,b"},
		{"block", "add", "board", "class"},
	} {
		code, out := run(args...)
		if code != 1 || !strings.Contains(out, "This page is closed") {
			t.Errorf("cli closed %v: want exit 1 with closed error, got %d %q", args, code, out)
		}
	}
	expectNoPageFrame(t, frames)

	if a.items["class"].Status != kit.StatusClosed || a.items["class"].Title != "Final Title" ||
		a.items["class"].Body != "# Final\n\nLocked." || a.items["class"].Revision != revAtClose {
		t.Fatalf("closed page state mutated: %+v", a.items["class"])
	}
	if string(nonContentBytes(t, a.items["class"])) != string(wantNonContent) {
		t.Fatal("closed page non-content state mutated")
	}
	if h, err := a.revisions().History("class", ""); err != nil || h.Active != "r2" || h.Previous != "r1" || len(h.Versions) != 2 {
		t.Fatalf("closed page history mutated: %+v (%v)", h, err)
	}
}

func TestCLIRevisionErrorsAndUsage(t *testing.T) {
	base, a := startServer(t, "")
	run := cli(base, a)

	if code, out := run("add", "--id", "errdoc", "--title", "V1", "--body", "Body 1"); code != 0 {
		t.Fatalf("add: %d %s", code, out)
	}

	// Undo with no previous revision returns exit 1 and error message.
	if code, out := run("undo", "errdoc"); code != 1 || !strings.Contains(out, "no previous revision") {
		t.Fatalf("cli undo with no previous (plain): %d %q", code, out)
	}
	if code, out := run("undo", "errdoc", "--json"); code != 1 || !strings.Contains(out, `"code":"conflict"`) || !strings.Contains(out, "no previous revision") {
		t.Fatalf("cli undo with no previous (json): %d %q", code, out)
	}

	// Restore of unknown revision returns exit 1 and error message.
	if code, out := run("restore", "errdoc", "r99"); code != 1 || !strings.Contains(out, "unknown revision") {
		t.Fatalf("cli restore unknown (plain): %d %q", code, out)
	}
	if code, out := run("restore", "errdoc", "r99", "--json"); code != 1 || !strings.Contains(out, `"code":"not_found"`) || !strings.Contains(out, "unknown revision") {
		t.Fatalf("cli restore unknown (json): %d %q", code, out)
	}

	// Usage validation errors return exit 2.
	for _, args := range [][]string{
		{"history"},
		{"undo"},
		{"restore", "errdoc"},
		{"undo", "errdoc", "--base-rev", "r1"},
		{"restore", "errdoc", "r1", "--base-rev", "r1"},
	} {
		if code, out := run(args...); code != 2 {
			t.Errorf("cli usage %v: want exit 2, got %d %q", args, code, out)
		}
	}
}

func TestPreS1SavedPagePreservedAndPartialWriteSafe(t *testing.T) {
	base, a := startServer(t, "")
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

	frames, closeStream := openPageEventStream(t, base, "legacy")
	defer closeStream()

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
	expectNoPageFrame(t, frames)

	// If the first post-S2 save fails on r2 after seeding r1, item state and disk roll back to pre-S1 state and no frame is emitted.
	snapDir := filepath.Join(a.dir, "revisions", "legacy", "_", "snapshots")
	if err := os.MkdirAll(filepath.Join(snapDir, "r2", "hold"), 0o700); err != nil {
		t.Fatal(err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","body":"Failed first edit"}`, "127.0.0.1:5000", owner); w.Code != http.StatusInternalServerError {
		t.Fatalf("pre-S1 failed first edit: want 500, got %d %s", w.Code, w.Body.String())
	}
	_ = os.RemoveAll(filepath.Join(snapDir, "r2"))
	expectNoPageFrame(t, frames)
	if a.items["legacy"].Title != "Pre-S1 Title" || a.items["legacy"].Body != "# Pre-S1 Title\n\nLegacy content." || a.items["legacy"].Revision != 4 {
		t.Fatalf("failed first edit left dirty legacy state: %+v", a.items["legacy"])
	}

	// Overwrite without base_rev leaves r1 as pre-S1 content and saves r2,
	// so undo brings back the prior pre-S1 title and body, increments Revision +1, and publishes 1 frame.
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","body":"Migrated body"}`, "127.0.0.1:5000", owner); w.Code != http.StatusOK {
		t.Fatalf("pre-S1 set: %d %s", w.Code, w.Body)
	}
	expectOnePageFrame(t, frames, "legacy", 5)
	if a.items["legacy"].Votes["ada@127.0.0.1"] != "a" || a.items["legacy"].Revision != 5 {
		t.Fatalf("pre-S1 votes/revision lost: %+v", a.items["legacy"])
	}
	if h, err := a.revisions().History("legacy", ""); err != nil || h.Active != "r2" || h.Previous != "r1" || len(h.Versions) != 2 {
		t.Fatalf("legacy first edit history = %+v (%v)", h, err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"undo","id":"legacy"}`, "127.0.0.1:5000", owner); w.Code != http.StatusOK {
		t.Fatalf("legacy first-edit undo: %d %s", w.Code, w.Body)
	}
	expectOnePageFrame(t, frames, "legacy", 6)
	if a.items["legacy"].Title != "Pre-S1 Title" || a.items["legacy"].Body != "# Pre-S1 Title\n\nLegacy content." || a.items["legacy"].Revision != 6 {
		t.Fatalf("legacy undo did not restore pre-S1 content cleanly: %+v", a.items["legacy"])
	}

	// Partial write error path: simulate item write failure on set, undo and restore by replacing items/legacy.json with a non-empty dir.
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","body":"Second body"}`, "127.0.0.1:5000", owner); w.Code != http.StatusOK {
		t.Fatalf("set r3: %d", w.Code)
	}
	expectOnePageFrame(t, frames, "legacy", 7)
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
	expectNoPageFrame(t, frames)
	if a.items["legacy"].Body != "Second body" || a.items["legacy"].Revision != 7 {
		t.Fatalf("in-memory item mutated on failed write: %+v", a.items["legacy"])
	}
	if err := os.RemoveAll(itemPath(a.dir, "legacy")); err != nil {
		t.Fatal(err)
	}
	if err := kit.AtomicWrite(itemPath(a.dir, "legacy"), origItemRaw); err != nil {
		t.Fatal(err)
	}

	// Partial write error path during set when revisions/ snapshots dir is blocked:
	// item file and in-memory item must roll back to match prior history, with no SSE frame.
	if err := os.MkdirAll(filepath.Join(snapDir, "r4", "hold"), 0o700); err != nil {
		t.Fatal(err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","body":"Rollback body"}`, "127.0.0.1:5000", owner); w.Code != http.StatusInternalServerError {
		t.Fatalf("set with blocked snapshot dir: want 500, got %d %s", w.Code, w.Body.String())
	}
	_ = os.RemoveAll(filepath.Join(snapDir, "r4"))
	expectNoPageFrame(t, frames)
	if a.items["legacy"].Body != "Second body" || a.items["legacy"].Revision != 7 {
		t.Fatalf("failed revision save did not roll back in-memory item: %+v", a.items["legacy"])
	}
	diskCheck := &app{dir: a.dir, items: map[string]*item{}}
	diskCheck.mu.Lock()
	diskItem, err := diskCheck.loadLocked("legacy")
	diskCheck.mu.Unlock()
	if err != nil || diskItem.Body != "Second body" || diskItem.Revision != 7 {
		t.Fatalf("failed revision save left dirty item on disk: %+v (%v)", diskItem, err)
	}

	// Restore failure when target snapshot bytes are unparseable (corrupt snapshot JSON):
	// state.json, item file, and in-memory state stay consistent at r3 / "Second body" / rev 7.
	if w := do(a, http.MethodPost, "/api/action", `{"action":"set","id":"legacy","body":"`+"```block frobnicate:bad\n```"+`"}`, "127.0.0.1:5000", owner); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid block body set: want 400, got %d", w.Code)
	}
	if err := os.WriteFile(filepath.Join(snapDir, "r1"), []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w := do(a, http.MethodPost, "/api/action", `{"action":"restore","id":"legacy","revision":"r1"}`, "127.0.0.1:5000", owner); w.Code != http.StatusInternalServerError {
		t.Fatalf("restore corrupt snapshot: want 500, got %d %s", w.Code, w.Body.String())
	}
	if h, err := a.revisions().History("legacy", ""); err != nil || h.Active != "r3" || h.Previous != "r1" {
		t.Fatalf("corrupt restore left revision pointer dirty: %+v (%v)", h, err)
	}
	expectNoPageFrame(t, frames)

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
