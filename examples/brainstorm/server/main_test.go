package main

import (
	"bytes"
	"encoding/json"
	"fmt"
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
		r.AddCookie(&http.Cookie{Name: "brainstorm-participant", Value: who})
	}
	if key != "" {
		r.Header.Set("X-Teacher-Key", key)
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	return w
}
func fresh(t *testing.T) *app {
	t.Helper()
	a, e := load(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func good(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}
func phase(t *testing.T, a *app, name string) {
	t.Helper()
	good(t, request(a, token(), a.board.Teacher, "/api/action", map[string]any{"action": "phase", "phase": name}))
}
func add(t *testing.T, a *app, who, text string) string {
	t.Helper()
	good(t, request(a, who, "", "/api/action", map[string]any{"action": "add", "text": text, "column": 0}))
	return a.board.Cards[len(a.board.Cards)-1].ID
}

func TestClassroomFlowPrivacyAndOwnership(t *testing.T) {
	a := fresh(t)
	alice, bob := token(), token()
	id := add(t, a, alice, "PRIVATE IDEA <script>alert(1)</script>")
	other := request(a, bob, "", "/api/state", nil)
	good(t, other)
	if strings.Contains(other.Body.String(), "PRIVATE IDEA") {
		t.Fatal("private idea leaked")
	}
	if !strings.Contains(request(a, alice, "", "/api/state", nil).Body.String(), "PRIVATE IDEA") {
		t.Fatal("owner cannot read own card")
	}
	if strings.Contains(other.Body.String(), a.board.Teacher) || strings.Contains(other.Body.String(), alice) {
		t.Fatal("capability leaked")
	}
	for _, op := range []string{"edit", "delete"} {
		w := request(a, bob, "", "/api/action", map[string]any{"action": op, "id": id, "text": "changed"})
		if w.Code != 403 {
			t.Fatalf("ownership bypass: %s %d", op, w.Code)
		}
	}
	if request(a, bob, "", "/api/action", map[string]any{"action": "phase", "phase": "discuss"}).Code != 403 {
		t.Fatal("student changed phase")
	}
	if request(a, alice, a.board.Teacher, "/api/export", nil).Code != 409 {
		t.Fatal("export exposed private writing")
	}
	phase(t, a, "discuss")
	if !strings.Contains(request(a, bob, "", "/api/state", nil).Body.String(), "PRIVATE IDEA") {
		t.Fatal("reveal failed")
	}
	if request(a, alice, "", "/api/action", map[string]any{"action": "edit", "id": id, "text": "late"}).Code != 409 {
		t.Fatal("late edit allowed")
	}
	if request(a, alice, "", "/api/export", nil).Code != 403 {
		t.Fatal("student exported")
	}
	w := request(a, alice, a.board.Teacher, "/api/export", nil)
	good(t, w)
	if strings.Contains(w.Body.String(), a.board.Teacher) || strings.Contains(w.Body.String(), alice) || strings.Contains(w.Body.String(), "\"owner\"") {
		t.Fatal("export leaked capability")
	}
	good(t, request(a, alice, a.board.Teacher, "/api/export?format=markdown", nil))
	if request(a, alice, a.board.Teacher, "/api/action", map[string]any{"action": "phase", "phase": "write"}).Code != 409 {
		t.Fatal("phase regressed")
	}
}
func TestVotingAndPersistence(t *testing.T) {
	a := fresh(t)
	who := token()
	ids := []string{}
	for i := 0; i < 4; i++ {
		ids = append(ids, add(t, a, who, fmt.Sprintf("idea %d", i)))
	}
	phase(t, a, "discuss")
	phase(t, a, "vote")
	for _, id := range ids[:3] {
		good(t, request(a, who, "", "/api/action", map[string]any{"action": "vote", "id": id}))
	}
	if request(a, who, "", "/api/action", map[string]any{"action": "vote", "id": ids[3]}).Code != 409 {
		t.Fatal("fourth vote accepted")
	}
	good(t, request(a, who, "", "/api/action", map[string]any{"action": "vote", "id": ids[0]}))
	good(t, request(a, who, "", "/api/action", map[string]any{"action": "vote", "id": ids[3]}))
	b, e := load(filepath.Dir(a.path))
	if e != nil {
		t.Fatal(e)
	}
	if len(b.board.Cards) != 4 || b.board.Phase != "vote" || b.board.Teacher != a.board.Teacher || !b.board.Cards[3].Votes[who] {
		t.Fatal("restart lost state")
	}
	phase(t, b, "closed")
	if request(b, who, "", "/api/action", map[string]any{"action": "vote", "id": ids[1]}).Code != 409 {
		t.Fatal("closed vote accepted")
	}
}
func TestThirtyConcurrentParticipants(t *testing.T) {
	a := fresh(t)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			who := token()
			w := request(a, who, "", "/api/action", map[string]any{"action": "add", "text": fmt.Sprintf("student-%d", i), "column": i % 5})
			if w.Code != 200 {
				t.Errorf("student %d: %d", i, w.Code)
			}
			var state struct {
				Cards []any `json:"cards"`
			}
			json.Unmarshal(request(a, who, "", "/api/state", nil).Body.Bytes(), &state)
			if len(state.Cards) != 1 {
				t.Errorf("student %d saw %d private cards", i, len(state.Cards))
			}
		}(i)
	}
	wg.Wait()
	if len(a.board.Cards) != 30 {
		t.Fatalf("lost writes: %d", len(a.board.Cards))
	}
	b, e := load(filepath.Dir(a.path))
	if e != nil || len(b.board.Cards) != 30 {
		t.Fatal("disk state differs")
	}
}
func TestSaveFailureIsNotAcknowledged(t *testing.T) {
	a := fresh(t)
	a.path = filepath.Join(t.TempDir(), "missing", "board.json")
	w := request(a, token(), "", "/api/action", map[string]any{"action": "add", "text": "keep draft"})
	if w.Code != 500 || len(a.board.Cards) != 0 || a.board.Revision != 0 {
		t.Fatal("failed save changed state")
	}
}
func TestInvalidInputAndCrossOrigin(t *testing.T) {
	a := fresh(t)
	for _, body := range []map[string]any{{"action": "add", "text": ""}, {"action": "add", "text": strings.Repeat("x", 1001)}, {"action": "add", "text": "valid", "column": 5}, {"action": "add", "text": "valid", "name": strings.Repeat("x", 41)}} {
		if request(a, token(), "", "/api/action", body).Code != 400 {
			t.Fatal("invalid input accepted")
		}
	}
	r := httptest.NewRequest("POST", "http://board/api/action", strings.NewReader(`{"action":"add","text":"evil"}`))
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
	p := filepath.Join(dir, "board.json")
	os.WriteFile(p, []byte("corrupt"), 0600)
	if _, e := load(dir); e == nil {
		t.Fatal("corruption ignored")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "corrupt" {
		t.Fatal("corrupt state overwritten")
	}
}
