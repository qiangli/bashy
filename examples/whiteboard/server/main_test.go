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
	sid := stroke(t, a, alice, "", "team1")
	nid := note(t, a, alice, "team1", "SECRET <script>alert(1)</script>")
	// State and exports never carry capabilities or owner tokens.
	for _, w := range []*httptest.ResponseRecorder{
		request(a, bob, "", "/api/state?board=team1", nil),
		request(a, bob, "", "/api/export?board=team1&format=json", nil),
	} {
		good(t, w)
		for _, leak := range []string{fac, alice, "owner", "facilitatorKey"} {
			if strings.Contains(w.Body.String(), leak) {
				t.Fatalf("capability or owner leaked: %q", leak)
			}
		}
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
	stroke(t, a, token(), "", "retro")
	note(t, a, token(), "retro", "keep me")
	good(t, request(a, token(), fac, "/api/action", map[string]any{"action": "lock", "board": "retro", "locked": true}))
	before := a.boards["retro"].Revision
	b := &app{dir: dir, boards: map[string]*Board{}}
	good(t, request(b, token(), "", "/api/state?board=retro", nil))
	got := b.boards["retro"]
	if len(got.Strokes) != 1 || len(got.Notes) != 1 || !got.Locked || got.Revision != before || got.Facilitator != fac {
		t.Fatalf("restart lost state: %+v", got)
	}
}

func TestConcurrentDraw(t *testing.T) {
	a := fresh(t)
	create(t, a, "jam")
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			who := token()
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
	good(t, request(b, token(), "", "/api/state?board=jam", nil))
	if len(b.boards["jam"].Strokes) != 30 || len(b.boards["jam"].Notes) != 30 {
		t.Fatal("disk state differs after concurrent draw")
	}
}

func TestWriteFailureIsNotAcknowledged(t *testing.T) {
	dir := t.TempDir()
	a := &app{dir: dir, boards: map[string]*Board{}}
	create(t, a, "fragile")
	rev := a.boards["fragile"].Revision
	boardsDir := filepath.Join(dir, "boards")
	if err := os.Chmod(boardsDir, 0500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(boardsDir, 0700)
	w := request(a, token(), "", "/api/action", map[string]any{
		"action": "stroke", "board": "fragile",
		"points": []Pt{{X: 1, Y: 1}, {X: 2, Y: 2}}, "color": "#000000", "width": 2,
	})
	if w.Code != 500 || len(a.boards["fragile"].Strokes) != 0 || a.boards["fragile"].Revision != rev {
		t.Fatalf("failed save changed state: %d rev %d", w.Code, a.boards["fragile"].Revision)
	}
}

func TestBoundsAndAccess(t *testing.T) {
	a := fresh(t)
	create(t, a, "bounds")
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
		if w := request(a, token(), "", "/api/action", body); w.Code == 200 {
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
	create(t, a, "demo")
	stroke(t, a, token(), "", "demo")
	note(t, a, token(), "demo", "demo note")
	w := request(a, token(), "", "/api/export?board=demo&format=json", nil)
	good(t, w)
	var v struct {
		Strokes []any `json:"strokes"`
		Notes   []any `json:"notes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || len(v.Strokes) != 1 || len(v.Notes) != 1 {
		t.Fatalf("bad JSON export: %s", w.Body.String())
	}
	p := request(a, token(), "", "/api/export?board=demo&format=png", nil)
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

func TestPrefixRouting(t *testing.T) {
	a := fresh(t)
	a.prefix = "/wb"
	if request(a, "", "", "/api/state?board=main", nil).Code != 404 {
		t.Fatal("unprefixed API reachable on a prefixed server")
	}
	good(t, request(a, "", "", "/wb/", nil))
	good(t, request(a, token(), "", "/wb/api/state?board=main", nil))
	good(t, request(a, token(), "", "/wb/api/action", map[string]any{"action": "create", "board": "prefixed"}))
}
