package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// TestMain doubles as the fake `bashy`: with FAKE_TUTOR_DIR set it answers
// `tutor status|done|next|extend` from canned JSON and logs every call.
func TestMain(m *testing.M) {
	if d := os.Getenv("FAKE_TUTOR_DIR"); d != "" {
		args := os.Args[1:]
		f, _ := os.OpenFile(filepath.Join(d, "calls.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		fmt.Fprintln(f, strings.Join(args, " "))
		f.Close()
		if len(args) > 1 && args[1] == "status" {
			b, _ := os.ReadFile(filepath.Join(d, "status.json"))
			os.Stdout.Write(b)
		} else {
			fmt.Println(`{"ok":true}`)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

const canned = `{"sprint":7,"title":"Session","clock":{"state":"running","remaining_s":3000},
"current":{"index":2,"total":3,"id":"t2","title":"Build it","owners":["alice"],"timebox_s":600,"elapsed_s":100,"remaining_s":500,"overdue":false},
"steps":[
 {"index":1,"id":"t1","title":"Warm up","owner":"bob","role":"participant","status":"done","timebox_s":300,"overdue":false,"evidence":"https://secret.example/bob"},
 {"index":2,"id":"t2","title":"Build it","owner":"alice","role":"participant","status":"doing","timebox_s":600,"overdue":false,"evidence":null},
 {"index":3,"id":"t3","title":"Share","owner":"bob","role":"participant","status":"todo","timebox_s":300,"overdue":false,"evidence":null}]}`

type env struct {
	s   *server
	ts  *httptest.Server
	dir string
}

func setup(t *testing.T, prefix string) *env {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake bashy re-exec test is unix-only here")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "status.json"), []byte(canned), 0o600)
	t.Setenv("FAKE_TUTOR_DIR", dir)
	exe, _ := os.Executable()
	s := newServer(7, exe, prefix, filepath.Join(dir, "7-serve.json"))
	s.refresh()
	ts := httptest.NewServer(s.handler())
	t.Cleanup(ts.Close)
	return &env{s, ts, dir}
}

func (e *env) do(t *testing.T, method, path, body string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest(method, e.ts.URL+path, strings.NewReader(body))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func (e *env) calls() string {
	b, _ := os.ReadFile(filepath.Join(e.dir, "calls.log"))
	return string(b)
}

func TestDisplayHidesSecrets(t *testing.T) {
	e := setup(t, "")
	for _, p := range []string{"/", "/api/display"} {
		code, body := e.do(t, "GET", p, "")
		if code != 200 {
			t.Fatalf("%s: %d", p, code)
		}
		for _, secret := range []string{e.s.tok.Owner, e.s.tok.Participants["alice"], e.s.tok.Participants["bob"], "secret.example"} {
			if strings.Contains(body, secret) {
				t.Fatalf("%s leaks %q", p, secret)
			}
		}
	}
	_, body := e.do(t, "GET", "/api/display", "")
	if !strings.Contains(body, "Build it") {
		t.Fatal(body)
	}
}

func TestOwnerKeyRequired(t *testing.T) {
	e := setup(t, "")
	for _, c := range [][2]string{{"POST", "next"}, {"POST", "extend"}, {"POST", "stop"}} {
		code, _ := e.do(t, c[0], "/t/wrongkey/"+c[1], "{}")
		if code != 403 {
			t.Fatalf("%s wrong key: %d", c[1], code)
		}
	}
	// A participant token is not a owner key.
	if code, _ := e.do(t, "POST", "/t/"+e.s.tok.Participants["alice"]+"/next", "{}"); code != 403 {
		t.Fatalf("participant token as key: %d", code)
	}
	if code, _ := e.do(t, "GET", "/t/nope", ""); code != 403 {
		t.Fatal(code)
	}
	if e.calls() != "tutor status 7 --json\n" {
		t.Fatalf("unauthorised calls reached bashy: %q", e.calls())
	}
	k := e.s.tok.Owner
	if code, body := e.do(t, "POST", "/t/"+k+"/next", "{}"); code != 200 {
		t.Fatal(code, body)
	}
	if code, body := e.do(t, "POST", "/t/"+k+"/extend", `{"by":"5m"}`); code != 200 {
		t.Fatal(code, body)
	}
	if code, _ := e.do(t, "POST", "/t/"+k+"/extend", `{"by":"--rm"}`); code != 400 {
		t.Fatal("bad duration accepted")
	}
	c := e.calls()
	if !strings.Contains(c, "tutor next 7 --json") || !strings.Contains(c, "tutor extend 7 t2 --by 5m --json") {
		t.Fatal(c)
	}
	code, body := e.do(t, "GET", "/api/t/"+k, "")
	if code != 200 || !strings.Contains(body, "secret.example") {
		t.Fatal(code, body)
	}
}

func TestParticipantTokenIsolation(t *testing.T) {
	e := setup(t, "")
	alice, bob := e.s.tok.Participants["alice"], e.s.tok.Participants["bob"]
	if alice == "" || bob == "" || alice == bob {
		t.Fatal("tokens not minted per owner")
	}
	// Bob owns no running step: he cannot finish alice's.
	if code, _ := e.do(t, "POST", "/s/"+bob+"/done", `{"step":"t2"}`); code != 403 {
		t.Fatalf("bob acting on alice's step: %d", code)
	}
	if code, _ := e.do(t, "POST", "/s/nosuchtoken/done", `{}`); code != 404 {
		t.Fatal(code)
	}
	// Alice cannot name another step.
	if code, _ := e.do(t, "POST", "/s/"+alice+"/done", `{"step":"t3"}`); code != 403 {
		t.Fatal(code)
	}
	if strings.Contains(e.calls(), "done") {
		t.Fatalf("done reached bashy: %s", e.calls())
	}
	code, body := e.do(t, "POST", "/s/"+alice+"/done", `{"evidence":"http://x/notepad/1"}`)
	if code != 200 {
		t.Fatal(code, body)
	}
	if !strings.Contains(e.calls(), "tutor done 7 t2 -m http://x/notepad/1 --json") {
		t.Fatal(e.calls())
	}
	// Views are private to the person.
	_, v := e.do(t, "GET", "/api/s/"+bob, "")
	if strings.Contains(v, "alice") && !strings.Contains(v, `"person":"bob"`) {
		t.Fatal(v)
	}
	var m map[string]any
	json.Unmarshal([]byte(v), &m)
	if m["mine"] != nil {
		t.Fatalf("bob sees a current step that is not his: %v", m["mine"])
	}
}

func TestTokensStableAndPrivate(t *testing.T) {
	e := setup(t, "")
	st, err := os.Stat(e.s.path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", st.Mode().Perm())
	}
	again := newServer(7, e.s.bin, "", e.s.path)
	if again.tok.Owner != e.s.tok.Owner || again.tok.Participants["alice"] != e.s.tok.Participants["alice"] {
		t.Fatal("links changed across restart")
	}
}

func TestPrefixAndRace(t *testing.T) {
	e := setup(t, "/tutor/")
	if code, _ := e.do(t, "GET", "/tutor/api/display", ""); code != 200 {
		t.Fatal(code)
	}
	if code, _ := e.do(t, "GET", "/api/display", ""); code == 200 {
		t.Fatal("served outside prefix")
	}
	alice := e.s.tok.Participants["alice"]
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e.s.refresh()
			e.do(t, "GET", "/tutor/api/display", "")
			e.do(t, "GET", "/tutor/api/s/"+alice, "")
			e.do(t, "POST", "/tutor/s/"+alice+"/done", `{}`)
		}()
	}
	wg.Wait()
}

func TestJoinDoneAndStop(t *testing.T) {
	e := setup(t, "")
	alice := e.s.tok.Participants["alice"]
	// stop reads the recorded address/key.
	if err := cmdJoin([]string{e.ts.URL + "/s/" + alice, "--done", "-m", "http://x/chart/1"}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(e.calls(), "-m http://x/chart/1") {
		t.Fatal(e.calls())
	}
	if err := cmdJoin([]string{e.ts.URL + "/s/" + alice + "x", "--done"}); err == nil {
		t.Fatal("bad token joined")
	}
	if err := cmdJoin([]string{"http://h/nothing"}); err == nil {
		t.Fatal("non-participant url accepted")
	}
	e.s.tok.Addr = strings.TrimPrefix(e.ts.URL, "http://")
	e.s.mu.Lock()
	e.s.saveLocked()
	e.s.mu.Unlock()
	if err := cmdStop([]string{"7", "-data", e.dir}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-e.s.quit:
	default:
		t.Fatal("stop did not signal shutdown")
	}
}

func TestFormatRolePageLabels(t *testing.T) {
	e := setup(t, "")
	if got := e.s.roleTitle(true, ""); got != "Host" {
		t.Fatal(got)
	}
	e.s.mu.Lock()
	e.s.st.Roles = Roles{Owner: "facilitator", Participants: []string{"speaker", "listener"}}
	e.s.st.Format = "meeting"
	for i := range e.s.st.Steps {
		e.s.st.Steps[i].Role = "speaker"
	}
	e.s.mu.Unlock()
	_, body := e.do(t, "GET", "/t/"+e.s.tok.Owner, "")
	if !strings.Contains(body, "<title>Facilitator</title>") {
		t.Fatal(body)
	}
	_, body = e.do(t, "GET", "/s/"+e.s.tok.Participants["alice"], "")
	if !strings.Contains(body, "<title>Speaker</title>") {
		t.Fatal(body)
	}
	_, body = e.do(t, "GET", "/api/s/"+e.s.tok.Participants["alice"], "")
	var view map[string]any
	if err := json.Unmarshal([]byte(body), &view); err != nil {
		t.Fatal(err)
	}
	if view["role"] != "speaker" {
		t.Fatal(view)
	}
	_, ownerJSON := e.do(t, "GET", "/api/t/"+e.s.tok.Owner, "")
	if !strings.Contains(ownerJSON, `"owner":"facilitator"`) {
		t.Fatal(ownerJSON)
	}
	roles := view["roles"].(map[string]any)
	if roles["owner"] != "facilitator" {
		t.Fatal(roles)
	}
}
