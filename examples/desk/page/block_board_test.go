package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	brainstormBinOnce sync.Once
	brainstormBin     string
	brainstormBinErr  error
)

// startBrainstorm runs the real brainstorm app (built from
// ../../brainstorm/server, never imported) on a free loopback port.
func startBrainstorm(t *testing.T) string {
	t.Helper()
	// A shaped fork verifies its own clone of page, without sibling sources.
	if _, err := os.Stat(filepath.Join("..", "..", "brainstorm", "server")); err != nil {
		t.Skip("brainstorm source is not beside page (a shaped fork's clone); runs in the bashy tree")
	}
	brainstormBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "page-brainstorm-bin-")
		if err != nil {
			brainstormBinErr = err
			return
		}
		brainstormBin = filepath.Join(dir, "brainstorm")
		cmd := exec.Command("go", "build", "-o", brainstormBin, ".")
		cmd.Dir = filepath.Join("..", "..", "brainstorm", "server")
		cmd.Env = append(os.Environ(), "GOWORK=off")
		if out, err := cmd.CombinedOutput(); err != nil {
			brainstormBinErr = fmt.Errorf("build brainstorm: %v\n%s", err, out)
		}
	})
	if brainstormBinErr != nil {
		t.Fatal(brainstormBinErr)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	cmd := exec.Command(brainstormBin, "-addr", addr, "-data", t.TempDir())
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
	t.Fatal("brainstorm server did not start")
	return ""
}

// classPageWithBoard starts the page (LAN audience, satellite secret) and
// the real brainstorm app.
func classPageWithBoard(t *testing.T) (string, *app, string) {
	url, a := startServer(t, "")
	a.brainstormURL = startBrainstorm(t)
	a.joinOptions.outpostSecret = testSecret
	w := do(a, http.MethodPost, "/api/action",
		`{"action":"add","id":"class","body":"# Class\n\nWelcome.","audience":"lan","join_code":"blue-fox"}`, "127.0.0.1:5000", nil)
	var created struct{ OwnerKey string }
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &created) != nil {
		t.Fatalf("add page: %d %s", w.Code, w.Body)
	}
	return url, a, created.OwnerKey
}

func (d *door) board(fields map[string]any) *httptest.ResponseRecorder {
	fields["action"], fields["id"] = "board", "class"
	raw, _ := json.Marshal(fields)
	r := httptest.NewRequest(http.MethodPost, "/api/action", bytes.NewReader(raw))
	r.Header.Set("Content-Type", "application/json")
	return d.send(r)
}

// brainstormAs calls brainstorm directly as its facilitator.
func brainstormAs(t *testing.T, a *app, key, method, path string, payload any) map[string]any {
	t.Helper()
	var body *bytes.Reader
	if payload != nil {
		raw, _ := json.Marshal(payload)
		body = bytes.NewReader(raw)
	} else {
		body = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, a.brainstormBase()+path, body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Facilitator-Key", key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out map[string]any
	if json.NewDecoder(resp.Body).Decode(&out) != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("brainstorm %s %s: %d %v", method, path, resp.StatusCode, out)
	}
	return out
}

func boardCards(t *testing.T, a *app, key, session string) map[string]float64 {
	t.Helper()
	st := brainstormAs(t, a, key, http.MethodGet, "/api/state?session="+session, nil)
	out := map[string]float64{}
	for _, raw := range st["cards"].([]any) {
		c := raw.(map[string]any)
		out[c["text"].(string)+"|"+c["name"].(string)] = c["votes"].(float64)
	}
	return out
}

func cardID(t *testing.T, a *app, key, session, text string) string {
	t.Helper()
	st := brainstormAs(t, a, key, http.MethodGet, "/api/state?session="+session, nil)
	for _, raw := range st["cards"].([]any) {
		if c := raw.(map[string]any); c["text"] == text {
			return c["id"].(string)
		}
	}
	t.Fatalf("no card %q", text)
	return ""
}

// The story's proof: page participants on both doors write cards under
// their handles and vote with one brainstorm identity each (a refresh or a
// lost cookie never adds a vote); phases stay brainstorm's; frames arrive
// over SSE; no facilitator key or voter token reaches participant HTML,
// state, block, show or export.
func TestBoardCardsAndVotesOnePerIdentity(t *testing.T) {
	url, a, key := classPageWithBoard(t)
	code, out := cli(url, a)("block", "add", "board", "class", "--title", "Retro")
	if code != 0 || !strings.Contains(out, "block: board-1 (board page-") {
		t.Fatalf("block add: %d %s", code, out)
	}
	bb := a.items["class"].Boards["board-1"]
	if bb == nil || bb.Key == "" || !strings.Contains(a.items["class"].Body, "```block board:"+bb.Session+" id=board-1\n```") {
		t.Fatalf("board record %+v body %q", bb, a.items["class"].Body)
	}
	session, fac := bb.Session, bb.Key
	if st := brainstormAs(t, a, fac, http.MethodGet, "/api/state?session="+session, nil); st["title"] != "Retro" || st["phase"] != "write" {
		t.Fatalf("session %v", st)
	}

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

	ana := &door{t: t, a: a, peer: "192.0.2.10:4000"}
	ben := &door{t: t, a: a, peer: "192.0.2.11:4000"}
	river := &door{t: t, a: a, peer: "127.0.0.1:9000", member: "river@example.test"}
	stranger := &door{t: t, a: a, peer: "192.0.2.99:4000"}
	for d, handle := range map[*door]string{ana: "Ana", ben: "Ben"} {
		if w := d.join(handle); w.Code != http.StatusSeeOther {
			t.Fatalf("join %s: %d %s", handle, w.Code, w.Body)
		}
	}
	if w := river.get("/blue-fox"); w.Code != http.StatusOK {
		t.Fatalf("member visit: %d", w.Code)
	}
	if w := stranger.board(map[string]any{"block": "board-1", "kind": "add", "text": "drive-by"}); w.Code != http.StatusForbidden {
		t.Fatalf("stranger add: %d %s", w.Code, w.Body)
	}
	for d, text := range map[*door]string{ana: "Ship sooner", ben: "Fewer meetings", river: "Pair more"} {
		if w := d.board(map[string]any{"block": "board-1", "kind": "add", "text": text, "column": 1}); w.Code != http.StatusOK {
			t.Fatalf("add %q: %d %s", text, w.Code, w.Body)
		}
	}
	for live := false; !live; {
		select {
		case f, ok := <-frames:
			if !ok {
				t.Fatal("event stream ended without a board frame")
			}
			live = strings.Contains(f, `"block":"board-1"`)
		case <-ctx.Done():
			t.Fatal("no live frame for the board block")
		}
	}

	// Writing is private: Ana sees her own card, edit controls, not Ben's.
	w := ana.get("/api/block?id=class&block=board-1")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Ship sooner") ||
		strings.Contains(w.Body.String(), "Fewer meetings") || !strings.Contains(w.Body.String(), `data-board-action="edit"`) {
		t.Fatalf("ana write view: %d %s", w.Code, w.Body)
	}
	m := regexp.MustCompile(`data-board-card="([^"]+)" data-mine><p>Ship sooner</p>`).FindStringSubmatch(w.Body.String())
	if m == nil {
		t.Fatalf("own card id: %s", w.Body)
	}
	ship := m[1]
	if w := ben.board(map[string]any{"block": "board-1", "kind": "delete", "card": ship}); w.Code != http.StatusForbidden {
		t.Fatalf("ben deletes ana's card: %d %s", w.Code, w.Body)
	}
	if w := ana.board(map[string]any{"block": "board-1", "kind": "edit", "card": ship, "text": "Ship weekly", "column": 1}); w.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", w.Code, w.Body)
	}

	// Phases are brainstorm's; a page participant cannot write while voting.
	for _, phase := range []string{"discuss", "vote"} {
		brainstormAs(t, a, fac, http.MethodPost, "/api/action", map[string]any{"action": "phase", "session": session, "phase": phase})
	}
	if w := ana.board(map[string]any{"block": "board-1", "kind": "add", "text": "late"}); w.Code != http.StatusConflict {
		t.Fatalf("add while voting: %d %s", w.Code, w.Body)
	}
	pair := cardID(t, a, fac, session, "Pair more")
	if w := ana.board(map[string]any{"block": "board-1", "kind": "vote", "card": pair}); w.Code != http.StatusOK {
		t.Fatalf("vote: %d %s", w.Code, w.Body)
	}
	// A refresh keeps the identity: voting again withdraws, never doubles.
	if w := ana.get("/blue-fox"); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `aria-pressed="true"`) {
		t.Fatalf("refresh: %d", w.Code)
	}
	if w := ana.board(map[string]any{"block": "board-1", "kind": "vote", "card": pair}); w.Code != http.StatusOK {
		t.Fatalf("unvote: %d %s", w.Code, w.Body)
	}
	if got := boardCards(t, a, fac, session)["Pair more|river@example.test"]; got != 0 {
		t.Fatalf("after toggle: %v votes", got)
	}
	// Ana loses her page cookie and reclaims her handle from the same
	// address: the same brainstorm identity, so one vote still.
	ana.cookie = ""
	if w := ana.join("Ana"); w.Code != http.StatusSeeOther {
		t.Fatalf("rejoin: %d %s", w.Code, w.Body)
	}
	for _, d := range []*door{ana, ben, river} {
		if w := d.board(map[string]any{"block": "board-1", "kind": "vote", "card": pair}); w.Code != http.StatusOK {
			t.Fatalf("vote: %d %s", w.Code, w.Body)
		}
	}
	if w := ana.board(map[string]any{"block": "board-1", "kind": "vote", "card": pair}); w.Code != http.StatusOK {
		t.Fatalf("ana unvote after rejoin: %d %s", w.Code, w.Body)
	}
	cards := boardCards(t, a, fac, session)
	if cards["Pair more|river@example.test"] != 2 || cards["Ship weekly|Ana"] != 0 || cards["Fewer meetings|Ben"] != 0 {
		t.Fatalf("cards by handle and votes: %v", cards)
	}
	if n := len(a.items["class"].BoardVoters); n != 3 {
		t.Fatalf("one brainstorm identity per participant, got %d", n)
	}

	// The organizer face links out to brainstorm's controls with the key;
	// nothing a participant or the record can read carries it or a token.
	if w := do(a, http.MethodGet, "/?id=class", "", "127.0.0.1:5000", map[string]string{"X-Facilitator-Key": key}); w.Code != http.StatusOK ||
		!strings.Contains(w.Body.String(), "#facilitator="+fac) {
		t.Fatalf("organizer face: %d", w.Code)
	}
	secrets := []string{fac}
	for _, token := range a.items["class"].BoardVoters {
		secrets = append(secrets, token)
	}
	reads := map[string]string{
		"page":  ana.get("/blue-fox").Body.String(),
		"block": ana.get("/api/block?id=class&block=board-1").Body.String(),
		"state": ana.get("/api/state?id=class").Body.String(),
	}
	_, reads["show"] = cli(url, a)("show", "class", "--json")
	for _, to := range []string{"md", "html"} {
		code, out := cli(url, a)("export", "class", "--to", to, "--key", key)
		if code != 0 {
			t.Fatalf("export %s: %d %s", to, code, out)
		}
		reads["export-"+to] = out
	}
	if !strings.Contains(reads["export-md"], "Pair more — river@example.test (2 votes)") {
		t.Fatalf("export md: %s", reads["export-md"])
	}
	for name, body := range reads {
		if name != "state" && !strings.Contains(body, "board") {
			t.Fatalf("%s does not carry the board: %s", name, body)
		}
		if (name == "page" || name == "block" || name == "export-html") && !strings.Contains(body, "Pair more") {
			t.Fatalf("%s does not render the cards", name)
		}
		for _, s := range secrets {
			if strings.Contains(body, s) {
				t.Fatalf("%s leaks a brainstorm key or token", name)
			}
		}
		if strings.Contains(body, "facilitator=") {
			t.Fatalf("%s carries a facilitator link", name)
		}
	}
}

// The organizer's "view as participant" acts on a page-local overlay:
// brainstorm never sees the preview's cards or votes.
func TestBoardPreviewNeverReachesBrainstorm(t *testing.T) {
	_, a, key := classPageWithBoard(t)
	if w := ownerAction(t, a, key, `{"action":"block","id":"class","kind":"board"}`); w.Code != http.StatusOK {
		t.Fatalf("add board: %d %s", w.Code, w.Body)
	}
	bb := a.items["class"].Boards["board-1"]
	cookies := map[string]string{}
	preview := func(method, target, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:5000"
		r.Header.Set("X-Facilitator-Key", key)
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		for name, value := range cookies {
			r.AddCookie(&http.Cookie{Name: name, Value: value})
		}
		w := httptest.NewRecorder()
		a.ServeHTTP(w, r)
		for _, c := range w.Result().Cookies() {
			cookies[c.Name] = c.Value
		}
		return w
	}
	if w := preview(http.MethodPost, "/api/action?view=participant",
		`{"action":"board","id":"class","block":"board-1","kind":"add","text":"Preview idea"}`); w.Code != http.StatusOK {
		t.Fatalf("preview add: %d %s", w.Code, w.Body)
	}
	if w := preview(http.MethodGet, "/api/block?id=class&block=board-1&view=participant", ""); !strings.Contains(w.Body.String(), "Preview idea") {
		t.Fatalf("preview block: %s", w.Body)
	}
	if len(a.items["class"].BoardVoters) != 0 {
		t.Fatal("the preview bound a brainstorm identity")
	}
	brainstormAs(t, a, bb.Key, http.MethodPost, "/api/action", map[string]any{"action": "phase", "session": bb.Session, "phase": "discuss"})
	if cards := boardCards(t, a, bb.Key, bb.Session); len(cards) != 0 {
		t.Fatalf("preview reached brainstorm: %v", cards)
	}
}

// An existing session is joined by name; the page holds no key for it, so
// the organizer link-out carries none. A missing session is refused.
func TestBoardOverExistingSession(t *testing.T) {
	url, a, _ := classPageWithBoard(t)
	if code, out := cli(url, a)("block", "add", "board", "class", "--session", "nope"); code == 0 || !strings.Contains(out, "Session not found") {
		t.Fatalf("missing session: %d %s", code, out)
	}
	if code, out := cli(url, a)("block", "add", "board", "class", "--session", "main", "--block", "retro"); code != 0 || !strings.Contains(out, "block: retro (board main)") {
		t.Fatalf("existing session: %d %s", code, out)
	}
	it := a.items["class"]
	if len(it.Boards) != 0 {
		t.Fatalf("no key is held for a session the page did not create: %v", it.Boards)
	}
	b := boardOf(it, "retro")
	if link, _ := a.boardLink(it, b); strings.Contains(link, "facilitator") || !strings.HasSuffix(link, "/?session=main") {
		t.Fatalf("link-out %q", link)
	}
}

// Moderation keeps one brainstorm identity per participant: rename and
// merge carry the token, kick drops it.
func TestBoardVotersFollowModeration(t *testing.T) {
	page := &PageItem{Participants: participantMap{
		"ana@192.0.2.1": {Handle: "Ana", Address: "192.0.2.1"},
		"ben@192.0.2.2": {Handle: "Ben", Address: "192.0.2.2"},
		"cy@192.0.2.3":  {Handle: "Cy", Address: "192.0.2.3"},
	}, BoardVoters: map[string]string{"ana@192.0.2.1": "t-ana", "ben@192.0.2.2": "t-ben"}}
	if err := moderateParticipant(page, "rename", "ana@192.0.2.1", "Anna"); err != nil {
		t.Fatal(err)
	}
	if page.BoardVoters["anna@192.0.2.1"] != "t-ana" || page.BoardVoters["ana@192.0.2.1"] != "" {
		t.Fatalf("rename: %v", page.BoardVoters)
	}
	if err := moderateParticipant(page, "merge", "ben@192.0.2.2", "cy@192.0.2.3"); err != nil {
		t.Fatal(err)
	}
	if page.BoardVoters["cy@192.0.2.3"] != "t-ben" || page.BoardVoters["ben@192.0.2.2"] != "" {
		t.Fatalf("merge: %v", page.BoardVoters)
	}
	if err := moderateParticipant(page, "kick", "cy@192.0.2.3", ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := page.BoardVoters["cy@192.0.2.3"]; ok {
		t.Fatalf("kick kept the token: %v", page.BoardVoters)
	}
}
