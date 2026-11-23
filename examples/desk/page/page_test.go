package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"desk-kit/contracttest"
)

func startServer(t *testing.T, breakM string) (string, *app) {
	t.Helper()
	old := breakMode
	breakMode = breakM
	t.Cleanup(func() { breakMode = old })
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "items"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{dir: dir, items: map[string]*item{}}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	a.baseURL = srv.URL
	return srv.URL, a
}

// cli runs the agent door against the test server and its store.
func cli(url string, a *app) func(...string) (int, string) {
	return func(args ...string) (int, string) {
		var out bytes.Buffer
		argv := append([]string{"--addr", strings.TrimPrefix(url, "http://"), "--data", a.dir}, args...)
		code := runCLI(argv, strings.NewReader(""), &out)
		return code, out.String()
	}
}

func pageConfig(base string, a *app) contracttest.Config {
	return contracttest.Config{BaseURL: base, Tool: tool, CLI: cli(base, a), Events: true, Embeddable: true,
		ItemURL: func(raw, _ string, created map[string]any) bool {
			u, err := url.Parse(raw)
			code, _ := created["join_code"].(string)
			return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && code != "" &&
				pageJoinCode(u.Path) == code && u.RawQuery == ""
		},
		BindGuard: func(addr string, lan bool) (string, error) {
			var logged bytes.Buffer
			err := bindGuard(addr, lan, &logged)
			return logged.String(), err
		}}
}

func TestDeskContract(t *testing.T) {
	url, a := startServer(t, "")
	contracttest.Run(t, pageConfig(url, a))
}

func TestContractRedOnBreak(t *testing.T) {
	for _, c := range []struct{ breakM, want string }{
		{"open-emits-content", "re-emits content"},
		{"no-csp", "Content-Security-Policy"},
		{"allow-secret", "secret field"},
		{"events-no-publish", "events: set must publish"},
	} {
		url, a := startServer(t, c.breakM)
		fails := contracttest.Check(pageConfig(url, a))
		if !strings.Contains(strings.Join(fails, "\n"), c.want) {
			t.Errorf("break %q: want a failure containing %q, got %q", c.breakM, c.want, fails)
		}
	}
}

func TestParseBlocks(t *testing.T) {
	body := "# Class\n\n```block poll:form-9f3a\n```\n\n" +
		"````md\n```block nope:x\n```\n````\n\n" +
		"~~~block\nembed:/whiteboard/?room=r1 id=board title=Sketch\n~~~\n" +
		"```block\npoll:form-2\n```\n```block here\n```\n"
	segs, err := parseBody(body)
	if err != nil {
		t.Fatal(err)
	}
	var got []block
	text := ""
	for _, s := range segs {
		if s.Block != nil {
			got = append(got, *s.Block)
		} else {
			text += s.Text
		}
	}
	want := []string{"poll-1 poll form-9f3a", "board embed /whiteboard/?room=r1", "poll-2 poll form-2", "here-1 here "}
	if len(got) != len(want) {
		t.Fatalf("blocks = %+v, want %v", got, want)
	}
	for i, b := range got {
		if s := b.ID + " " + b.Kind + " " + b.Ref; s != want[i] {
			t.Errorf("block %d = %q, want %q", i, s, want[i])
		}
	}
	if got[1].Options["title"] != "Sketch" {
		t.Errorf("embed options = %v", got[1].Options)
	}
	if !strings.Contains(text, "```block nope:x") {
		t.Errorf("a block fence inside an ordinary fence must stay text:\n%s", text)
	}

	for _, bad := range []string{
		"```block frobnicate:x\n```\n",
		"```block poll\n```\n",
		"```block here:x\n```\n",
		"```block embed:javascript:alert(1)\n```\n",
		"```block embed://evil.example/x\n```\n",
		"```block poll:a\n```\n```block poll:b id=poll-1\n```\n",
		"```block poll:a\n",
		"```block\npoll:a\npoll:b\n```\n",
		"```block poll:a nokey\n```\n",
	} {
		if _, err := parseBody(bad); err == nil {
			t.Errorf("parseBody(%q): want an error", bad)
		}
	}
}

const classPage = "# Sunday class\n\nWelcome.\n\n```block poll:form-9f3a\n```\n\n<script>alert(1)</script>\n"

func TestCLIRoundTrip(t *testing.T) {
	url, a := startServer(t, "")
	run := cli(url, a)
	f := filepath.Join(t.TempDir(), "class.md")
	os.WriteFile(f, []byte(classPage), 0o644)

	code, out := run("add", "--id", "class", "--code", "blue-fox", "-f", f, "--audience", "lan")
	if code != 0 || !strings.HasPrefix(out, "class "+url+"/blue-fox filled-by-agent") || strings.Contains(out, "Welcome") {
		t.Fatalf("add: %d %q", code, out)
	}
	code, out = run("open", "class")
	if code != 0 || !strings.Contains(out, url+"/blue-fox") || !strings.HasSuffix(out, "\nblue-fox\n") || strings.Contains(out, "Welcome") {
		t.Fatalf("open must print the join link and code without page content: %d %q", code, out)
	}
	code, out = run("show", "class", "--json")
	var shown struct {
		Title, Audience string
		Blocks          []block
		Revision        int
	}
	if code != 0 || json.Unmarshal([]byte(out), &shown) != nil {
		t.Fatalf("show: %d %q", code, out)
	}
	if shown.Title != "Sunday class" || shown.Audience != "lan" || len(shown.Blocks) != 1 || shown.Blocks[0].ID != "poll-1" {
		t.Errorf("show = %+v", shown)
	}

	bad := filepath.Join(t.TempDir(), "bad.md")
	os.WriteFile(bad, []byte("```block frobnicate:x\n```\n"), 0o644)
	if code, out = run("set", "class", "-f", bad); code != 2 || !strings.Contains(out, "unknown block kind") {
		t.Errorf("set with an unknown block kind: want exit 2, got %d %q", code, out)
	}
	if code, out = run("set", "class", "--audience", "everyone"); code != 2 {
		t.Errorf("set with a bad audience: want exit 2, got %d %q", code, out)
	}
	if code, out = run("set", "class", "--title", "Week 2"); code != 0 {
		t.Errorf("set --title: %d %q", code, out)
	}

	md := filepath.Join(t.TempDir(), "class-record.md")
	if code, out = run("export", "class", "--to", "md", "-o", md); code != 0 {
		t.Fatalf("export md: %d %q", code, out)
	}
	rec, _ := os.ReadFile(md)
	if !strings.Contains(string(rec), "> **poll** `poll-1` `form-9f3a`") || strings.Contains(string(rec), "```block") {
		t.Errorf("md record must carry each block as its labelled line:\n%s", rec)
	}
	code, out = run("export", "class", "--to", "html")
	if code != 0 || !strings.Contains(out, `data-block="poll-1"`) || strings.Contains(out, "<script>alert(1)") {
		t.Errorf("export html: %d\n%s", code, out)
	}
	if code, _ = run("export", "class", "--to", "pdf"); code != 2 {
		t.Errorf("export --to pdf: want exit 2, got %d", code)
	}
	if code, out = run("share", "class"); code != 0 || !strings.Contains(out, "invite: "+url+"/blue-fox#invite=") {
		t.Errorf("share: %d %q", code, out)
	}
	if code, out = run("close", "class"); code != 0 || !strings.Contains(out, " closed") {
		t.Errorf("close: %d %q", code, out)
	}
	if code, out = run("list"); code != 0 || strings.TrimSpace(out) != "class closed "+url+"/blue-fox" {
		t.Errorf("list: %d %q", code, out)
	}
}

func TestPageRendersBlocks(t *testing.T) {
	url, a := startServer(t, "")
	if code, out := cli(url, a)("add", "--id", "class", "--body", classPage); code != 0 {
		t.Fatal(out)
	}
	rq, err := http.NewRequest(http.MethodGet, url+"/?id=class", nil)
	if err != nil {
		t.Fatal(err)
	}
	rq.Header.Set("X-Facilitator-Key", a.items["class"].Owner)
	r, err := http.DefaultClient.Do(rq)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var b bytes.Buffer
	b.ReadFrom(r.Body)
	page := b.String()
	for _, want := range []string{`data-block="poll-1"`, `data-kind="poll"`, "Welcome.", "api/events?id=class", `data-page-revision>1<`} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks %q", want)
		}
	}
	if strings.Contains(page, "<script>alert(1)") {
		t.Error("page passes author HTML through")
	}
	r2, _ := http.Get(url + "/?id=nope")
	if r2.StatusCode != http.StatusNotFound {
		t.Errorf("unknown page: want 404, got %d", r2.StatusCode)
	}
}

// do sends one request straight to the handler from a chosen peer address.
func do(a *app, method, target, body, peer string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.RemoteAddr = peer
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	return w
}

func addPage(t *testing.T, a *app, id, audience string) string {
	t.Helper()
	w := do(a, "POST", "/api/action", `{"action":"add","id":"`+id+`","body":"# P\n\nsecret-plan","audience":"`+audience+`"}`, "127.0.0.1:5000", nil)
	var v struct{ OwnerKey string }
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &v) != nil || v.OwnerKey == "" {
		t.Fatalf("add: %d %s", w.Code, w.Body)
	}
	return v.OwnerKey
}

// A host or satellite page is served to this host only; a LAN peer (the
// server runs with --lan) sees a lan page and nothing else.
func TestAudienceGate(t *testing.T) {
	_, a := startServer(t, "")
	key := addPage(t, a, "host-page", "host")
	addPage(t, a, "sat-page", "satellite")
	addPage(t, a, "lan-page", "lan")
	lan := "192.0.2.20:5000"
	for _, id := range []string{"host-page", "sat-page"} {
		for _, req := range [][2]string{{"GET", "/?id=" + id}, {"GET", "/api/state?id=" + id}, {"GET", "/api/events?id=" + id},
			{"POST", `{"action":"show","id":"` + id + `"}`}, {"POST", `{"action":"open","id":"` + id + `"}`}} {
			var w *httptest.ResponseRecorder
			if req[0] == "GET" {
				w = do(a, "GET", req[1], "", lan, nil)
			} else {
				w = do(a, "POST", "/api/action", req[1], lan, nil)
			}
			if w.Code != http.StatusForbidden || strings.Contains(w.Body.String(), "secret-plan") {
				t.Errorf("%s %s from the LAN: want 403 without content, got %d", id, req[1], w.Code)
			}
		}
	}
	w := do(a, "POST", "/api/action", `{"action":"list"}`, lan, nil)
	if strings.Contains(w.Body.String(), "host-page") || !strings.Contains(w.Body.String(), "lan-page") {
		t.Errorf("list from the LAN must name lan pages only: %s", w.Body)
	}
	if w := do(a, "GET", "/?id=lan-page", "", lan, nil); w.Code != 200 || !strings.Contains(w.Body.String(), "Name or email") || strings.Contains(w.Body.String(), "secret-plan") {
		t.Errorf("new LAN peer should receive the handle prompt: got %d", w.Code)
	}
	if w := do(a, "POST", "/api/action", `{"action":"show","id":"host-page"}`, lan, map[string]string{"X-Facilitator-Key": key}); w.Code != 200 {
		t.Errorf("the organizer's key opens a host page from anywhere: got %d", w.Code)
	}
}

func TestOnlyOrganizerChangesAndExports(t *testing.T) {
	_, a := startServer(t, "")
	addPage(t, a, "p", "host")
	local := "127.0.0.1:5000"
	for _, body := range []string{`{"action":"set","id":"p","body":"x"}`, `{"action":"close","id":"p"}`, `{"action":"share","id":"p"}`} {
		if w := do(a, "POST", "/api/action", body, local, nil); w.Code != http.StatusForbidden {
			t.Errorf("%s without the owner key: want 403, got %d", body, w.Code)
		}
	}
	for _, to := range []string{"md", "html"} {
		if w := do(a, "GET", "/api/export?id=p&to="+to, "", local, nil); w.Code != http.StatusForbidden {
			t.Errorf("export %s without the owner key: want 403, got %d", to, w.Code)
		}
	}
}

// Security contract: show, state, export and the page carry participants
// by handle and auth only — never an address, fingerprint or cookie.
func TestParticipantAddressesNeverLeave(t *testing.T) {
	check := func(breakM string) []string {
		_, a := startServer(t, breakM)
		key := addPage(t, a, "p", "lan")
		a.mu.Lock()
		next := a.items["p"].clone()
		next.Participants = participantMap{"ada@192.0.2.7": {Handle: "ada", Address: "192.0.2.7", Fingerprint: "fp-7f3a",
			Cookie: "cookie-9b1c", Auth: "anonymous", JoinedAt: "2026-11-29T10:00:00Z"}}
		if err := a.saveLocked(&next, "here"); err != nil {
			t.Fatal(err)
		}
		a.mu.Unlock()
		owner := map[string]string{"X-Facilitator-Key": key}
		var leaks []string
		for _, r := range []struct{ name, method, target, body string }{
			{"show", "POST", "/api/action", `{"action":"show","id":"p"}`},
			{"open", "POST", "/api/action", `{"action":"open","id":"p"}`},
			{"state", "GET", "/api/state?id=p", ""},
			{"page", "GET", "/?id=p", ""},
			{"export md", "GET", "/api/export?id=p&to=md", ""},
			{"export html", "GET", "/api/export?id=p&to=html", ""},
		} {
			w := do(a, r.method, r.target, r.body, "127.0.0.1:5000", owner)
			if w.Code != 200 {
				t.Fatalf("%s: %d %s", r.name, w.Code, w.Body)
			}
			out := w.Body.String()
			if strings.HasPrefix(r.name, "show") || strings.HasPrefix(r.name, "export") {
				if !strings.Contains(out, "ada") {
					t.Errorf("%s: participant handle missing", r.name)
				}
			}
			for _, secret := range []string{"192.0.2.7", "fp-7f3a", "cookie-9b1c"} {
				if strings.Contains(out, secret) {
					leaks = append(leaks, r.name+" carries "+secret)
				}
			}
		}
		return leaks
	}
	if leaks := check(""); len(leaks) > 0 {
		t.Errorf("participant data leaked: %v", leaks)
	}
	if leaks := check("export-addresses"); len(leaks) == 0 {
		t.Error("break export-addresses: the leak check stayed green, want red")
	}
}
