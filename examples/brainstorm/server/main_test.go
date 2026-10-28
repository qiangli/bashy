package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSameOriginThroughLoopbackProxy(t *testing.T) {
	r := httptest.NewRequest("POST", "http://127.0.0.1:18191/api/action", nil)
	r.RemoteAddr = "127.0.0.1:34567"
	r.Header.Set("X-Forwarded-Host", "example.test")
	r.Header.Set("X-Forwarded-Proto", "https")
	for _, tc := range []struct {
		origin string
		remote string
		want   bool
	}{
		{"http://127.0.0.1:18191", "127.0.0.1:34567", true},
		{"https://example.test", "127.0.0.1:34567", true},
		{"https://other.test", "127.0.0.1:34567", false},
		{"http://example.test", "127.0.0.1:34567", false},
		{"https://example.test", "192.0.2.1:34567", false},
	} {
		r.RemoteAddr = tc.remote
		u, err := url.Parse(tc.origin)
		if err != nil {
			t.Fatal(err)
		}
		if got := sameOrigin(r, u); got != tc.want {
			t.Errorf("origin %s remote %s: got %v, want %v", tc.origin, tc.remote, got, tc.want)
		}
	}
}

func request(a *app, who, key, session, path string, body any) *httptest.ResponseRecorder {
	method := "GET"
	var b bytes.Buffer
	if body != nil {
		method = "POST"
		if m, ok := body.(map[string]any); ok {
			if _, has := m["session"]; !has {
				m["session"] = session
			}
		}
		json.NewEncoder(&b).Encode(body)
	}
	if strings.Contains(path, "?") {
		path += "&session=" + session
	} else {
		path += "?session=" + session
	}
	r := httptest.NewRequest(method, path, &b)
	if who != "" {
		r.AddCookie(&http.Cookie{Name: "brainstorm-participant", Value: who})
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
	a, e := loadForTest(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return a
}
func loadForTest(dir string) (*app, error) {
	a := &app{dir: dir, sessions: map[string]*Session{}}
	a.mu.Lock()
	defer a.mu.Unlock()
	if _, err := a.ensureMainLocked(); err != nil {
		return nil, err
	}
	return a, nil
}
func good(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
}
func phase(t *testing.T, a *app, session, name string) {
	t.Helper()
	s, err := func() (*Session, error) { a.mu.Lock(); defer a.mu.Unlock(); return a.loadLocked(session) }()
	if err != nil {
		t.Fatal(err)
	}
	good(t, request(a, token(), s.Facilitator, session, "/api/action", map[string]any{"action": "phase", "phase": name}))
}
func add(t *testing.T, a *app, session, who, text string) string {
	t.Helper()
	good(t, request(a, who, "", session, "/api/action", map[string]any{"action": "add", "text": text, "column": 0}))
	a.mu.Lock()
	defer a.mu.Unlock()
	s, _ := a.loadLocked(session)
	return s.Cards[len(s.Cards)-1].ID
}
func create(t *testing.T, a *app, name, title string, budget int) string {
	t.Helper()
	body := map[string]any{"action": "create", "session": name, "voteBudget": budget}
	if title != "" {
		body["title"] = title
	}
	w := request(a, token(), "", "main", "/api/action", body)
	good(t, w)
	var v struct {
		FacilitatorKey string `json:"facilitatorKey"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil || v.FacilitatorKey == "" {
		t.Fatalf("create did not reveal facilitator key: %s", w.Body.String())
	}
	return v.FacilitatorKey
}

func TestSessionFlowPrivacyAndOwnership(t *testing.T) {
	a := fresh(t)
	alice, bob := token(), token()
	id := add(t, a, "main", alice, "PRIVATE IDEA <script>alert(1)</script>")
	other := request(a, bob, "", "main", "/api/state", nil)
	good(t, other)
	if strings.Contains(other.Body.String(), "PRIVATE IDEA") {
		t.Fatal("private idea leaked")
	}
	if !strings.Contains(request(a, alice, "", "main", "/api/state", nil).Body.String(), "PRIVATE IDEA") {
		t.Fatal("owner cannot read own card")
	}
	a.mu.Lock()
	key := a.sessions["main"].Facilitator
	a.mu.Unlock()
	if strings.Contains(other.Body.String(), key) || strings.Contains(other.Body.String(), alice) {
		t.Fatal("capability leaked")
	}
	for _, op := range []string{"edit", "delete"} {
		w := request(a, bob, "", "main", "/api/action", map[string]any{"action": op, "id": id, "text": "changed"})
		if w.Code != 403 {
			t.Fatalf("ownership bypass: %s %d", op, w.Code)
		}
	}
	if request(a, bob, "", "main", "/api/action", map[string]any{"action": "phase", "phase": "discuss"}).Code != 403 {
		t.Fatal("participant changed phase")
	}
	if request(a, alice, key, "main", "/api/export", nil).Code != 409 {
		t.Fatal("export exposed private writing")
	}
	phase(t, a, "main", "discuss")
	if !strings.Contains(request(a, bob, "", "main", "/api/state", nil).Body.String(), "PRIVATE IDEA") {
		t.Fatal("reveal failed")
	}
	if request(a, alice, "", "main", "/api/action", map[string]any{"action": "edit", "id": id, "text": "late"}).Code != 409 {
		t.Fatal("late edit allowed")
	}
	if request(a, alice, "", "main", "/api/export", nil).Code != 403 {
		t.Fatal("participant exported")
	}
	w := request(a, alice, key, "main", "/api/export", nil)
	good(t, w)
	if strings.Contains(w.Body.String(), key) || strings.Contains(w.Body.String(), alice) || strings.Contains(w.Body.String(), "\"owner\"") {
		t.Fatal("export leaked capability")
	}
	good(t, request(a, alice, key, "main", "/api/export?format=markdown", nil))
	if request(a, alice, key, "main", "/api/action", map[string]any{"action": "phase", "phase": "write"}).Code != 409 {
		t.Fatal("phase regressed")
	}
}

func TestCreateSessionConfigAndIsolation(t *testing.T) {
	a := fresh(t)
	key := create(t, a, "retro", "October Retro", 5)
	if request(a, token(), "", "main", "/api/action", map[string]any{"action": "create", "session": "retro"}).Code != 409 {
		t.Fatal("duplicate session accepted")
	}
	if request(a, token(), "", "main", "/api/action", map[string]any{"action": "create", "session": ""}).Code != 409 {
		t.Fatal("empty session name did not default to existing main")
	}
	for _, bad := range []string{"has space", "../x", strings.Repeat("x", 65)} {
		if request(a, token(), "", "main", "/api/action", map[string]any{"action": "create", "session": bad}).Code != 400 {
			t.Fatalf("bad session name accepted: %q", bad)
		}
	}
	if request(a, token(), "", "main", "/api/action", map[string]any{"action": "create", "session": "big", "voteBudget": 11}).Code != 400 {
		t.Fatal("vote budget 11 accepted")
	}
	if request(a, token(), "", "main", "/api/action", map[string]any{"action": "create", "session": "wide", "columns": []string{}}).Code != 400 {
		t.Fatal("empty columns accepted")
	}
	who := token()
	id := add(t, a, "retro", who, "retro idea")
	other := request(a, token(), "", "main", "/api/state", nil)
	good(t, other)
	if strings.Contains(other.Body.String(), "retro idea") {
		t.Fatal("idea leaked across sessions")
	}
	st := request(a, who, key, "retro", "/api/state", nil)
	good(t, st)
	var v struct {
		Title      string   `json:"title"`
		Columns    []string `json:"columns"`
		VoteBudget int      `json:"voteBudget"`
	}
	if err := json.Unmarshal(st.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Title != "October Retro" || v.VoteBudget != 5 {
		t.Fatalf("session config lost: %+v", v)
	}
	for i := 0; i < 5; i++ {
		add(t, a, "retro", who, fmt.Sprintf("option %d", i))
	}
	phase(t, a, "retro", "discuss")
	phase(t, a, "retro", "vote")
	a.mu.Lock()
	s := a.sessions["retro"]
	ids := make([]string, len(s.Cards))
	for i, c := range s.Cards {
		ids[i] = c.ID
	}
	a.mu.Unlock()
	_ = id
	for _, cid := range ids[:5] {
		good(t, request(a, who, "", "retro", "/api/action", map[string]any{"action": "vote", "id": cid}))
	}
	if request(a, who, "", "retro", "/api/action", map[string]any{"action": "vote", "id": ids[5]}).Code != 409 {
		t.Fatal("vote beyond budget accepted")
	}
	left := request(a, who, "", "retro", "/api/state", nil)
	if !strings.Contains(left.Body.String(), `"votesLeft":0`) {
		t.Fatalf("votesLeft wrong: %s", left.Body.String())
	}
}

func TestCustomColumnsAndPrompts(t *testing.T) {
	a := fresh(t)
	w := request(a, token(), "", "main", "/api/action", map[string]any{
		"action": "create", "session": "plan", "title": "Sprint Plan",
		"columns": []string{"Keep", "Drop"}, "prompts": []string{"What works?", "What slows us?"},
		"voteBudget": 2,
	})
	good(t, w)
	var v struct {
		Columns []string `json:"columns"`
		Prompts []string `json:"prompts"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.Columns) != 2 || v.Prompts[1] != "What slows us?" {
		t.Fatalf("custom columns/prompts lost: %+v", v)
	}
	who := token()
	if request(a, who, "", "plan", "/api/action", map[string]any{"action": "add", "text": "x", "column": 2}).Code != 400 {
		t.Fatal("out-of-range column accepted for custom columns")
	}
	good(t, request(a, who, "", "plan", "/api/action", map[string]any{"action": "add", "text": "keep shipping", "column": 0}))
	if request(a, token(), "", "main", "/api/action", map[string]any{
		"action": "create", "session": "mismatch", "columns": []string{"A", "B"}, "prompts": []string{"only one"},
	}).Code != 400 {
		t.Fatal("mismatched prompts accepted")
	}
}

func TestLegacyClassroomBoardCompat(t *testing.T) {
	dir := t.TempDir()
	teacher := token()
	owner := token()
	legacy := fmt.Sprintf(`{"version":1,"phase":"write","teacher":%q,"cards":[{"id":"c1","column":0,"text":"legacy private","name":"Sam","owner":%q,"votes":{}}],"revision":4}`, teacher, owner)
	before := filepath.Join(dir, "board.json")
	if err := os.WriteFile(before, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := loadForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(before); string(got) != legacy {
		t.Fatal("legacy board.json was modified")
	}
	raw, _ := os.ReadFile(sessionPath(dir, "main"))
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if s.Facilitator != teacher || s.Phase != "write" || len(s.Cards) != 1 || s.VoteBudget != 3 || s.Title == "" || len(s.Columns) != 5 {
		t.Fatalf("legacy board not honored: %+v", s)
	}
	other := request(a, token(), "", "main", "/api/state", nil)
	good(t, other)
	if strings.Contains(other.Body.String(), "legacy private") {
		t.Fatal("legacy private idea leaked")
	}
	if !strings.Contains(request(a, owner, "", "main", "/api/state", nil).Body.String(), "legacy private") {
		t.Fatal("legacy owner cannot read own card")
	}
	// The old teacher header still authorizes facilitator actions.
	r := httptest.NewRequest("GET", "/api/state?session=main", nil)
	r.Header.Set("X-Teacher-Key", teacher)
	rec := httptest.NewRecorder()
	a.ServeHTTP(rec, r)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"facilitator":true`) {
		t.Fatalf("legacy teacher key rejected: %d %s", rec.Code, rec.Body.String())
	}
	good(t, request(a, token(), teacher, "main", "/api/action", map[string]any{"action": "phase", "phase": "discuss"}))
	if !strings.Contains(request(a, token(), "", "main", "/api/state", nil).Body.String(), "legacy private") {
		t.Fatal("legacy reveal failed")
	}
}

func TestVotingAndPersistence(t *testing.T) {
	a := fresh(t)
	who := token()
	ids := []string{}
	for i := 0; i < 4; i++ {
		ids = append(ids, add(t, a, "main", who, fmt.Sprintf("idea %d", i)))
	}
	phase(t, a, "main", "discuss")
	phase(t, a, "main", "vote")
	for _, id := range ids[:3] {
		good(t, request(a, who, "", "main", "/api/action", map[string]any{"action": "vote", "id": id}))
	}
	if request(a, who, "", "main", "/api/action", map[string]any{"action": "vote", "id": ids[3]}).Code != 409 {
		t.Fatal("fourth vote accepted")
	}
	good(t, request(a, who, "", "main", "/api/action", map[string]any{"action": "vote", "id": ids[0]}))
	good(t, request(a, who, "", "main", "/api/action", map[string]any{"action": "vote", "id": ids[3]}))
	c, e2 := loadSessionFromDir(a.dir, "main")
	if e2 != nil {
		t.Fatal(e2)
	}
	a.mu.Lock()
	teacher := a.sessions["main"].Facilitator
	a.mu.Unlock()
	if len(c.Cards) != 4 || c.Phase != "vote" || c.Facilitator != teacher || !c.Cards[3].Votes[who] {
		t.Fatal("restart lost state")
	}
	phase(t, a, "main", "closed")
	if request(a, who, "", "main", "/api/action", map[string]any{"action": "vote", "id": ids[1]}).Code != 409 {
		t.Fatal("closed vote accepted")
	}
}
func loadSessionFromDir(dir, name string) (*Session, error) {
	raw, err := os.ReadFile(sessionPath(dir, name))
	if err != nil {
		return nil, err
	}
	var s Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func TestThirtyConcurrentParticipants(t *testing.T) {
	a := fresh(t)
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			who := token()
			w := request(a, who, "", "main", "/api/action", map[string]any{"action": "add", "text": fmt.Sprintf("student-%d", i), "column": i % 5})
			if w.Code != 200 {
				t.Errorf("student %d: %d", i, w.Code)
			}
			var state struct {
				Cards []any `json:"cards"`
			}
			json.Unmarshal(request(a, who, "", "main", "/api/state", nil).Body.Bytes(), &state)
			if len(state.Cards) != 1 {
				t.Errorf("student %d saw %d private cards", i, len(state.Cards))
			}
		}(i)
	}
	wg.Wait()
	a.mu.Lock()
	n := len(a.sessions["main"].Cards)
	a.mu.Unlock()
	if n != 30 {
		t.Fatalf("lost writes: %d", n)
	}
	c, e := loadSessionFromDir(a.dir, "main")
	if e != nil || len(c.Cards) != 30 {
		t.Fatal("disk state differs")
	}
}
func TestSaveFailureIsNotAcknowledged(t *testing.T) {
	a := fresh(t)
	key := create(t, a, "doomed", "", 0)
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatal(err)
	}
	a.dir = blocker
	w := request(a, token(), "", "doomed", "/api/action", map[string]any{"action": "add", "text": "keep draft"})
	if w.Code != 500 {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, _ := a.loadLocked("doomed")
	_ = key
	if len(s.Cards) != 0 || s.Revision != 0 {
		t.Fatal("failed save changed state")
	}
}
func TestInvalidInputAndCrossOrigin(t *testing.T) {
	a := fresh(t)
	for _, body := range []map[string]any{{"action": "add", "text": ""}, {"action": "add", "text": strings.Repeat("x", 1001)}, {"action": "add", "text": "valid", "column": 5}, {"action": "add", "text": "valid", "name": strings.Repeat("x", 41)}} {
		if request(a, token(), "", "main", "/api/action", body).Code != 400 {
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
	w = request(a, "", "", "main", "/", nil)
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
	p := sessionPath(dir, "main")
	os.MkdirAll(filepath.Dir(p), 0700)
	os.WriteFile(p, []byte("corrupt"), 0600)
	if _, e := loadForTest(dir); e == nil {
		t.Fatal("corruption ignored")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "corrupt" {
		t.Fatal("corrupt state overwritten")
	}
}
func TestUnknownSessionReadsDoNotReserve(t *testing.T) {
	a := fresh(t)
	// A guest read of an unknown session must not mint it: state, export,
	// and non-create actions return 404 without touching disk.
	if w := request(a, token(), "", "chosen", "/api/state", nil); w.Code != 404 {
		t.Fatalf("state reserved unknown session: %d %s", w.Code, w.Body.String())
	}
	if w := request(a, token(), "", "chosen", "/api/export", nil); w.Code != 404 {
		t.Fatalf("export reserved unknown session: %d %s", w.Code, w.Body.String())
	}
	if w := request(a, token(), "", "chosen", "/api/action", map[string]any{"action": "add", "text": "squat", "column": 0}); w.Code != 404 {
		t.Fatalf("action reserved unknown session: %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(sessionPath(a.dir, "chosen")); !os.IsNotExist(err) {
		t.Fatal("guest read persisted a session file")
	}
	a.mu.Lock()
	_, reserved := a.sessions["chosen"]
	a.mu.Unlock()
	if reserved {
		t.Fatal("guest read cached a session")
	}
	// The facilitator can still create the name and receives the only
	// copy of the facilitator capability.
	fac := create(t, a, "chosen", "Chosen Topic", 2)
	good(t, request(a, token(), "", "chosen", "/api/state", nil))
	if w := request(a, token(), "", "chosen", "/api/export", nil); w.Code != 403 {
		t.Fatalf("participant exported: %d %s", w.Code, w.Body.String())
	}
	phase(t, a, "chosen", "discuss")
	w := request(a, token(), fac, "chosen", "/api/export", nil)
	good(t, w)
	for _, leak := range []string{fac, "facilitatorKey"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Fatalf("export leaked facilitator capability: %q", leak)
		}
	}
}

func TestPrefixRouting(t *testing.T) {
	a := fresh(t)
	a.prefix = "/brainstorm"
	if got := request(a, token(), "", "main", "/api/state", nil); got.Code != 404 {
		t.Fatalf("unprefixed path accepted under prefix: %d", got.Code)
	}
	r := httptest.NewRequest("GET", "/brainstorm/api/state?session=main", nil)
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("prefixed state failed: %d %s", w.Code, w.Body.String())
	}
	// Unknown names stay 404 behind a prefix too; nothing is minted.
	r404 := httptest.NewRequest("GET", "/brainstorm/api/state?session=missing", nil)
	w404 := httptest.NewRecorder()
	a.ServeHTTP(w404, r404)
	if w404.Code != 404 {
		t.Fatalf("prefixed unknown read did not return 404: %d %s", w404.Code, w404.Body.String())
	}
	if a.stripFails() {
		t.Fatal("strip helper broken")
	}
}
func (a *app) stripFails() bool {
	if _, ok := a.stripPrefix("/brainstorm/api/state"); !ok {
		return true
	}
	if p, ok := a.stripPrefix("/brainstorm"); !ok || p != "/" {
		return true
	}
	if _, ok := a.stripPrefix("/other/api/state"); ok {
		return true
	}
	return false
}

func createPrivate(t *testing.T, a *app, name string) string {
	t.Helper()
	w := request(a, token(), "", name, "/api/action", map[string]any{"action": "create", "private": true})
	good(t, w)
	var v struct {
		FacilitatorKey string `json:"facilitatorKey"`
	}
	json.Unmarshal(w.Body.Bytes(), &v)
	return v.FacilitatorKey
}
func invite(t *testing.T, a *app, fac, name, label string) string {
	t.Helper()
	w := request(a, token(), fac, name, "/api/action", map[string]any{"action": "invite", "name": label})
	good(t, w)
	var v struct {
		Code string `json:"inviteCode"`
	}
	json.Unmarshal(w.Body.Bytes(), &v)
	if v.Code == "" {
		t.Fatal("no invite code")
	}
	return v.Code
}
func denied(t *testing.T, w *httptest.ResponseRecorder, what string) {
	t.Helper()
	if w.Code != 404 && w.Code != 403 {
		t.Fatalf("%s not denied: %d %s", what, w.Code, w.Body.String())
	}
}

func TestPrivateSessionInvitesAndRevocation(t *testing.T) {
	dir := t.TempDir()
	a, _ := loadForTest(dir)
	fac := createPrivate(t, a, "club")
	alice, bob, eve := token(), token(), token()
	add404 := map[string]any{"action": "add", "text": "x", "column": 0}

	// Nonmember / guessed URL: every surface denied, indistinguishable from unknown.
	denied(t, request(a, eve, "", "club", "/api/state", nil), "state")
	denied(t, request(a, eve, "", "club", "/api/export", nil), "export")
	denied(t, request(a, eve, "", "club", "/api/action", add404), "write")
	denied(t, request(a, eve, "", "club", "/api/action", map[string]any{"action": "vote", "id": "x"}), "vote")
	denied(t, request(a, eve, "", "club", "/api/action", map[string]any{"action": "join", "invite": token()}), "bad join")
	denied(t, request(a, eve, "", "club", "/api/action", map[string]any{"action": "invite"}), "nonowner invite")
	missing := request(a, eve, "", "nosuch", "/api/state", nil).Body.String()
	if request(a, eve, "", "club", "/api/state", nil).Body.String() != missing {
		t.Fatal("private session distinguishable from unknown")
	}

	// Invited distinct users keep private-writing semantics.
	ca, cb := invite(t, a, fac, "club", "alice"), invite(t, a, fac, "club", "bob")
	good(t, request(a, alice, "", "club", "/api/action", map[string]any{"action": "join", "invite": ca}))
	good(t, request(a, bob, "", "club", "/api/action", map[string]any{"action": "join", "invite": cb}))
	denied(t, request(a, eve, "", "club", "/api/action", map[string]any{"action": "join", "invite": ca}), "reused invite")
	good(t, request(a, alice, "", "club", "/api/action", map[string]any{"action": "add", "text": "ALICE SECRET", "column": 0}))
	if strings.Contains(request(a, bob, "", "club", "/api/state", nil).Body.String(), "ALICE SECRET") {
		t.Fatal("member saw another member's private card")
	}
	if !strings.Contains(request(a, alice, "", "club", "/api/state", nil).Body.String(), "ALICE SECRET") {
		t.Fatal("author lost own card")
	}
	if strings.Contains(request(a, alice, "", "club", "/api/state", nil).Body.String(), "members") {
		t.Fatal("member view lists members")
	}
	fv := request(a, token(), fac, "club", "/api/state", nil).Body.String()
	if !strings.Contains(fv, "alice") || !strings.Contains(fv, handleOf(bob)) {
		t.Fatalf("owner cannot see members: %s", fv)
	}

	// Survives restart.
	a2, err := loadForTest(dir)
	if err != nil {
		t.Fatal(err)
	}
	good(t, request(a2, bob, "", "club", "/api/state", nil))
	denied(t, request(a2, eve, "", "club", "/api/state", nil), "post-restart nonmember")

	// Revoke: immediate denial everywhere, and only the owner may revoke.
	denied(t, request(a2, bob, "", "club", "/api/action", map[string]any{"action": "revoke", "id": handleOf(alice)}), "member revoke")
	good(t, request(a2, token(), fac, "club", "/api/action", map[string]any{"action": "revoke", "id": handleOf(bob)}))
	denied(t, request(a2, bob, "", "club", "/api/state", nil), "revoked state")
	denied(t, request(a2, bob, "", "club", "/api/export", nil), "revoked export")
	denied(t, request(a2, bob, "", "club", "/api/action", add404), "revoked write")
	denied(t, request(a2, bob, "", "club", "/api/action", map[string]any{"action": "vote", "id": "x"}), "revoked vote")
	good(t, request(a2, alice, "", "club", "/api/state", nil))
	// Revoking a pending invite kills it.
	cc := invite(t, a2, fac, "club", "carol")
	good(t, request(a2, token(), fac, "club", "/api/action", map[string]any{"action": "revoke", "id": cc}))
	denied(t, request(a2, eve, "", "club", "/api/action", map[string]any{"action": "join", "invite": cc}), "revoked invite")
	a3, _ := loadForTest(dir)
	denied(t, request(a3, bob, "", "club", "/api/state", nil), "revoked after restart")
}

func TestOpenSessionsAndLegacyStayOpen(t *testing.T) {
	a := fresh(t)
	good(t, request(a, token(), "", "main", "/api/state", nil))
	fac := create(t, a, "pub", "", 3)
	good(t, request(a, token(), "", "pub", "/api/state", nil))
	if w := request(a, token(), fac, "pub", "/api/action", map[string]any{"action": "invite"}); w.Code != 409 {
		t.Fatalf("invite on open session: %d", w.Code)
	}
	good(t, request(a, token(), "", "pub", "/api/action", map[string]any{"action": "join"}))
}

func TestPrivateSessionPrefix(t *testing.T) {
	a := fresh(t)
	fac := createPrivate(t, a, "px")
	a.prefix = "/bs"
	r := httptest.NewRequest("GET", "/bs/api/state?session=px", nil)
	r.AddCookie(&http.Cookie{Name: "brainstorm-participant", Value: token()})
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	denied(t, w, "prefixed nonmember")
	r = httptest.NewRequest("GET", "/bs/api/state?session=px", nil)
	r.Header.Set("X-Facilitator-Key", fac)
	w = httptest.NewRecorder()
	a.ServeHTTP(w, r)
	good(t, w)
}
