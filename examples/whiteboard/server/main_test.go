package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func request(a *app, who, key, path string, body any) *httptest.ResponseRecorder {
	method := "GET"
	var b bytes.Buffer
	if body != nil {
		method = "POST"
		json.NewEncoder(&b).Encode(body)
	}
	r := httptest.NewRequest(method, path, &b)
	if who != "" {
		r.AddCookie(&http.Cookie{Name: "whiteboard-participant", Value: who})
	}
	if key != "" {
		r.Header.Set("X-Facilitator-Key", key)
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	return w
}
func fresh(t *testing.T) *app {
	t.Helper()
	return &app{dir: t.TempDir(), boards: map[string]*Board{}}
}
func good(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}
func create(t *testing.T, a *app, board string) string {
	t.Helper()
	w := request(a, token(), "", "/api/action", map[string]any{"action": "create", "board": board})
	good(t, w)
	var v struct {
		FacilitatorKey string `json:"facilitatorKey"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || !validToken(v.FacilitatorKey) {
		t.Fatalf("create did not mint a facilitator capability: %s", w.Body.String())
	}
	if request(a, token(), "", "/api/action", map[string]any{"action": "create", "board": board}).Code != 409 {
		t.Fatal("duplicate board accepted")
	}
	return v.FacilitatorKey
}
func invite(t *testing.T, a *app, fac, board string) string {
	t.Helper()
	w := request(a, token(), fac, "/api/action", map[string]any{"action": "invite", "board": board})
	good(t, w)
	var v struct {
		Invite     string `json:"invite"`
		InviteCode string `json:"inviteCode"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || !validToken(v.InviteCode) || v.Invite != v.InviteCode {
		t.Fatalf("invite did not mint a code: %s", w.Body.String())
	}
	return v.InviteCode
}
func join(t *testing.T, a *app, who, board, code string) {
	t.Helper()
	good(t, request(a, who, "", "/api/action", map[string]any{"action": "join", "board": board, "invite": code}))
}
func stroke(t *testing.T, a *app, who, key, board string) string {
	t.Helper()
	w := request(a, who, key, "/api/action", map[string]any{
		"action": "stroke", "board": board,
		"points": []Pt{{X: 10, Y: 10}, {X: 200, Y: 150}},
		"color":  "#1f5c53", "width": 4,
	})
	good(t, w)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.boards[board].Strokes[len(a.boards[board].Strokes)-1].ID
}
func note(t *testing.T, a *app, who, board, text string) string {
	t.Helper()
	w := request(a, who, "", "/api/action", map[string]any{
		"action": "note", "board": board, "x": 120, "y": 120, "text": text,
	})
	good(t, w)
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.boards[board].Notes[len(a.boards[board].Notes)-1].ID
}

func TestFacilitatorParticipantIsolation(t *testing.T) {
	a := fresh(t)
	fac := create(t, a, "team1")
	alice, bob := token(), token()
	code := invite(t, a, fac, "team1")
	join(t, a, alice, "team1", code)
	if request(a, bob, "", "/api/action", map[string]any{"action": "join", "board": "team1", "invite": code}).Code != 403 {
		t.Fatal("used invite admitted a second participant")
	}
	join(t, a, bob, "team1", invite(t, a, fac, "team1"))
	sid := stroke(t, a, alice, "", "team1")
	nid := note(t, a, alice, "team1", "SECRET <script>alert(1)</script>")
	// State and exports never carry capabilities or owner tokens.
	for _, w := range []*httptest.ResponseRecorder{
		request(a, bob, "", "/api/state?board=team1", nil),
		request(a, bob, "", "/api/export?board=team1&format=json", nil),
	} {
		good(t, w)
		for _, leak := range []string{fac, alice, code, "owner", "facilitatorKey", "inviteCode"} {
			if strings.Contains(w.Body.String(), leak) {
				t.Fatalf("capability or owner leaked: %q", leak)
			}
		}
		if strings.Contains(w.Body.String(), `"members"`) || strings.Contains(w.Body.String(), `"invites"`) {
			t.Fatal("member roster leaked to participant")
		}
	}
	// A stranger with only a guessed URL is denied on every path.
	stranger := token()
	if request(a, stranger, "", "/api/state?board=team1", nil).Code != 403 {
		t.Fatal("nonmember read state")
	}
	if request(a, stranger, "", "/api/export?board=team1&format=json", nil).Code != 403 {
		t.Fatal("nonmember read export")
	}
	if request(a, stranger, "", "/api/export?board=team1&format=png", nil).Code != 403 {
		t.Fatal("nonmember read png export")
	}
	if request(a, stranger, "", "/api/action", map[string]any{
		"action": "stroke", "board": "team1",
		"points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "#000000", "width": 2,
	}).Code != 403 {
		t.Fatal("nonmember stroke accepted")
	}
	// Only the facilitator can mint invites; participants cannot.
	if request(a, bob, "", "/api/action", map[string]any{"action": "invite", "board": "team1"}).Code != 403 {
		t.Fatal("participant minted an invite")
	}
	// Participants cannot touch each other's work or the facilitator controls.
	if request(a, bob, "", "/api/action", map[string]any{"action": "delete-stroke", "board": "team1", "id": sid}).Code != 403 {
		t.Fatal("participant deleted another stroke")
	}
	if request(a, bob, "", "/api/action", map[string]any{"action": "edit-note", "board": "team1", "id": nid, "text": "hijacked"}).Code != 403 {
		t.Fatal("participant edited another note")
	}
	if request(a, bob, "", "/api/action", map[string]any{"action": "clear", "board": "team1"}).Code != 403 {
		t.Fatal("participant cleared the board")
	}
	if request(a, bob, "", "/api/action", map[string]any{"action": "lock", "board": "team1", "locked": true}).Code != 403 {
		t.Fatal("participant locked the board")
	}
	// The facilitator can moderate and lock; a locked board stops participants only.
	good(t, request(a, token(), fac, "/api/action", map[string]any{"action": "delete-note", "board": "team1", "id": nid}))
	good(t, request(a, token(), fac, "/api/action", map[string]any{"action": "lock", "board": "team1", "locked": true}))
	if request(a, bob, "", "/api/action", map[string]any{"action": "stroke", "board": "team1",
		"points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "#000000", "width": 2}).Code != 403 {
		t.Fatal("locked board accepted a participant stroke")
	}
	if request(a, bob, "", "/api/action", map[string]any{"action": "note", "board": "team1", "x": 5, "y": 5, "text": "late"}).Code != 403 {
		t.Fatal("locked board accepted a participant note")
	}
	stroke(t, a, token(), fac, "team1")
	good(t, request(a, token(), fac, "/api/action", map[string]any{"action": "lock", "board": "team1", "locked": false}))
	stroke(t, a, bob, "", "team1")
	// Boards are isolated sessions.
	other := create(t, a, "team2")
	good(t, request(a, token(), other, "/api/action", map[string]any{"action": "clear", "board": "team2"}))
	if got := len(a.boards["team1"].Strokes); got != 3 {
		t.Fatalf("clear leaked across boards: team1 has %d strokes", got)
	}
	if request(a, token(), fac, "/api/action", map[string]any{"action": "clear", "board": "team2"}).Code != 403 {
		t.Fatal("facilitator key worked on another board")
	}
}

func TestPersistenceAcrossRestart(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, boards: map[string]*Board{}}
	fac := create(t, a, "retro")
	code := invite(t, a, fac, "retro")
	member := token()
	join(t, a, member, "retro", code)
	stroke(t, a, member, "", "retro")
	note(t, a, member, "retro", "keep me")
	good(t, request(a, token(), fac, "/api/action", map[string]any{"action": "lock", "board": "retro", "locked": true}))
	before := a.boards["retro"].Revision
	b := &app{dir: dir, boards: map[string]*Board{}}
	good(t, request(b, member, "", "/api/state?board=retro", nil))
	got := b.boards["retro"]
	if len(got.Strokes) != 1 || len(got.Notes) != 1 || !got.Locked || got.Revision != before || got.Facilitator != fac {
		t.Fatalf("restart lost state: %+v", got)
	}
	if !got.isMember(member) || len(got.Invites) != 0 {
		t.Fatal("restart lost membership")
	}
}

func TestConcurrentDraw(t *testing.T) {
	a := fresh(t)
	fac := create(t, a, "jam")
	code := invite(t, a, fac, "jam")
	members := make([]string, 30)
	for i := range members {
		members[i] = token()
		if i > 0 {
			code = invite(t, a, fac, "jam")
		}
		join(t, a, members[i], "jam", code)
	}
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			who := members[i]
			w := request(a, who, "", "/api/action", map[string]any{
				"action": "stroke", "board": "jam",
				"points": []Pt{{X: float64(i * 10), Y: 10}, {X: float64(i*10 + 5), Y: 60}},
				"color":  "#123456", "width": 3,
			})
			if w.Code != 200 {
				t.Errorf("participant %d stroke: %d", i, w.Code)
				return
			}
			if w := request(a, who, "", "/api/action", map[string]any{
				"action": "note", "board": "jam", "x": float64(i * 10), "y": 200, "text": fmt.Sprintf("note-%d", i),
			}); w.Code != 200 {
				t.Errorf("participant %d note: %d", i, w.Code)
			}
		}(i)
	}
	wg.Wait()
	if len(a.boards["jam"].Strokes) != 30 || len(a.boards["jam"].Notes) != 30 {
		t.Fatalf("lost writes: %d strokes %d notes", len(a.boards["jam"].Strokes), len(a.boards["jam"].Notes))
	}
	b := &app{dir: a.dir, boards: map[string]*Board{}}
	good(t, request(b, members[0], "", "/api/state?board=jam", nil))
	if len(b.boards["jam"].Strokes) != 30 || len(b.boards["jam"].Notes) != 30 {
		t.Fatal("disk state differs after concurrent draw")
	}
}

func TestWriteFailureIsNotAcknowledged(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, boards: map[string]*Board{}}
	fac := create(t, a, "fragile")
	code := invite(t, a, fac, "fragile")
	member := token()
	join(t, a, member, "fragile", code)
	rev := a.boards["fragile"].Revision
	boardsDir := filepath.Join(dir, "boards")
	if err := os.Chmod(boardsDir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(boardsDir, 0700)
	w := request(a, member, "", "/api/action", map[string]any{
		"action": "stroke", "board": "fragile",
		"points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "#000000", "width": 2,
	})
	if w.Code != 500 || len(a.boards["fragile"].Strokes) != 0 || a.boards["fragile"].Revision != rev {
		t.Fatalf("failed save changed state: %d rev %d", w.Code, a.boards["fragile"].Revision)
	}
}

func TestBoundsAndAccess(t *testing.T) {
	a := fresh(t)
	facBounds := create(t, a, "bounds")
	codeBounds := invite(t, a, facBounds, "bounds")
	memberBounds := token()
	join(t, a, memberBounds, "bounds", codeBounds)
	for _, body := range []map[string]any{
		{"action": "stroke", "board": "bounds", "points": []Pt{{X: 1, Y: 1}}, "color": "#000000", "width": 2},
		{"action": "stroke", "board": "bounds", "points": []Pt{{X: 1, Y: 1}, {X: 9999, Y: 2}}, "color": "#000000", "width": 2},
		{"action": "stroke", "board": "bounds", "points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "red", "width": 2},
		{"action": "stroke", "board": "bounds", "points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "#000000", "width": 99},
		{"action": "note", "board": "bounds", "x": 10, "y": 10, "text": ""},
		{"action": "note", "board": "bounds", "x": 10, "y": 10, "text": strings.Repeat("x", 501)},
		{"action": "note", "board": "bounds", "x": -5, "y": 10, "text": "off-canvas"},
		{"action": "stroke", "board": "../escape", "points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "#000000", "width": 2},
		{"action": "bogus", "board": "bounds"},
	} {
		if w := request(a, memberBounds, "", "/api/action", body); w.Code == 200 {
			t.Fatalf("invalid input accepted: %v", body)
		}
	}
	if w := request(a, token(), "", "/api/state?board=../escape", nil); w.Code == 200 {
		t.Fatal("board name escape accepted")
	}
	r := httptest.NewRequest("POST", "http://board/api/action", strings.NewReader(`{"action":"note","board":"bounds","x":1,"y":1,"text":"evil"}`))
	r.Header.Set("Origin", "https://elsewhere.example")
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin write accepted")
	}
	w = request(a, "", "", "/", nil)
	good(t, w)
	if len(w.Result().Cookies()) != 1 || !w.Result().Cookies()[0].HttpOnly {
		t.Fatal("missing participant cookie")
	}
	if !strings.Contains(w.Body.String(), "textContent=text") {
		t.Fatal("UI must render untrusted text as text")
	}
}

func TestCorruptStateIsPreserved(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "boards"), 0700); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "boards", "broken.json")
	os.WriteFile(p, []byte("corrupt"), 0600)
	a := &app{dir: dir, boards: map[string]*Board{}}
	a.mu.Lock()
	_, err := a.loadLocked("broken")
	a.mu.Unlock()
	if err == nil {
		t.Fatal("corruption ignored")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "corrupt" {
		t.Fatal("corrupt state overwritten")
	}
}

func TestExportsAreValid(t *testing.T) {
	a := fresh(t)
	facDemo := create(t, a, "demo")
	codeDemo := invite(t, a, facDemo, "demo")
	memberDemo := token()
	join(t, a, memberDemo, "demo", codeDemo)
	stroke(t, a, memberDemo, "", "demo")
	note(t, a, memberDemo, "demo", "demo note")
	w := request(a, memberDemo, "", "/api/export?board=demo&format=json", nil)
	good(t, w)
	var v struct {
		Strokes []any `json:"strokes"`
		Notes   []any `json:"notes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || len(v.Strokes) != 1 || len(v.Notes) != 1 {
		t.Fatalf("bad JSON export: %s", w.Body.String())
	}
	if request(a, token(), "", "/api/export?board=demo&format=json", nil).Code != 403 {
		t.Fatal("nonmember exported a private board")
	}
	p := request(a, memberDemo, "", "/api/export?board=demo&format=png", nil)
	if p.Code != 200 || p.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("bad PNG export: %d", p.Code)
	}
	img, err := png.Decode(bytes.NewReader(p.Body.Bytes()))
	if err != nil {
		t.Fatalf("PNG does not decode: %v", err)
	}
	if img.Bounds().Dx() != int(canvasW) || img.Bounds().Dy() != int(canvasH) {
		t.Fatalf("PNG has wrong size: %v", img.Bounds())
	}
}

func TestUnknownBoardReadsDoNotReserve(t *testing.T) {
	a := fresh(t)
	// A guest read of an unknown board must not mint it: state, export,
	// and non-create actions return 404 without touching disk.
	if w := request(a, token(), "", "/api/state?board=chosen", nil); w.Code != 404 {
		t.Fatalf("state reserved unknown board: %d %s", w.Code, w.Body.String())
	}
	if w := request(a, token(), "", "/api/export?board=chosen&format=json", nil); w.Code != 404 {
		t.Fatalf("export reserved unknown board: %d %s", w.Code, w.Body.String())
	}
	if w := request(a, token(), "", "/api/action", map[string]any{
		"action": "stroke", "board": "chosen",
		"points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "#000000", "width": 2,
	}); w.Code != 404 {
		t.Fatalf("action reserved unknown board: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(a.dir, "boards", "chosen.json")); !os.IsNotExist(err) {
		t.Fatal("guest read persisted a board file")
	}
	a.mu.Lock()
	_, reserved := a.boards["chosen"]
	a.mu.Unlock()
	if reserved {
		t.Fatal("guest read cached a board")
	}
	// The facilitator can still create the name and receives the only
	// copy of the facilitator capability. Named boards are private: a
	// stranger is denied while an invited member reads fine.
	fac := create(t, a, "chosen")
	if request(a, token(), "", "/api/state?board=chosen", nil).Code != 403 {
		t.Fatal("nonmember read a private board")
	}
	code := invite(t, a, fac, "chosen")
	member := token()
	join(t, a, member, "chosen", code)
	good(t, request(a, member, "", "/api/state?board=chosen", nil))
	w := request(a, member, "", "/api/export?board=chosen&format=json", nil)
	good(t, w)
	for _, leak := range []string{fac, "facilitatorKey"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Fatalf("export leaked facilitator capability: %q", leak)
		}
	}
}

func TestPrefixRouting(t *testing.T) {
	a := fresh(t)
	a.prefix = "/wb"
	if request(a, "", "", "/api/state?board=main", nil).Code != 404 {
		t.Fatal("unprefixed API reachable on a prefixed server")
	}
	good(t, request(a, "", "", "/wb/", nil))
	// Reads mint nothing: the default board answers only once created,
	// mirroring server startup which ensures "main" before serving.
	if request(a, token(), "", "/wb/api/state?board=main", nil).Code != 404 {
		t.Fatal("unknown board read did not return 404")
	}
	w := request(a, token(), "", "/wb/api/action", map[string]any{"action": "create", "board": "main"})
	good(t, w)
	good(t, request(a, token(), "", "/wb/api/state?board=main", nil))
	w2 := request(a, token(), "", "/wb/api/action", map[string]any{"action": "create", "board": "prefixed"})
	good(t, w2)
	var created struct {
		FacilitatorKey string `json:"facilitatorKey"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	// Prefixed private boards enforce membership too: a stranger is denied
	// on state, action, and export until invited.
	stranger := token()
	if request(a, stranger, "", "/wb/api/state?board=prefixed", nil).Code != 403 {
		t.Fatal("prefixed nonmember read state")
	}
	if request(a, stranger, "", "/wb/api/export?board=prefixed&format=json", nil).Code != 403 {
		t.Fatal("prefixed nonmember read export")
	}
	if request(a, stranger, "", "/wb/api/action", map[string]any{
		"action": "note", "board": "prefixed", "x": 10, "y": 10, "text": "hi",
	}).Code != 403 {
		t.Fatal("prefixed nonmember wrote")
	}
	iw := request(a, token(), created.FacilitatorKey, "/wb/api/action", map[string]any{"action": "invite", "board": "prefixed"})
	good(t, iw)
	var iv struct {
		InviteCode string `json:"inviteCode"`
	}
	if err := json.Unmarshal(iw.Body.Bytes(), &iv); err != nil {
		t.Fatal(err)
	}
	good(t, request(a, stranger, "", "/wb/api/action", map[string]any{"action": "join", "board": "prefixed", "invite": iv.InviteCode}))
	good(t, request(a, stranger, "", "/wb/api/state?board=prefixed", nil))
	good(t, request(a, stranger, "", "/wb/api/export?board=prefixed&format=json", nil))
}

func TestInvitesAndRevocation(t *testing.T) {
	a := fresh(t)
	fac := create(t, a, "private1")
	alice, bob, stranger := token(), token(), token()
	// Guessed URLs are denied on state, action, and both export formats.
	if request(a, alice, "", "/api/state?board=private1", nil).Code != 403 {
		t.Fatal("nonmember read state")
	}
	if request(a, alice, "", "/api/action", map[string]any{
		"action": "stroke", "board": "private1",
		"points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "#000000", "width": 2,
	}).Code != 403 {
		t.Fatal("nonmember stroke accepted")
	}
	if request(a, alice, "", "/api/export?board=private1&format=json", nil).Code != 403 {
		t.Fatal("nonmember exported json")
	}
	if request(a, alice, "", "/api/export?board=private1&format=png", nil).Code != 403 {
		t.Fatal("nonmember exported png")
	}
	// Bad invite codes are refused.
	if request(a, alice, "", "/api/action", map[string]any{"action": "join", "board": "private1", "invite": token()}).Code != 403 {
		t.Fatal("bad invite accepted")
	}
	// Owner mints an invite; invited users can draw, note, and sync.
	code := invite(t, a, fac, "private1")
	join(t, a, alice, "private1", code)
	if request(a, bob, "", "/api/action", map[string]any{"action": "join", "board": "private1", "invite": code}).Code != 403 {
		t.Fatal("used invite admitted a second participant")
	}
	join(t, a, bob, "private1", invite(t, a, fac, "private1"))
	pendingCode := invite(t, a, fac, "private1")
	stroke(t, a, alice, "", "private1")
	note(t, a, bob, "private1", "hello")
	good(t, request(a, alice, "", "/api/state?board=private1", nil))
	good(t, request(a, bob, "", "/api/export?board=private1&format=json", nil))
	good(t, request(a, alice, "", "/api/export?board=private1&format=png", nil))
	if request(a, stranger, "", "/api/state?board=private1", nil).Code != 403 {
		t.Fatal("stranger read after invites")
	}
	// Facilitator state carries the roster; member state does not leak it.
	fw := request(a, token(), fac, "/api/state?board=private1", nil)
	good(t, fw)
	if !strings.Contains(fw.Body.String(), alice) || !strings.Contains(fw.Body.String(), pendingCode) {
		t.Fatal("facilitator roster missing member or invite")
	}
	mw := request(a, alice, "", "/api/state?board=private1", nil)
	good(t, mw)
	for _, leak := range []string{fac, code, "facilitatorKey", "inviteCode"} {
		if strings.Contains(mw.Body.String(), leak) {
			t.Fatalf("member state leaked %q", leak)
		}
	}
	// Revoked user loses reads and writes immediately.
	good(t, request(a, token(), fac, "/api/action", map[string]any{"action": "revoke", "board": "private1", "id": alice}))
	if request(a, alice, "", "/api/state?board=private1", nil).Code != 403 {
		t.Fatal("revoked user still reads state")
	}
	if request(a, alice, "", "/api/export?board=private1&format=json", nil).Code != 403 {
		t.Fatal("revoked user still exports")
	}
	if request(a, alice, "", "/api/action", map[string]any{
		"action": "note", "board": "private1", "x": 10, "y": 10, "text": "back",
	}).Code != 403 {
		t.Fatal("revoked user still writes")
	}
	// The other member keeps working; only the facilitator can revoke.
	note(t, a, bob, "private1", "still here")
	if request(a, bob, "", "/api/action", map[string]any{"action": "revoke", "board": "private1", "id": bob}).Code != 403 {
		t.Fatal("participant revoked a member")
	}
	// Revoked invites stop working for new joiners.
	good(t, request(a, token(), fac, "/api/action", map[string]any{"action": "revoke", "board": "private1", "invite": pendingCode}))
	if request(a, stranger, "", "/api/action", map[string]any{"action": "join", "board": "private1", "invite": pendingCode}).Code != 403 {
		t.Fatal("revoked invite still joins")
	}
	// Membership survives a restart.
	dir := a.dir
	b := &app{dir: dir, boards: map[string]*Board{}}
	good(t, request(b, bob, "", "/api/state?board=private1", nil))
	if request(b, alice, "", "/api/state?board=private1", nil).Code != 403 {
		t.Fatal("revocation lost across restart")
	}
}

func TestMainBoardStaysOpen(t *testing.T) {
	a := fresh(t)
	facMain := create(t, a, "main")
	// The legacy default board needs no invite: anyone reads and writes.
	good(t, request(a, token(), "", "/api/state?board=main", nil))
	good(t, request(a, token(), "", "/api/export?board=main&format=json", nil))
	w := request(a, token(), "", "/api/action", map[string]any{
		"action": "stroke", "board": "main",
		"points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "#000000", "width": 2,
	})
	good(t, w)
	if request(a, token(), facMain, "/api/action", map[string]any{"action": "invite", "board": "main"}).Code != 400 {
		t.Fatal("main board should not mint invites")
	}
}

func TestLegacyBoardFilePreserved(t *testing.T) {
	dir := t.TempDir()
	legacy := Board{Version: 1, Name: "legacy", Facilitator: token(), Strokes: []Stroke{{ID: token(), Points: []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, Color: "#000000", Width: 2, Owner: token()}}, Notes: []Note{{ID: token(), X: 5, Y: 5, Text: "old", Color: "#fff3bf", Owner: token()}}}
	if err := saveBoard(boardPath(dir, "legacy"), legacy); err != nil {
		t.Fatal(err)
	}
	a := &app{dir: dir, boards: map[string]*Board{}}
	good(t, request(a, token(), legacy.Facilitator, "/api/state?board=legacy", nil))
	got := a.boards["legacy"]
	if len(got.Strokes) != 1 || len(got.Notes) != 1 || got.Facilitator != legacy.Facilitator {
		t.Fatal("legacy board data lost")
	}
}
