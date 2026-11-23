package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	formBinOnce sync.Once
	formBin     string
	formBinErr  error
)

// startForm runs the real form app (built from ../form/server, never
// imported) on a free loopback port, the way the page composes with it.
func startForm(t *testing.T) string {
	t.Helper()
	// A shaped fork verifies its own clone of page, without sibling sources.
	if _, err := os.Stat(filepath.Join("..", "form", "server")); err != nil {
		t.Skip("form source is not beside page (a shaped fork's clone); runs in the bashy tree")
	}
	formBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "page-form-bin-")
		if err != nil {
			formBinErr = err
			return
		}
		formBin = filepath.Join(dir, "form")
		cmd := exec.Command("go", "build", "-o", formBin, "./server")
		cmd.Dir = filepath.Join("..", "form")
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			formBinErr = fmt.Errorf("build form: %v\n%s", err, out)
		}
	})
	if formBinErr != nil {
		t.Fatal(formBinErr)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	cmd := exec.Command(formBin, "serve", "--addr", addr, "--data", t.TempDir())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	base := "http://" + addr
	for i := 0; i < 100; i++ {
		if r, err := http.Get(base + "/healthz"); err == nil {
			r.Body.Close()
			return base
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("form server did not start")
	return ""
}

const testSecret = "test-only"
const satellitePrefix = "/matrix/h/example/app/page"

// door is one fake participant: a peer address (the LAN door) or the
// outpost tunnel with signed headers (the satellite door), plus the
// page-participant cookie the browser would keep.
type door struct {
	t       *testing.T
	a       *app
	peer    string
	forward string // satellite: the forwarded client address
	member  string // satellite: the signed member name
	cookie  string
}

func (d *door) headers(r *http.Request) {
	r.RemoteAddr = d.peer
	r.Header.Set("User-Agent", "page-proof")
	if d.cookie != "" {
		r.Header.Set("Cookie", pageParticipantCookie+"="+d.cookie)
	}
	if d.forward == "" && d.member == "" {
		return
	}
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	r.Header.Set("X-Forwarded-Prefix", satellitePrefix)
	r.Header.Set("X-Outpost-Identity-Ts", ts)
	if d.forward != "" {
		r.Header.Set("X-Forwarded-For", d.forward+", 127.0.0.1")
	}
	if d.member != "" {
		r.Header.Set("X-Outpost-Relationship", "member")
		r.Header.Set("Remote-User", d.member)
		signJoinHeaders(r, testSecret, []string{"v2", "member", "", "", "", d.member, satellitePrefix, ts}, "X-Outpost-Relationship-Sig")
	} else {
		signJoinHeaders(r, testSecret, []string{"", "", satellitePrefix, ts}, "X-Outpost-Identity-Sig")
	}
}

func (d *door) send(r *http.Request) *httptest.ResponseRecorder {
	d.headers(r)
	w := httptest.NewRecorder()
	d.a.ServeHTTP(w, r)
	for _, c := range w.Result().Cookies() {
		if c.Name == pageParticipantCookie {
			d.cookie = c.Value
		}
	}
	return w
}

func (d *door) get(path string) *httptest.ResponseRecorder {
	return d.send(httptest.NewRequest(http.MethodGet, path, nil))
}

func (d *door) join(handle string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, "/blue-fox", strings.NewReader(url.Values{"handle": {handle}}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return d.send(r)
}

func (d *door) answer(block string, answers any) *httptest.ResponseRecorder {
	raw, _ := json.Marshal(map[string]any{"action": "answer", "id": "class", "block": block, "answers": answers})
	r := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(string(raw)))
	r.Header.Set("Content-Type", "application/json")
	return d.send(r)
}

// classPageWithForm starts the page (LAN audience, satellite secret) and
// the real form app.
func classPageWithForm(t *testing.T) (string, *app, string) {
	url, a := startServer(t, "")
	a.formURL = startForm(t)
	a.joinOptions.outpostSecret = testSecret
	w := do(a, http.MethodPost, "/api/action",
		`{"action":"add","id":"class","body":"# Class\n\nWelcome.","audience":"lan","join_code":"blue-fox"}`, "127.0.0.1:5000", nil)
	var created struct{ OwnerKey string }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &created) != nil {
		t.Fatalf("add page: %d %s", w.Code, w.Body)
	}
	return url, a, created.OwnerKey
}

func ownerAction(t *testing.T, a *app, key, body string) *httptest.ResponseRecorder {
	t.Helper()
	return do(a, http.MethodPost, "/api/action", body, "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key})
}

func results(t *testing.T, a *app, key, block string) nativeResults {
	t.Helper()
	w := ownerAction(t, a, key, `{"action":"results","id":"class","block":"`+block+`"}`)
	var res nativeResults
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &res) != nil {
		t.Fatalf("results: %d %s", w.Code, w.Body)
	}
	return res
}

func tally(res nativeResults) map[string]string {
	out := map[string]string{}
	for _, o := range res.Options {
		out[o.Option] = strconv.Itoa(o.Votes) + ":" + strings.Join(o.Handles, ",")
	}
	return out
}

// The story's proof: a three-votes-per-participant ballot with distinct
// fake identities on both doors, one refreshing (and losing its cookie)
// and re-voting, counted once; tallies arrive over the SSE stream; the
// participant face, state and export carry no organizer face, form key or
// address.
func TestThreeVoteBallotBothDoorsCountedOnce(t *testing.T) {
	url, a, key := classPageWithForm(t)
	code, out := cli(url, a)("block", "add", "poll", "class", "--options", "red,green,blue,amber", "--votes", "3", "--title", "Pick three")
	if code != 0 || !strings.Contains(out, "block: poll-1 (poll form-") {
		t.Fatalf("block add: %d %s", code, out)
	}
	nb := a.items["class"].Natives["poll-1"]
	if nb == nil || nb.Key == "" || !nb.Open || nb.Votes != 3 {
		t.Fatalf("native block %+v", nb)
	}
	if !strings.Contains(a.items["class"].Body, "```block poll:"+nb.Form+" id=poll-1\n```") {
		t.Fatalf("fence not appended: %q", a.items["class"].Body)
	}

	// The live stream, opened before any vote.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sreq, _ := http.NewRequestWithContext(ctx, http.MethodGet, url+"/api/events?id=class", nil)
	stream, err := http.DefaultClient.Do(sreq)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	frames := make(chan string, 64)
	go func() {
		sc := bufio.NewScanner(stream.Body)
		for sc.Scan() {
			if data, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
				frames <- data
			}
		}
		close(frames)
	}()

	ana := &door{t: t, a: a, peer: "192.0.2.10:4000"}                                // LAN
	ben := &door{t: t, a: a, peer: "192.0.2.11:4000"}                                // LAN
	river := &door{t: t, a: a, peer: "127.0.0.1:9000", member: "river@example.test"} // satellite member
	dee := &door{t: t, a: a, peer: "127.0.0.1:9001", forward: "198.51.100.7"}        // satellite anonymous
	for d, handle := range map[*door]string{ana: "Ana", ben: "Ben", dee: "Dee"} {
		if w := d.join(handle); w.Code != http.StatusSeeOther {
			t.Fatalf("join %s: %d %s", handle, w.Code, w.Body)
		}
	}
	if w := river.get("/blue-fox"); w.Code != http.StatusOK {
		t.Fatalf("member visit: %d", w.Code)
	}
	ballots := []struct {
		d     *door
		picks []string
	}{
		{ana, []string{"red", "green", "blue"}},
		{ben, []string{"red"}},
		{river, []string{"green", "amber"}},
		{dee, []string{"red", "blue", "amber"}},
	}
	for _, b := range ballots {
		if w := b.d.answer("poll-1", map[string]any{"vote": b.picks}); w.Code != http.StatusOK {
			t.Fatalf("vote %v: %d %s", b.picks, w.Code, w.Body)
		}
	}
	for live := false; !live; {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatal("event stream ended without a poll frame")
			}
			live = strings.Contains(f, `"block":"poll-1"`)
		case <-ctx.Done():
			t.Fatal("no live frame for the poll block")
		}
	}

	// Ana refreshes and re-votes; then loses her cookie, reclaims her
	// handle from the same address and votes again.
	if w := ana.get("/blue-fox"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `data-native-block="poll-1"`) {
		t.Fatalf("refresh: %d", w.Code)
	}
	if w := ana.answer("poll-1", map[string]any{"vote": []string{"amber"}}); w.Code != http.StatusOK {
		t.Fatalf("re-vote: %d %s", w.Code, w.Body)
	}
	ana.cookie = ""
	if w := ana.join("Ana"); w.Code != http.StatusSeeOther {
		t.Fatalf("reclaim: %d %s", w.Code, w.Body)
	}
	if w := ana.answer("poll-1", map[string]any{"vote": []string{"amber", "green"}}); w.Code != http.StatusOK {
		t.Fatalf("vote after reclaim: %d %s", w.Code, w.Body)
	}

	// Refusals: over budget, a non-option, the same handle from another
	// address, and a caller who has not joined.
	if w := ben.answer("poll-1", map[string]any{"vote": []string{"red", "green", "blue", "amber"}}); w.Code != http.StatusBadRequest {
		t.Fatalf("four votes: %d", w.Code)
	}
	if w := ben.answer("poll-1", map[string]any{"vote": []string{"pink"}}); w.Code != http.StatusBadRequest {
		t.Fatalf("non-option: %d", w.Code)
	}
	impostor := &door{t: t, a: a, peer: "192.0.2.99:4000"}
	if w := impostor.join("Ana"); w.Code != http.StatusConflict {
		t.Fatalf("same handle, second address: %d", w.Code)
	}
	if w := impostor.answer("poll-1", map[string]any{"vote": []string{"red"}}); w.Code != http.StatusForbidden {
		t.Fatalf("vote without joining: %d", w.Code)
	}

	res := results(t, a, key, "poll-1")
	want := map[string]string{
		"red":   "2:Ben,Dee",
		"green": "2:Ana,river@example.test",
		"blue":  "1:Dee",
		"amber": "3:Ana,Dee,river@example.test",
	}
	if res.Count != 4 || fmt.Sprint(tally(res)) != fmt.Sprint(want) {
		t.Fatalf("tally %d %v, want 4 %v", res.Count, tally(res), want)
	}
	if len(res.Answers) != 4 || res.Answers[0].Handle != "Ana" || answerText(res.Answers[0].Answers["vote"]) != "amber, green" {
		t.Fatalf("handle -> answer %+v", res.Answers)
	}
	// form itself holds exactly one respondent per identity.
	var snap formSnapshot
	if err := a.formCall(nb.Key, "", map[string]any{"action": "show", "id": nb.Form}, &snap); err != nil || len(snap.Results.Respondents) != 4 {
		t.Fatalf("form respondents %d: %v", len(snap.Results.Respondents), err)
	}

	// Security contract on every participant surface.
	secrets := []string{nb.Key, key, "192.0.2.10", "192.0.2.11", "198.51.100.7"}
	for token := range snap.Results.Respondents {
		secrets = append(secrets, token)
	}
	participantViews := map[string]string{
		"page":  ana.get("/blue-fox").Body.String(),
		"block": ana.get("/api/block?id=class&block=poll-1").Body.String(),
		"state": ana.get("/api/state?id=class").Body.String(),
		"show":  ana.send(jsonReq(`{"action":"show","id":"class"}`)).Body.String(),
	}
	for name, body := range participantViews {
		forbidden := append(secrets, `data-face="organizer"`)
		if name != "show" { // show lists the page roster (handle, auth) by design
			forbidden = append(forbidden, "river@example.test", "Ben,")
		}
		for _, s := range forbidden {
			if strings.Contains(body, s) {
				t.Errorf("participant %s carries %q", name, s)
			}
		}
	}
	if !strings.Contains(participantViews["block"], `data-face="participant"`) || !strings.Contains(participantViews["block"], `value="amber" checked`) {
		t.Errorf("participant face lacks own vote: %s", participantViews["block"])
	}
	if strings.Contains(participantViews["block"], "data-native-results") {
		t.Error("results shown to participants before the organizer made them visible")
	}

	// The organizer face and the record.
	organizer := do(a, http.MethodGet, "/?id=class", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key}).Body.String()
	if !strings.Contains(organizer, `data-face="organizer"`) || !strings.Contains(organizer, "Ana, Dee, river@example.test") {
		t.Errorf("organizer face lacks option -> handles")
	}
	for _, to := range []string{"md", "html"} {
		w := do(a, http.MethodGet, "/api/export?id=class&to="+to, "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key})
		body := w.Body.String()
		if w.Code != http.StatusOK {
			t.Fatalf("export %s: %d", to, w.Code)
		}
		for _, s := range secrets {
			if strings.Contains(body, s) {
				t.Errorf("export %s carries %q", to, s)
			}
		}
		if to == "md" && !strings.Contains(body, "| amber | 3 | Ana, Dee, river@example.test |") {
			t.Errorf("export md lacks the tally with handles:\n%s", body)
		}
	}
}

func jsonReq(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/action", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	return r
}

// Organizer controls: close refuses votes and shows closed, open accepts
// them again, results visibility shows counts (never handles) to
// participants; every change pushes the block's frame.
func TestOrganizerOpenCloseAndResultsVisibility(t *testing.T) {
	url, a, _ := classPageWithForm(t)
	if code, out := cli(url, a)("block", "add", "poll", "class", "--options", "yes,no", "--closed", "--block", "go"); code != 0 {
		t.Fatalf("block add: %s", out)
	}
	ana := &door{t: t, a: a, peer: "192.0.2.10:4000"}
	ben := &door{t: t, a: a, peer: "192.0.2.11:4000"}
	ana.join("Ana")
	ben.join("Ben")
	if w := ana.answer("go", map[string]any{"vote": "yes"}); w.Code != http.StatusConflict {
		t.Fatalf("vote on a closed poll: %d", w.Code)
	}
	if body := ana.get("/api/block?id=class&block=go").Body.String(); !strings.Contains(body, `data-native-status="closed"`) || strings.Contains(body, `type="submit"`) {
		t.Fatalf("closed face: %s", body)
	}
	if w := ana.answer("go", map[string]any{"vote": "yes"}); w.Code != http.StatusConflict {
		t.Fatal("still closed")
	}
	before := a.items["class"].Revision
	if code, out := cli(url, a)("block", "open", "class", "go"); code != 0 || !strings.Contains(out, "open=true") {
		t.Fatalf("open: %s", out)
	}
	if a.items["class"].Revision != before+1 {
		t.Fatal("open did not save a revision (no live frame)")
	}
	if w := ana.answer("go", map[string]any{"vote": "yes"}); w.Code != http.StatusOK {
		t.Fatalf("vote after open: %d %s", w.Code, w.Body)
	}
	ben.answer("go", map[string]any{"vote": "no"})
	if w := ana.answer("go", map[string]any{"vote": []string{"yes", "no"}}); w.Code != http.StatusBadRequest {
		t.Fatalf("two votes on a one-vote poll: %d", w.Code)
	}
	if code, out := cli(url, a)("block", "show-results", "class", "go"); code != 0 {
		t.Fatalf("show-results: %s", out)
	}
	body := ana.get("/api/block?id=class&block=go").Body.String()
	if !strings.Contains(body, "data-native-results") || !strings.Contains(body, "2 responses") || strings.Contains(body, "Ben") {
		t.Fatalf("visible results face: %s", body)
	}
	cli(url, a)("block", "hide-results", "class", "go")
	if body := ana.get("/api/block?id=class&block=go").Body.String(); strings.Contains(body, "data-native-results") {
		t.Fatal("results still visible after hide-results")
	}
	cli(url, a)("block", "close", "class", "go")
	if w := ben.answer("go", map[string]any{"vote": "yes"}); w.Code != http.StatusConflict {
		t.Fatalf("vote after close: %d", w.Code)
	}
	code, out := cli(url, a)("block", "results", "class", "go")
	if code != 0 || !strings.Contains(out, "yes: 1 (Ana)") || !strings.Contains(out, "no: 1 (Ben)") || !strings.Contains(out, "closed") {
		t.Fatalf("results text: %s", out)
	}
	// Only the organizer controls a block or reads its results.
	for _, body := range []string{`{"action":"block","id":"class","block":"go","kind":"open"}`, `{"action":"results","id":"class","block":"go"}`,
		`{"action":"block","id":"class","kind":"poll","options":["a","b"]}`} {
		if w := ana.send(jsonReq(body)); w.Code != http.StatusForbidden {
			t.Errorf("participant %s: %d", body, w.Code)
		}
	}
}

// --anonymous keeps one tally per identity but hides handles everywhere:
// the results view, the organizer face and the export.
func TestAnonymousPollHidesHandles(t *testing.T) {
	url, a, key := classPageWithForm(t)
	if code, out := cli(url, a)("block", "add", "poll", "class", "--options", "a,b", "--anonymous", "--block", "secret-ballot"); code != 0 {
		t.Fatalf("block add: %s", out)
	}
	ana := &door{t: t, a: a, peer: "192.0.2.10:4000"}
	ben := &door{t: t, a: a, peer: "192.0.2.11:4000"}
	ana.join("Ana")
	ben.join("Ben")
	ana.answer("secret-ballot", map[string]any{"vote": "a"})
	ana.answer("secret-ballot", map[string]any{"vote": "b"})
	ben.answer("secret-ballot", map[string]any{"vote": "b"})
	res := results(t, a, key, "secret-ballot")
	raw, _ := json.Marshal(res)
	if res.Count != 2 || fmt.Sprint(tally(res)) != fmt.Sprint(map[string]string{"a": "0:", "b": "2:"}) || strings.Contains(string(raw), "Ana") {
		t.Fatalf("anonymous results %s", raw)
	}
	organizer := do(a, http.MethodGet, "/api/block?id=class&block=secret-ballot", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key}).Body.String()
	md := do(a, http.MethodGet, "/api/export?id=class&to=md", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key}).Body.String()
	section, _, _ := strings.Cut(md, "## Participants")
	for name, body := range map[string]string{"organizer face": organizer, "export block": section} {
		if strings.Contains(body, "Ana") || strings.Contains(body, "Ben") {
			t.Errorf("%s names a voter:\n%s", name, body)
		}
	}
	if !strings.Contains(section, "| b | 2 |") {
		t.Errorf("export lacks the anonymous tally:\n%s", section)
	}
}

// A form block collects one answer per identity (a resubmit replaces it);
// the results view and the record list handle -> answer; a rename moves
// the identity's answer with it, a kick drops it from the tally.
func TestFormBlockCollectsPerHandle(t *testing.T) {
	url, a, key := classPageWithForm(t)
	def := filepath.Join(t.TempDir(), "team.json")
	os.WriteFile(def, []byte(`{"title":"Teams","fields":[{"id":"team","label":"Team name","kind":"text","required":true},{"id":"lead","label":"Lead","kind":"yes-no"}]}`), 0o600)
	if code, out := cli(url, a)("block", "add", "form", "class", "--def", def); code != 0 || !strings.Contains(out, "block: form-1") {
		t.Fatalf("block add form: %s", out)
	}
	bad := filepath.Join(t.TempDir(), "file.json")
	os.WriteFile(bad, []byte(`[{"id":"cv","label":"CV","kind":"file"}]`), 0o600)
	if code, _ := cli(url, a)("block", "add", "form", "class", "--def", bad); code == 0 {
		t.Fatal("a file field was accepted")
	}
	ana := &door{t: t, a: a, peer: "192.0.2.10:4000"}
	ben := &door{t: t, a: a, peer: "192.0.2.11:4000"}
	ana.join("Ana")
	ben.join("Ben")
	if w := ana.answer("form-1", map[string]any{"team": "Owls", "lead": "yes"}); w.Code != http.StatusOK {
		t.Fatalf("answer: %d %s", w.Code, w.Body)
	}
	ana.answer("form-1", map[string]any{"team": "Hawks", "lead": "no"})
	if w := ben.answer("form-1", map[string]any{"lead": "yes"}); w.Code != http.StatusBadRequest {
		t.Fatalf("missing required field: %d %s", w.Code, w.Body)
	}
	ben.answer("form-1", map[string]any{"team": "Owls", "lead": "yes"})
	res := results(t, a, key, "form-1")
	if res.Count != 2 || res.Answers[0].Handle != "Ana" || res.Answers[0].Answers["team"] != "Hawks" || res.Answers[1].Handle != "Ben" {
		t.Fatalf("handle -> answer %+v", res.Answers)
	}
	md := do(a, http.MethodGet, "/api/export?id=class&to=md", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key}).Body.String()
	if !strings.Contains(md, "| Participant | Team name | Lead |") || !strings.Contains(md, "| Ana | Hawks | no |") {
		t.Fatalf("record:\n%s", md)
	}
	if body := ana.get("/api/block?id=class&block=form-1").Body.String(); !strings.Contains(body, `value="Hawks"`) || strings.Contains(body, "Owls") {
		t.Fatalf("participant face shows someone else's answer or misses its own: %s", body)
	}
	if w := ownerAction(t, a, key, `{"action":"participant","id":"class","kind":"rename","source":"Ben","target":"Benjamin"}`); w.Code != http.StatusOK {
		t.Fatalf("rename: %d %s", w.Code, w.Body)
	}
	if res := results(t, a, key, "form-1"); res.Count != 2 || res.Answers[1].Handle != "Benjamin" {
		t.Fatalf("after rename %+v", res.Answers)
	}
	if w := ownerAction(t, a, key, `{"action":"participant","id":"class","kind":"kick","source":"Ana"}`); w.Code != http.StatusOK {
		t.Fatalf("kick: %d %s", w.Code, w.Body)
	}
	if res := results(t, a, key, "form-1"); res.Count != 1 || res.Answers[0].Handle != "Benjamin" {
		t.Fatalf("after kick %+v", res.Answers)
	}
}

// Without the form app the page still renders and says so; nothing leaks.
func TestNativeBlockFormUnreachable(t *testing.T) {
	_, a, key := classPageWithForm(t)
	w := ownerAction(t, a, key, `{"action":"block","id":"class","kind":"poll","options":["a","b"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	a.formURL = "http://127.0.0.1:1"
	ana := &door{t: t, a: a, peer: "192.0.2.10:4000"}
	ana.join("Ana")
	if body := ana.get("/blue-fox").Body.String(); !strings.Contains(body, "not available right now") {
		t.Fatalf("unreachable form face: %s", body)
	}
	if w := ana.answer("poll-1", map[string]any{"vote": "a"}); w.Code != http.StatusBadGateway {
		t.Fatalf("answer with form down: %d", w.Code)
	}
	if w := ownerAction(t, a, key, `{"action":"block","id":"class","kind":"poll","options":["a"]}`); w.Code != http.StatusBadRequest {
		t.Fatalf("one-option poll: %d", w.Code)
	}
}

func TestCheckBallot(t *testing.T) {
	nb := &nativeBlock{Options: []string{"a", "b", "c"}, Votes: 2}
	for _, c := range []struct {
		vote any
		ok   bool
	}{
		{[]any{"a"}, true}, {[]any{"a", "b"}, true}, {[]any{"a", "b", "c"}, false},
		{[]any{"a", "a"}, false}, {[]any{}, false}, {"a", true}, {[]any{"x"}, false}, {3.0, false},
	} {
		err := checkBallot(nb, map[string]any{"vote": c.vote})
		if (err == nil) != c.ok {
			t.Errorf("vote %v: %v", c.vote, err)
		}
	}
	if err := checkBallot(nb, map[string]any{"vote": "a", "extra": "x"}); err == nil {
		t.Error("extra answer accepted")
	}
}
