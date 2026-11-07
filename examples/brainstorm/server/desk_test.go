package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"desk-kit/contracttest"
)

func deskServer(t *testing.T) (*httptest.Server, *app) {
	t.Helper()
	a := fresh(t)
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	return srv, a
}

// The shared desk contract runs against the real session server. The test
// server binds loopback; the shipped default stays LAN (see
// TestDefaultBindStaysLAN). StateHasContent: /api/state is the page's full
// session state, so it is exempt from the contentless-state probe.
func TestDeskContract(t *testing.T) {
	srv, _ := deskServer(t)
	contracttest.Run(t, contracttest.Config{BaseURL: srv.URL, Tool: "brainstorm", CLI: deskCLICallback(srv.URL), ItemParam: "session", StateHasContent: true})
}

func TestDefaultBindStaysLAN(t *testing.T) {
	host, _, err := net.SplitHostPort(defaultAddr)
	if err != nil || host != "0.0.0.0" {
		t.Fatalf("brainstorm keeps its LAN default bind (recorded exception); got %q", defaultAddr)
	}
}

func TestDeskPageCarriesKit(t *testing.T) {
	for _, m := range []string{"data-desk-copy", "desk-badge", "desk-item-header", "Copied", "prefers-color-scheme"} {
		if !bytes.Contains(page, []byte(m)) {
			t.Errorf("page lacks kit marker %q", m)
		}
	}
}

func TestDeskDoorMapsOntoSessions(t *testing.T) {
	_, a := deskServer(t)
	act := func(who, key string, body map[string]any) *httptest.ResponseRecorder {
		return request(a, who, key, "main", "/api/action", body)
	}
	w := act(token(), "", map[string]any{"action": "add", "id": "desk1", "body": "hello-idea"})
	good(t, w)
	var created map[string]any
	json.Unmarshal(w.Body.Bytes(), &created)
	key, _ := created["ownerKey"].(string)
	if !validToken(key) || created["facilitatorKey"] != key || created["id"] != "desk1" || created["status"] != "pending" {
		t.Fatalf("new: %v", created)
	}
	if strings.Contains(w.Body.String(), "hello-idea") {
		t.Fatal("new re-emitted content")
	}
	// Strangers cannot open/get a private session, nor see it in list.
	for _, name := range []string{"open", "show"} {
		if c := act(token(), "", map[string]any{"action": name, "id": "desk1"}).Code; c != 404 {
			t.Errorf("%s by stranger: want 404, got %d", name, c)
		}
	}
	l := act(token(), "", map[string]any{"action": "list"})
	good(t, l)
	if strings.Contains(l.Body.String(), "desk1") {
		t.Errorf("list leaked a private session: %s", l.Body.String())
	}
	// share needs the facilitator key; the invite admits one person via the page's join.
	if c := act(token(), "", map[string]any{"action": "share", "id": "desk1"}).Code; c != 404 {
		t.Errorf("share by stranger: want 404, got %d", c)
	}
	sh := act(token(), key, map[string]any{"action": "share", "id": "desk1"})
	good(t, sh)
	var shared map[string]any
	json.Unmarshal(sh.Body.Bytes(), &shared)
	invite, _ := shared["invite"].(string)
	if invite == "" || !strings.HasSuffix(shared["inviteURL"].(string), "#invite="+invite) {
		t.Fatalf("share: %v", shared)
	}
	guest := token()
	good(t, request(a, guest, "", "desk1", "/api/action", map[string]any{"action": "join", "invite": invite}))
	good(t, act(guest, "", map[string]any{"action": "open", "id": "desk1"}))
	// The existing page API sees the same session and card; put adds another.
	good(t, act(guest, "", map[string]any{"action": "set", "id": "desk1", "body": "guest-idea"}))
	good(t, request(a, token(), key, "desk1", "/api/action", map[string]any{"action": "phase", "phase": "discuss"}))
	g := act(guest, "", map[string]any{"action": "show", "id": "desk1"})
	good(t, g)
	if !strings.Contains(g.Body.String(), "hello-idea") || !strings.Contains(g.Body.String(), "guest-idea") {
		t.Fatalf("get after reveal lacks cards: %s", g.Body.String())
	}
	if c := act(guest, "", map[string]any{"action": "set", "id": "desk1", "body": "late"}).Code; c != 409 {
		t.Errorf("put after writing ended: want 409, got %d", c)
	}
	if c := act(guest, "", map[string]any{"action": "close", "id": "desk1"}).Code; c != 403 {
		t.Errorf("close by guest: want 403, got %d", c)
	}
	cl := act(token(), key, map[string]any{"action": "close", "id": "desk1"})
	good(t, cl)
	if !strings.Contains(cl.Body.String(), `"status":"closed"`) {
		t.Errorf("close: %s", cl.Body.String())
	}
	if !strings.Contains(request(a, token(), key, "desk1", "/api/state", nil).Body.String(), `"phase":"closed"`) {
		t.Error("close did not close the session on the page API")
	}
}

func TestDeskTrailingDataRejected(t *testing.T) {
	srv, _ := deskServer(t)
	for _, body := range []string{`{"action":"list"}{"action":"list"}`, `{"action":"list"}{}`} {
		resp, err := srv.Client().Post(srv.URL+"/api/action", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 400 {
			t.Errorf("%s: want 400, got %d", body, resp.StatusCode)
		}
	}
}

func TestCLIOpenHidesContent(t *testing.T) {
	srv, _ := deskServer(t)
	addr := strings.TrimPrefix(srv.URL, "http://")
	var out bytes.Buffer
	if c := runCLI([]string{"-addr", addr, "--json", "add", "--id", "cli1", "--body", "cli-secret-idea"}, &out); c != 0 {
		t.Fatalf("new: %d %s", c, out.String())
	}
	var created map[string]any
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("new --json: %s", out.String())
	}
	key := created["ownerKey"].(string)
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "open", "cli1", "--key", key}, &out); c != 0 {
		t.Fatalf("open: %d %s", c, out.String())
	}
	got := strings.Fields(out.String())
	if len(got) != 3 || got[0] != "cli1" || got[2] != "pending" || !strings.Contains(got[1], "session=cli1") {
		t.Errorf("open must print exactly id url status, got %q", out.String())
	}
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "list", "--key", key}, &out); c != 0 || !strings.Contains(out.String(), "cli1 pending") || strings.Contains(out.String(), "cli-secret-idea") {
		t.Errorf("list: %d %s", c, out.String())
	}
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "bogus"}, &out); c != 2 {
		t.Errorf("unknown verb: want 2, got %d", c)
	}
}

func TestCLIMultiRoundPresetsAndHistory(t *testing.T) {
	srv, _ := deskServer(t)
	addr := strings.TrimPrefix(srv.URL, "http://")

	var out bytes.Buffer
	if c := runCLI([]string{"-addr", addr, "add", "--id", "r1", "--title", "Round CLI", "--preset", "Seed alpha", "--preset", "Seed beta", "--json"}, &out); c != 0 {
		t.Fatalf("add with presets: %d %s", c, out.String())
	}
	var created map[string]any
	json.Unmarshal(out.Bytes(), &created)
	key, _ := created["ownerKey"].(string)

	// Advance to Round 2 skipping directly to vote and filtering by query "alpha".
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "set", "r1", "--key", key, "--next-round", "--start-phase", "vote", "--filter-query", "alpha"}, &out); c != 0 {
		t.Fatalf("set --next-round: %d %s", c, out.String())
	}

	// Show current round (Round 2): only "Seed alpha" should be present.
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "show", "r1", "--key", key}, &out); c != 0 {
		t.Fatalf("show r1: %d %s", c, out.String())
	}
	if !strings.Contains(out.String(), "Seed alpha") || strings.Contains(out.String(), `"text":"Seed beta"`) {
		// Note: rounds[0] in state has Seed beta, so parse JSON cards directly.
	}
	var st2 struct {
		Round int              `json:"round"`
		Phase string           `json:"phase"`
		Cards []map[string]any `json:"cards"`
	}
	json.Unmarshal(out.Bytes(), &st2)
	if st2.Round != 2 || st2.Phase != "vote" || len(st2.Cards) != 1 || st2.Cards[0]["text"] != "Seed alpha" {
		t.Fatalf("unexpected round 2 show: %+v", st2)
	}

	// Show historical Round 1 via --round 1: both cards present with advanced flags.
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "show", "r1", "--key", key, "--round", "1"}, &out); c != 0 {
		t.Fatalf("show --round 1: %d %s", c, out.String())
	}
	var st1 struct {
		ViewingRound  int              `json:"viewingRound"`
		AdvancedCount int              `json:"advancedCount"`
		Cards         []map[string]any `json:"cards"`
	}
	json.Unmarshal(out.Bytes(), &st1)
	if st1.ViewingRound != 1 || st1.AdvancedCount != 1 || len(st1.Cards) != 2 {
		t.Fatalf("unexpected round 1 show: %+v", st1)
	}

	// Create a new session seeded from Round 1 of r1.
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "add", "--id", "r2", "--from-session", "r1", "--from-round", "1", "--from-key", key, "--json"}, &out); c != 0 {
		t.Fatalf("add --from-session: %d %s", c, out.String())
	}
	var created2 map[string]any
	json.Unmarshal(out.Bytes(), &created2)
	key2, _ := created2["ownerKey"].(string)

	out.Reset()
	if c := runCLI([]string{"-addr", addr, "show", "r2", "--key", key2}, &out); c != 0 {
		t.Fatalf("show r2: %d %s", c, out.String())
	}
	var stCarry struct {
		FromSession string           `json:"fromSession"`
		FromRound   int              `json:"fromRound"`
		Cards       []map[string]any `json:"cards"`
	}
	json.Unmarshal(out.Bytes(), &stCarry)
	if stCarry.FromSession != "r1" || stCarry.FromRound != 1 || len(stCarry.Cards) != 2 {
		t.Fatalf("unexpected carried session r2: %+v", stCarry)
	}

	// Reopen Round 1 on r1.
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "set", "r1", "--key", key, "--reopen-round", "1"}, &out); c != 0 {
		t.Fatalf("set --reopen-round 1: %d %s", c, out.String())
	}
	out.Reset()
	if c := runCLI([]string{"-addr", addr, "show", "r1", "--key", key}, &out); c != 0 {
		t.Fatalf("show r1 after reopen: %d %s", c, out.String())
	}
	json.Unmarshal(out.Bytes(), &st2)
	if st2.Round != 1 || len(st2.Cards) != 2 {
		t.Fatalf("expected round 1 with 2 cards after reopen, got %+v", st2)
	}
}

func participantTokenForEmail(email string) string {
	sum := sha256.Sum256([]byte("brainstorm-e2e:" + email))
	return hex.EncodeToString(sum[:24]) // 48 hex chars matching kit.ValidToken
}

func TestBashySandboxE2EHappyPathWithFakeEmails(t *testing.T) {
	if testing.Short() {
		t.Skip("runs isolated bashy sandbox E2E")
	}
	bashyBin, err := exec.LookPath("bashy")
	if err != nil {
		t.Skip("bashy not on PATH")
	}
	srv, _ := deskServer(t)
	addr := strings.TrimPrefix(srv.URL, "http://")

	sandbox := t.TempDir()
	const (
		facilitatorEmail = "facilitator@example.test"
		aliceEmail       = "alice@example.test"
		bobEmail         = "bob@example.test"
	)
	caches, err := exec.Command("go", "env", "GOMODCACHE", "GOCACHE").Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	goenv := strings.Split(strings.TrimSpace(string(caches)), "\n")
	env := append(os.Environ(),
		"HOME="+filepath.Join(sandbox, "home"),
		"BASHY_HOME="+filepath.Join(sandbox, "bashy-home"),
		"BASHY_FLEET_DIR="+filepath.Join(sandbox, "fleet"),
		"BASHY_COORD_DIR="+filepath.Join(sandbox, "coord"),
		"BASHY_ROOM_DIR="+filepath.Join(sandbox, "room"),
		"BASHY_APPS_DIR="+filepath.Join(sandbox, "apps"),
		"XDG_CONFIG_HOME="+filepath.Join(sandbox, "config"),
		"XDG_DATA_HOME="+filepath.Join(sandbox, "data"),
		"XDG_CACHE_HOME="+filepath.Join(sandbox, "cache"),
		"BRAINSTORM_ADDR="+addr,
		"BASHY_SPRINT_ANNOUNCE=0",
		"GOMODCACHE="+goenv[0],
		"GOCACHE="+goenv[1],
		"GIT_AUTHOR_NAME=Facilitator",
		"GIT_AUTHOR_EMAIL="+facilitatorEmail,
		"GIT_COMMITTER_NAME=Facilitator",
		"GIT_COMMITTER_EMAIL="+facilitatorEmail,
	)
	if err := os.MkdirAll(filepath.Join(sandbox, "home"), 0o700); err != nil {
		t.Fatal(err)
	}
	bshPath, err := filepath.Abs("../brainstorm.bsh")
	if err != nil {
		t.Fatal(err)
	}
	runSandboxBsh := func(key string, args ...string) string {
		t.Helper()
		argv := append([]string{"--bashsharp", bshPath}, args...)
		cmd := exec.Command(bashyBin, argv...)
		cmd.Env = env
		if key != "" {
			cmd.Env = append(cmd.Env, "BRAINSTORM_KEY="+key)
		}
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bashy %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}
	httpJSON := func(method, path, key, email string, body any) map[string]any {
		t.Helper()
		var rBody *bytes.Reader
		if body != nil {
			raw, _ := json.Marshal(body)
			rBody = bytes.NewReader(raw)
		} else {
			rBody = bytes.NewReader(nil)
		}
		req, err := http.NewRequest(method, srv.URL+path, rBody)
		if err != nil {
			t.Fatal(err)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if key != "" {
			req.Header.Set("X-Facilitator-Key", key)
		}
		if email != "" {
			req.AddCookie(&http.Cookie{Name: "brainstorm-participant", Value: participantTokenForEmail(email)})
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s %s (%s): status %d", method, path, email, resp.StatusCode)
		}
		var out map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	// 1. Facilitator (facilitator@example.test) creates session with preset seed cards in the bashy sandbox.
	var created map[string]any
	if err := json.Unmarshal([]byte(runSandboxBsh("", "add", "--id", "sbx-e2e", "--title", "Sandbox Happy Path", "--preset", "Seed 1: Solar roof\nSeed 2: Rainwater harvesting", "--json")), &created); err != nil {
		t.Fatal(err)
	}
	facKey, _ := created["ownerKey"].(string)

	// 2. Invite and join alice@example.test and bob@example.test.
	for _, email := range []string{aliceEmail, bobEmail} {
		inv := httpJSON("POST", "/api/action", facKey, facilitatorEmail, map[string]any{"action": "invite", "session": "sbx-e2e", "name": email})
		code, _ := inv["inviteCode"].(string)
		httpJSON("POST", "/api/action", "", email, map[string]any{"action": "join", "session": "sbx-e2e", "invite": code})
	}

	// 3. Round 1 write phase: alice@example.test and bob@example.test see the 2 preset cards and add their own ideas.
	httpJSON("POST", "/api/action", "", aliceEmail, map[string]any{"action": "add", "session": "sbx-e2e", "column": 0, "name": aliceEmail, "text": "Alice idea: grid battery"})
	httpJSON("POST", "/api/action", "", bobEmail, map[string]any{"action": "add", "session": "sbx-e2e", "column": 1, "name": bobEmail, "text": "Bob idea: diesel generator"})

	// 4. Reveal and vote in Round 1.
	runSandboxBsh(facKey, "set", "sbx-e2e", "--phase", "discuss")
	runSandboxBsh(facKey, "set", "sbx-e2e", "--phase", "vote")
	st1 := httpJSON("GET", "/api/state?session=sbx-e2e", "", aliceEmail, nil)
	cards1, _ := st1["cards"].([]any)
	cardIDByText := map[string]string{}
	for _, raw := range cards1 {
		cm := raw.(map[string]any)
		cardIDByText[cm["text"].(string)] = cm["id"].(string)
	}
	for _, voter := range []string{aliceEmail, bobEmail} {
		httpJSON("POST", "/api/action", "", voter, map[string]any{"action": "vote", "session": "sbx-e2e", "id": cardIDByText["Seed 1: Solar roof"]})
		httpJSON("POST", "/api/action", "", voter, map[string]any{"action": "vote", "session": "sbx-e2e", "id": cardIDByText["Alice idea: grid battery"]})
	}

	// 5. Advance to Round 2 in write phase with min-votes=2.
	runSandboxBsh(facKey, "set", "sbx-e2e", "--next-round", "--start-phase", "write", "--min-votes", "2")
	var st2 struct {
		Round  int              `json:"round"`
		Phase  string           `json:"phase"`
		Cards  []map[string]any `json:"cards"`
		Rounds []RoundSnapshot  `json:"rounds"`
	}
	if err := json.Unmarshal([]byte(runSandboxBsh(facKey, "show", "sbx-e2e")), &st2); err != nil {
		t.Fatal(err)
	}
	if st2.Round != 2 || st2.Phase != "write" || len(st2.Cards) != 2 || len(st2.Rounds) != 1 || st2.Rounds[0].AdvancedCount != 2 {
		t.Fatalf("unexpected round 2 state: %+v", st2)
	}

	// 6. Inspect Round 1 history via CLI (--round 1) and carry forward into a new session.
	var r1Hist struct {
		ViewingRound  int `json:"viewingRound"`
		AdvancedCount int `json:"advancedCount"`
	}
	if err := json.Unmarshal([]byte(runSandboxBsh(facKey, "show", "sbx-e2e", "--round", "1")), &r1Hist); err != nil {
		t.Fatal(err)
	}
	if r1Hist.ViewingRound != 1 || r1Hist.AdvancedCount != 2 {
		t.Fatalf("unexpected round 1 history: %+v", r1Hist)
	}
}

func deskCLICallback(base string) func(...string) (int, string) {
	return func(args ...string) (int, string) {
		var out bytes.Buffer
		argv := append([]string{"--addr", strings.TrimPrefix(base, "http://")}, args...)
		code := runCLI(argv, &out)
		return code, out.String()
	}
}
