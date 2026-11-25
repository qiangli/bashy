// Focused tests for the desk form tool (F1): the shared contract goes
// green, and each form behaviour has its own red/green proof — secret
// refusal, definition validation, required enforcement, partial-save
// persistence across reloads, YAML/JSON parity, the recommended mark on
// the page, and CLI/HTTP duality (open hides content, get returns
// answers keyed by field id).
package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	kit "github.com/qiangli/bashy/examples/desk/kit"
	"github.com/qiangli/bashy/examples/desk/kit/contracttest"
)

func startServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "items"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{dir: dir, items: map[string]*item{}}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	a.baseURL = srv.URL
	return srv.URL
}

// serveApp exposes the app behind the test server for page assertions.
func serveApp(t *testing.T) (*app, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "items"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{dir: dir, items: map[string]*item{}}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	a.baseURL = srv.URL
	return a, srv.URL
}

func post(t *testing.T, base, body string, key string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest("POST", base+"/api/action", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Facilitator-Key", key)
	}
	r, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r.Body)
	return r.StatusCode, buf.Bytes()
}

func newForm(t *testing.T, base, payload string) (string, string) {
	t.Helper()
	code, raw := post(t, base, payload, "")
	if code != 200 {
		t.Fatalf("new: want 200, got %d: %s", code, raw)
	}
	var v struct {
		ID       string `json:"id"`
		URL      string `json:"url"`
		Status   string `json:"status"`
		OwnerKey string `json:"ownerKey"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("new: not JSON: %s", raw)
	}
	if v.ID == "" || v.OwnerKey == "" {
		t.Fatalf("new: missing id/ownerKey in %s", raw)
	}
	return v.ID, v.OwnerKey
}

const contactDef = `[{"id":"name","label":"Full name","kind":"text","required":true},` +
	`{"id":"contact","label":"Preferred contact","kind":"choice","required":true,` +
	`"options":["email","phone","post"],"recommended":"email"},` +
	`{"id":"topics","label":"Topics","kind":"multi","options":["news","events","offers"],"recommended":"news"},` +
	`{"id":"subscribe","label":"Subscribe?","kind":"yes-no","required":true}]`

func TestDeskContract(t *testing.T) {
	base := startServer(t)
	contracttest.Run(t, contracttest.Config{BaseURL: base, Tool: "form", CLI: deskCLICallback(base)})
}

// TestRenderEndpoint proves deskRender's server half: the mounted POST
// /api/render returns the shared renderer's sanitized HTML — script
// stripped, Markdown rendered, chart fences drawn with the C1 renderer.
func TestRenderEndpoint(t *testing.T) {
	base := startServer(t)
	body, _ := json.Marshal(map[string]string{"text": "# Help\n\n<script>alert(1)</script>\n\n```chart widget=bar\nname,value\na,1\nb,2\n```\n"})
	req, err := http.NewRequest("POST", base+"/api/render", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("render: want 200, got %d: %s", res.StatusCode, raw)
	}
	var v struct {
		HTML string `json:"html"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("render: not JSON: %s", raw)
	}
	for _, want := range []string{`<div class="desk-content">`, "<h1", "<svg", "desk-chart", `class="desk-series-1"`} {
		if !strings.Contains(v.HTML, want) {
			t.Errorf("rendered HTML lacks %q:\n%s", want, v.HTML)
		}
	}
	for _, banned := range []string{"<script", "alert(1)", "<style", "style=", "font-family"} {
		if strings.Contains(v.HTML, banned) {
			t.Errorf("rendered HTML contains banned %q:\n%s", banned, v.HTML)
		}
	}
	// GET is refused; the endpoint is POST-only.
	get, err := http.Get(base + "/api/render")
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	if get.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/render: got %d, want 405", get.StatusCode)
	}
}

func TestSecretDefinitionRefused(t *testing.T) {
	base := startServer(t)
	for _, tc := range []struct {
		name string
		def  string
	}{
		{"secret kind", `[{"id":"token","label":"Token","kind":"secret"}]`},
		{"secret-named id", `[{"id":"db_password","label":"DB password","kind":"text"}]`},
		{"secret answer key", ``},
	} {
		var code int
		var raw []byte
		if tc.def == "" {
			id, _ := newForm(t, base, `{"action":"add","title":"s","definition":`+contactDef+`}`)
			code, raw = post(t, base, `{"action":"answer","id":"`+id+`","answers":{"api_key":"x"}}`, "")
		} else {
			code, raw = post(t, base, `{"action":"add","title":"s","definition":`+tc.def+`}`, "")
		}
		if code != 400 {
			t.Errorf("%s: want 400, got %d (%s)", tc.name, code, raw)
			continue
		}
		low := strings.ToLower(string(raw))
		if !strings.Contains(low, "secret") || !strings.Contains(low, "bashy ask") {
			t.Errorf("%s: refusal must name secrets and point to bashy ask, got %q", tc.name, raw)
		}
	}
}

func TestDefinitionValidation(t *testing.T) {
	base := startServer(t)
	for _, tc := range []struct {
		name string
		def  string
		want string
	}{
		{"unknown kind", `[{"id":"a","label":"A","kind":"slider"}]`, "unknown kind"},
		{"options on text", `[{"id":"a","label":"A","kind":"text","options":["x"]}]`, "no options"},
		{"empty options on choice", `[{"id":"a","label":"A","kind":"choice","options":[]}]`, "at least one option"},
		{"dangling recommended", `[{"id":"a","label":"A","kind":"choice","options":["x"],"recommended":"y"}]`, "not one of its options"},
		{"recommended on text", `[{"id":"a","label":"A","kind":"text","recommended":"x"}]`, "no options"},
		{"duplicate id", `[{"id":"a","label":"A","kind":"text"},{"id":"a","label":"B","kind":"text"}]`, "twice"},
		{"bad id", `[{"id":"has space","label":"A","kind":"text"}]`, "for an id"},
		{"missing label", `[{"id":"a","kind":"text"}]`, "label is required"},
	} {
		code, raw := post(t, base, `{"action":"add","title":"s","definition":`+tc.def+`}`, "")
		if code != 400 {
			t.Errorf("%s: want 400, got %d (%s)", tc.name, code, raw)
			continue
		}
		if !strings.Contains(string(raw), tc.want) {
			t.Errorf("%s: error should say %q, got %q", tc.name, tc.want, raw)
		}
	}
}

func TestAnswerRoundTripAndReload(t *testing.T) {
	a, base := serveApp(t)
	id, key := newForm(t, base, `{"action":"add","title":"Contact","definition":`+contactDef+`}`)

	// Partial save: required fields missing, still stored as pending.
	code, raw := post(t, base, `{"action":"answer","id":"`+id+`","answers":{"name":"Ada"}}`, "")
	if code != 200 {
		t.Fatalf("partial answer: want 200, got %d: %s", code, raw)
	}
	if !strings.Contains(string(raw), `"pending"`) {
		t.Fatalf("partial answer: want status pending, got %s", raw)
	}

	// Reload from disk in a fresh app: partial saves survive reload.
	fresh := &app{dir: a.dir, baseURL: base, items: map[string]*item{}}
	it, err := fresh.loadLocked(id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if it.Answers["name"] != "Ada" {
		t.Fatalf("reload: answers lost, got %v", it.Answers)
	}

	// Complete submit with a missing required field is refused by name.
	code, raw = post(t, base, `{"action":"answer","id":"`+id+`","complete":true,`+
		`"answers":{"name":"Ada","contact":"email"}}`, "")
	if code != 400 || !strings.Contains(string(raw), "subscribe") {
		t.Fatalf("complete with missing required: want 400 naming subscribe, got %d: %s", code, raw)
	}

	// Unknown answer ids never vanish silently.
	code, raw = post(t, base, `{"action":"answer","id":"`+id+`","answers":{"bogus":"x"}}`, "")
	if code != 400 || !strings.Contains(string(raw), "no such field") {
		t.Fatalf("unknown answer id: want 400, got %d: %s", code, raw)
	}

	// Full submit: answered, readable back keyed by field id.
	complete := `{"action":"answer","id":"` + id + `","complete":true,"answers":` +
		`{"name":"Ada","contact":"email","topics":["news","offers"],"subscribe":true}}`
	if code, raw := post(t, base, complete, ""); code != 200 {
		t.Fatalf("complete answer: want 200, got %d: %s", code, raw)
	}
	code, raw = post(t, base, `{"action":"show","id":"`+id+`"}`, key)
	if code != 200 {
		t.Fatalf("get: want 200, got %d: %s", code, raw)
	}
	var got struct {
		Status  string         `json:"status"`
		Answers map[string]any `json:"answers"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("get: not JSON: %s", raw)
	}
	if got.Status != "answered" {
		t.Errorf("get: want status answered, got %q", got.Status)
	}
	if got.Answers["name"] != "Ada" || got.Answers["contact"] != "email" {
		t.Errorf("get: answers not keyed by field id: %v", got.Answers)
	}
	if got.Answers["subscribe"] != "yes" {
		t.Errorf("get: yes-no should normalise to yes/no, got %v", got.Answers["subscribe"])
	}
	topics, _ := got.Answers["topics"].([]any)
	if len(topics) != 2 {
		t.Errorf("get: multi should keep both picks, got %v", got.Answers["topics"])
	}
}

func TestYAMLJSONParity(t *testing.T) {
	jraw, err := os.ReadFile("../examples/contact.json")
	if err != nil {
		t.Fatal(err)
	}
	yraw, err := os.ReadFile("../examples/contact.yaml")
	if err != nil {
		t.Fatal(err)
	}
	jt, jdef, err := ParseDefinition(jraw, "contact.json")
	if err != nil {
		t.Fatalf("json: %v", err)
	}
	yt, ydef, err := ParseDefinition(yraw, "contact.yaml")
	if err != nil {
		t.Fatalf("yaml: %v", err)
	}
	if jt != yt {
		t.Errorf("title differs: %q vs %q", jt, yt)
	}
	jc, _ := json.Marshal(jdef)
	yc, _ := json.Marshal(ydef)
	if !bytes.Equal(jc, yc) {
		t.Errorf("definitions differ:\njson: %s\nyaml: %s", jc, yc)
	}
	if len(ydef) != 6 {
		t.Errorf("want 6 example fields, got %d", len(ydef))
	}
}

func TestRecommendedMarkOnPage(t *testing.T) {
	_, base := serveApp(t)
	id, _ := newForm(t, base, `{"action":"add","title":"Contact","definition":`+contactDef+`}`)
	r, err := http.Get(base + "/?id=" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	buf := new(bytes.Buffer)
	_, _ = buf.ReadFrom(r.Body)
	page := buf.String()
	if !strings.Contains(page, "desk-recommended") || !strings.Contains(page, "recommended") {
		t.Errorf("page lacks the recommended mark for the suggested option")
	}
	if !strings.Contains(page, `value="email"`) {
		t.Errorf("page lacks the recommended option itself")
	}
}

func TestCLIOpenHidesContentGetReturnsAnswers(t *testing.T) {
	srvURL := startServer(t)
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer
	newArgs := []string{"add", "-addr", addr, "--json", "--id", "cli-probe", "--title", "T",
		"--body", "cli-secret-body", "--def", "../examples/contact.json"}
	if code := runCLI(newArgs, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}
	var created map[string]any
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("new: not JSON: %s", out.String())
	}
	key, _ := created["ownerKey"].(string)
	if key == "" {
		t.Fatalf("new: no ownerKey in %s", out.String())
	}
	if strings.Contains(out.String(), "cli-secret-body") {
		t.Errorf("CLI new re-emits content: %s", out.String())
	}
	out.Reset()
	if code := runCLI([]string{"open", "-addr", addr, "cli-probe"}, &out); code != 0 {
		t.Fatalf("open: exit %d: %s", code, out.String())
	}
	if strings.Contains(out.String(), "cli-secret-body") {
		t.Errorf("CLI open re-emits content: %s", out.String())
	}
	if !strings.Contains(out.String(), "cli-probe") {
		t.Errorf("CLI open lacks the id: %s", out.String())
	}
	out.Reset()
	if code := runCLI([]string{"set", "-addr", addr, "--key", key, "--answer", "name=Ada", "cli-probe"}, &out); code != 0 {
		t.Fatalf("put: exit %d: %s", code, out.String())
	}
	out.Reset()
	if code := runCLI([]string{"show", "-addr", addr, "--key", key, "--json", "cli-probe"}, &out); code != 0 {
		t.Fatalf("get: exit %d: %s", code, out.String())
	}
	var got struct {
		Answers map[string]any `json:"answers"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("get: not JSON: %s", out.String())
	}
	if got.Answers["name"] != "Ada" {
		t.Errorf("CLI get must return answers keyed by field id: %s", out.String())
	}
}

func TestCLIAddPrintsWorkingInviteURL(t *testing.T) {
	base := startServer(t)
	addr := strings.TrimPrefix(base, "http://")
	var out bytes.Buffer
	if code := runCLI([]string{"add", "-addr", addr, "--id", "invite-probe", "--title", "Invite probe"}, &out); code != 0 {
		t.Fatalf("add: exit %d: %s", code, out.String())
	}
	parts := strings.Fields(strings.SplitN(out.String(), "\n", 2)[0])
	if len(parts) != 3 || parts[0] != "invite-probe" {
		t.Fatalf("add: expected id URL status, got %q", out.String())
	}
	link, err := url.Parse(parts[1])
	if err != nil || link.Query().Get("id") != "invite-probe" {
		t.Fatalf("add: invalid form URL %q: %v", parts[1], err)
	}
	invite := strings.TrimPrefix(link.Fragment, "invite=")
	if !strings.HasPrefix(link.Fragment, "invite=") || invite == "" {
		t.Fatalf("add: URL lacks invite: %q", parts[1])
	}
	code, raw := post(t, base, `{"action":"join","id":"invite-probe","invite":"`+invite+`"}`, "")
	if code != 200 {
		t.Fatalf("printed invite cannot be redeemed: %d %s", code, raw)
	}
	keyLine := strings.SplitN(out.String(), "ownerKey: ", 2)
	if len(keyLine) != 2 {
		t.Fatalf("add: missing owner key: %q", out.String())
	}
	owner := strings.Fields(keyLine[1])[0]
	out.Reset()
	if code := runCLI([]string{"share", "-addr", addr, "--key", owner, "invite-probe"}, &out); code != 0 {
		t.Fatalf("share: exit %d: %s", code, out.String())
	}
	shared := strings.Fields(strings.SplitN(out.String(), "\n", 2)[0])
	if len(shared) != 3 || !strings.Contains(shared[1], "#invite=") || shared[1] == parts[1] {
		t.Fatalf("share: expected a fresh invite URL, got %q", out.String())
	}
}

// Flags work after the id too (form get ID --json): argument order must
// never silently drop a flag.
func TestCLIFlagsAfterID(t *testing.T) {
	srvURL := startServer(t)
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer
	if code := runCLI([]string{"add", "-addr", addr, "--json", "--id", "order-probe",
		"--title", "T", "--def", "../examples/contact.json"}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}
	var created map[string]any
	_ = json.Unmarshal(out.Bytes(), &created)
	key, _ := created["ownerKey"].(string)
	out.Reset()
	if code := runCLI([]string{"show", "order-probe", "--key", key, "-addr", addr, "--json"}, &out); code != 0 {
		t.Fatalf("get ID --json: exit %d: %s", code, out.String())
	}
	var got struct {
		Definition []FormField `json:"definition"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("get ID --json must print the raw JSON object: %s", out.String())
	}
	if len(got.Definition) != 6 {
		t.Errorf("want 6 definition fields, got %d: %s", len(got.Definition), out.String())
	}
}

func TestCLIWaitAnsweredAndTimeout(t *testing.T) {
	a, srvURL := serveApp(t)
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer
	if code := runCLI([]string{"add", "-addr", addr, "-data", a.dir, "--json", "--id", "wait-probe",
		"--def", "../examples/contact.json"}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}
	var created map[string]any
	_ = json.Unmarshal(out.Bytes(), &created)
	if key, _ := created["ownerKey"].(string); key == "" {
		t.Fatalf("new: no ownerKey in %s", out.String())
	}

	// The creating owner does not have to copy the returned key into --key:
	// wait resolves it from the same local form store.
	out.Reset()
	if code := runCLI([]string{"wait", "-addr", addr, "-data", a.dir,
		"--timeout", "50ms", "--interval", "10ms", "wait-probe"}, &out); code != 1 {
		t.Fatalf("wait on pending: want exit 1, got %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "timed out") {
		t.Errorf("wait timeout should say so: %s", out.String())
	}

	// Answered form: wait returns the answers, exit 0.
	complete := `{"action":"answer","id":"wait-probe","complete":true,"answers":` +
		`{"name":"Ada","contact":"phone","subscribe":"no"}}`
	if code, raw := post(t, srvURL, complete, ""); code != 200 {
		t.Fatalf("complete: want 200, got %d: %s", code, raw)
	}
	out.Reset()
	if code := runCLI([]string{"wait", "-addr", addr, "-data", a.dir, "--json",
		"--timeout", "5s", "--interval", "10ms", "wait-probe"}, &out); code != 0 {
		t.Fatalf("wait on answered: want exit 0, got %d: %s", code, out.String())
	}
	var got struct {
		Status  string         `json:"status"`
		Answers map[string]any `json:"answers"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("wait: not JSON: %s", out.String())
	}
	if got.Status != "answered" || got.Answers["contact"] != "phone" {
		t.Errorf("wait should return the answered form: %s", out.String())
	}
}

const followupDef = `[{"id":"why","label":"Why?","kind":"textarea","required":true},` +
	`{"id":"route","label":"Which route?","kind":"choice","options":["safe","fast"],"recommended":"safe"}]`

func TestQuestionnaireAppendRoundKeepsStableQuestionIDs(t *testing.T) {
	base := startServer(t)
	id, key := newForm(t, base, `{"action":"ask","title":"Plan","definition":`+contactDef+`}`)

	code, raw := post(t, base, `{"action":"answer","id":"`+id+`","answers":{"q1":"Ada"}}`, "")
	if code != 200 {
		t.Fatalf("partial q1: want 200, got %d: %s", code, raw)
	}
	code, raw = post(t, base, `{"action":"ask","id":"`+id+`","definition":`+followupDef+`}`, key)
	if code != 200 {
		t.Fatalf("append: want 200, got %d: %s", code, raw)
	}
	code, raw = post(t, base, `{"action":"show","id":"`+id+`"}`, key)
	if code != 200 {
		t.Fatalf("get: want 200, got %d: %s", code, raw)
	}
	var got struct {
		Definition []FormField               `json:"definition"`
		Answers    map[string]questionAnswer `json:"answers"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("get: not questionnaire JSON: %s", raw)
	}
	if len(got.Definition) != 6 || got.Definition[0].ID != "q1" || got.Definition[3].ID != "q4" ||
		got.Definition[4].ID != "q5" || got.Definition[5].ID != "q6" {
		t.Fatalf("append must retain q1..q4 and add q5..q6: %#v", got.Definition)
	}
	if got.Answers["q1"].Status != "answered" || got.Answers["q1"].Answer != "Ada" {
		t.Errorf("q1 answer/status changed after append: %#v", got.Answers["q1"])
	}
	if got.Answers["q5"].Status != "pending" {
		t.Errorf("new q5 should be pending: %#v", got.Answers["q5"])
	}
}

func TestQuestionnairePartialSaveSurvivesReloadAndMerges(t *testing.T) {
	a, base := serveApp(t)
	id, _ := newForm(t, base, `{"action":"ask","title":"Plan","definition":`+followupDef+`}`)
	if code, raw := post(t, base, `{"action":"answer","id":"`+id+`","answers":{"q2":"safe"}}`, ""); code != 200 {
		t.Fatalf("save q2: want 200, got %d: %s", code, raw)
	}

	fresh := &app{dir: a.dir, baseURL: base, items: map[string]*item{}}
	it, err := fresh.loadLocked(id)
	if err != nil {
		t.Fatal(err)
	}
	if it.Answers["q2"] != "safe" {
		t.Fatalf("q2 lost on reload: %#v", it.Answers)
	}
	if code, raw := post(t, base, `{"action":"answer","id":"`+id+`","answers":{"q1":"Because"}}`, ""); code != 200 {
		t.Fatalf("save q1: want 200, got %d: %s", code, raw)
	}
	fresh.items = map[string]*item{}
	it, err = fresh.loadLocked(id)
	if err != nil {
		t.Fatal(err)
	}
	if it.Answers["q1"] != "Because" || it.Answers["q2"] != "safe" {
		t.Fatalf("out-of-order partial saves must merge: %#v", it.Answers)
	}
}

func TestQuestionnaireRefusesSecretQuestions(t *testing.T) {
	base := startServer(t)
	code, raw := post(t, base, `{"action":"ask","definition":[`+
		`{"id":"api_key","label":"API key","kind":"text"}]}`, "")
	if code != 400 || !strings.Contains(strings.ToLower(string(raw)), "bashy ask") {
		t.Fatalf("secret question: want 400 pointing to bashy ask/vault, got %d: %s", code, raw)
	}
}

func TestQuestionnaireOpenHidesContent(t *testing.T) {
	srvURL := startServer(t)
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer
	if code := runCLI([]string{"ask", "-addr", addr, "--json", "--id", "questions",
		"--title", "Private plan", "--def", "../examples/contact.json"}, &out); code != 0 {
		t.Fatalf("ask: exit %d: %s", code, out.String())
	}
	out.Reset()
	if code := runCLI([]string{"open", "questions", "-addr", addr, "--json"}, &out); code != 0 {
		t.Fatalf("open: exit %d: %s", code, out.String())
	}
	if strings.Contains(out.String(), "Private plan") || strings.Contains(out.String(), "Full name") ||
		strings.Contains(out.String(), "definition") || strings.Contains(out.String(), "answers") {
		t.Errorf("open re-emits questionnaire content: %s", out.String())
	}
}

func TestQuestionnairePageNumbersQuestionsAndMarksRecommendation(t *testing.T) {
	_, base := serveApp(t)
	id, _ := newForm(t, base, `{"action":"ask","title":"Plan","definition":`+followupDef+`}`)
	r, err := http.Get(base + "/?id=" + id)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	raw, _ := io.ReadAll(r.Body)
	page := string(raw)
	for _, want := range []string{"Q1", "Q2", "recommended", `data-question-id="q1"`} {
		if !strings.Contains(page, want) {
			t.Errorf("questionnaire page lacks %q", want)
		}
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

func TestCLIRefusesForeignServerStoreAndDiscoversServerInfo(t *testing.T) {
	for _, k := range []string{"FORM_ADDR", "FORM_DATA", "FORM_PREFIX", "FORM_PUBLIC_URL", "FORM_KEY", "BASHY_APP", "BASHY_APP_ADDR", "BASHY_APP_DATA"} {
		t.Setenv(k, "")
	}
	// Server 1 represents another custom app (e.g. team-signup) with its own store.
	foreignApp, foreignURL := serveApp(t)
	foreignAddr := strings.TrimPrefix(foreignURL, "http://")
	defaultDir := t.TempDir()

	// Explicit --addr + mismatched --data must refuse before mutating foreignApp.
	var out bytes.Buffer
	code := runCLI([]string{"add", "--addr", foreignAddr, "--data", defaultDir, "--def", "../examples/contact.json", "--title", "Review"}, &out)
	if code != 1 || !strings.Contains(out.String(), "different form store") {
		t.Fatalf("expected foreign-store refusal (exit 1), got %d: %s", code, out.String())
	}
	entries, _ := os.ReadDir(filepath.Join(foreignApp.dir, "items"))
	if len(entries) != 0 {
		t.Fatalf("foreign store must remain untouched, got %d items", len(entries))
	}

	// JSON mode returns a structured store_mismatch error.
	out.Reset()
	code = runCLI([]string{"list", "--addr", foreignAddr, "--data", defaultDir, "--json"}, &out)
	if code != 1 || !strings.Contains(out.String(), `"store_mismatch"`) {
		t.Fatalf("expected JSON store_mismatch error, got %d: %s", code, out.String())
	}

	// A legacy pre-fix server without X-Desk-Store is also refused when --data is expected.
	var legacyActionHits atomic.Int32
	legacySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		legacyActionHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer legacySrv.Close()
	out.Reset()
	legacyAddr := strings.TrimPrefix(legacySrv.URL, "http://")
	code = runCLI([]string{"add", "--addr", legacyAddr, "--data", defaultDir, "--def", "../examples/contact.json"}, &out)
	if code != 1 || legacyActionHits.Load() != 0 {
		t.Fatalf("legacy foreign server should be refused before /api/action: exit=%d hits=%d out=%s", code, legacyActionHits.Load(), out.String())
	}

	// Non-default stores derive their own port instead of 127.0.0.1:18107.
	if got := derivedStoreAddr(foreignApp.dir); got == defaultFormAddr {
		t.Fatalf("custom store derived default address %s", got)
	}

	// When a store has a live server.json, -data DIR discovers its listener without -addr.
	ownApp, ownURL := serveApp(t)
	ownAddr := strings.TrimPrefix(ownURL, "http://")
	writeServerInfo(ownApp.dir, ownAddr, "")
	out.Reset()
	if code := runCLI([]string{"add", "--data", ownApp.dir, "--id", "discovered-form", "--def", "../examples/contact.json", "--title", "Own"}, &out); code != 0 {
		t.Fatalf("add via server.json discovery: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "discovered-form") {
		t.Fatalf("unexpected add output: %s", out.String())
	}
}

func TestCLIDiscoversAppRuntime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake bashy script uses /bin/sh")
	}
	a, base := serveApp(t)
	for _, k := range []string{"FORM_ADDR", "FORM_DATA", "FORM_PREFIX", "FORM_PUBLIC_URL", "FORM_KEY", "BASHY_APP", "BASHY_APP_ADDR", "BASHY_APP_DATA"} {
		t.Setenv(k, "")
	}
	t.Setenv("BASHY_HOME", t.TempDir())
	const publicURL = "https://console.example.test/form"
	payload, err := json.Marshal(kit.AppRuntime{Addr: strings.TrimPrefix(base, "http://"), Data: a.dir, URL: publicURL})
	if err != nil {
		t.Fatal(err)
	}
	fake := filepath.Join(t.TempDir(), "bashy")
	script := "#!/bin/sh\n" +
		"[ \"$*\" = 'app show form --runtime --json' ] || exit 1\n" +
		"printf '%s\\n' \"$TEST_DESK_APP_RUNTIME\"\n"
	if err := os.WriteFile(fake, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_BIN", fake)
	t.Setenv("TEST_DESK_APP_RUNTIME", string(payload))

	var out bytes.Buffer
	if code := runCLI([]string{"add", "--id", "console-form", "--def", "../examples/contact.json"}, &out); code != 0 {
		t.Fatalf("add via runtime discovery: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), publicURL+"/?id=console-form") {
		t.Fatalf("expected console mount URL in %q", out.String())
	}
}

func TestCLIWaitResolvesCachedOwnerKeyWithoutSharedDataDir(t *testing.T) {
	for _, k := range []string{"FORM_ADDR", "FORM_DATA", "FORM_PREFIX", "FORM_PUBLIC_URL", "FORM_KEY", "BASHY_APP", "BASHY_APP_ADDR", "BASHY_APP_DATA"} {
		t.Setenv(k, "")
	}
	// Server stores items in its own temp dir; client runs with a separate BASHY_HOME
	// (reproducing direct cached-binary execution with -addr and no matching -data).
	_, srvURL := serveApp(t)
	addr := strings.TrimPrefix(srvURL, "http://")
	creatorHome := t.TempDir()
	t.Setenv("BASHY_HOME", creatorHome)

	var out bytes.Buffer
	if code := runCLI([]string{"add", "-addr", addr, "--id", "direct-wait", "--def", "../examples/contact.json", "--title", "Direct"}, &out); code != 0 {
		t.Fatalf("add: exit %d: %s", code, out.String())
	}
	complete := `{"action":"answer","id":"direct-wait","complete":true,"answers":{"name":"Ada","contact":"email","subscribe":"yes"}}`
	if code, raw := post(t, srvURL, complete, ""); code != 200 {
		t.Fatalf("answer: %d %s", code, raw)
	}

	out.Reset()
	if code := runCLI([]string{"wait", "-addr", addr, "--timeout", "2s", "--interval", "10ms", "--json", "direct-wait"}, &out); code != 0 {
		t.Fatalf("wait without FORM_KEY or shared -data failed: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), `"Ada"`) {
		t.Fatalf("wait output missing answer: %s", out.String())
	}

	// Another caller with a fresh BASHY_HOME did not create the form and is still denied.
	t.Setenv("BASHY_HOME", t.TempDir())
	out.Reset()
	if code := runCLI([]string{"wait", "-addr", addr, "--timeout", "100ms", "--interval", "10ms", "direct-wait"}, &out); code != 1 || !strings.Contains(out.String(), "Join this form") {
		t.Fatalf("unauthenticated caller must be denied: exit %d: %s", code, out.String())
	}
}

func TestCLIHelpDocumentsWaitTimeoutAndPositionalDef(t *testing.T) {
	for _, argv := range [][]string{{"-h"}, {"--help"}, {"wait", "-h"}, {"wait", "--help"}} {
		var out bytes.Buffer
		if code := runCLI(argv, &out); code != 0 {
			t.Fatalf("%v: want exit 0, got %d: %s", argv, code, out.String())
		}
		if !strings.Contains(out.String(), "--timeout") || !strings.Contains(out.String(), "5m") {
			t.Fatalf("%v: help must document --timeout and default 5m:\n%s", argv, out.String())
		}
	}

	srvURL := startServer(t)
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer
	if code := runCLI([]string{"add", "../examples/contact.json", "-addr", addr, "--id", "pos-add", "--title", "Positional"}, &out); code != 0 {
		t.Fatalf("positional add: exit %d: %s", code, out.String())
	}
	out.Reset()
	if code := runCLI([]string{"ask", "../examples/contact.json", "-addr", addr, "--id", "pos-ask"}, &out); code != 0 {
		t.Fatalf("positional ask: exit %d: %s", code, out.String())
	}
}

func TestIdempotentJoinByRedeemingParticipant(t *testing.T) {
	_, base := serveApp(t)
	var out bytes.Buffer
	addr := strings.TrimPrefix(base, "http://")
	if code := runCLI([]string{"add", "-addr", addr, "--id", "rejoin-probe", "--def", "../examples/contact.json"}, &out); code != 0 {
		t.Fatalf("add: %d %s", code, out.String())
	}
	firstLine := strings.Fields(strings.SplitN(out.String(), "\n", 2)[0])
	u, err := url.Parse(firstLine[1])
	if err != nil {
		t.Fatal(err)
	}
	invite := strings.TrimPrefix(u.Fragment, "invite=")

	// Browser 1 visits the page (getting a form-participant cookie) and redeems the invite twice (reload).
	browser1 := participant(t, base)
	for attempt := 1; attempt <= 2; attempt++ {
		code, res := participantAction(t, browser1, base, `{"action":"join","id":"rejoin-probe","invite":"`+invite+`"}`)
		if code != 200 {
			t.Fatalf("browser1 join attempt %d: want 200, got %d (%v)", attempt, code, res)
		}
	}

	// Browser 2 (different participant cookie) cannot reuse the redeemed single-use invite.
	browser2 := participant(t, base)
	code, _ := participantAction(t, browser2, base, `{"action":"join","id":"rejoin-probe","invite":"`+invite+`"}`)
	if code != 403 {
		t.Fatalf("browser2 reused invite: want 403, got %d", code)
	}
}

func TestCLIOpenFlagCreatesAndOpensInviteURL(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake browser recorder uses /bin/sh")
	}
	base := startServer(t)
	addr := strings.TrimPrefix(base, "http://")
	recFile := filepath.Join(t.TempDir(), "opened.txt")
	fakeBrowser := filepath.Join(t.TempDir(), "fake-browser.sh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> \"" + recFile + "\"\n"
	if err := os.WriteFile(fakeBrowser, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORM_BROWSER", fakeBrowser)

	var out bytes.Buffer
	if code := runCLI([]string{"add", "-addr", addr, "--id", "open-add", "--def", "../examples/contact.json", "--title", "Open test", "--open"}, &out); code != 0 {
		t.Fatalf("add --open: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "opened: "+base+"/?id=open-add#invite=") {
		t.Fatalf("add --open stdout lacks opened invite URL:\n%s", out.String())
	}

	out.Reset()
	if code := runCLI([]string{"ask", "-addr", addr, "--id", "open-ask", "--def", "../examples/contact.json", "--title", "Ask test", "--open", "--json"}, &out); code != 0 {
		t.Fatalf("ask --open --json: exit %d: %s", code, out.String())
	}
	var askObj map[string]any
	if err := json.Unmarshal(out.Bytes(), &askObj); err != nil || askObj["opened"] != true {
		t.Fatalf("ask --open --json lacks opened=true: %v %s", err, out.String())
	}

	recorded, err := os.ReadFile(recFile)
	if err != nil {
		t.Fatalf("read recorded browser opens: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(recorded)), "\n")
	if len(lines) != 2 || !strings.Contains(lines[0], "?id=open-add#invite=") || !strings.Contains(lines[1], "?id=open-ask#invite=") {
		t.Fatalf("unexpected opened URLs: %q", lines)
	}

	// Unsupported verb with --open fails closed with exit 2.
	out.Reset()
	if code := runCLI([]string{"list", "-addr", addr, "--open"}, &out); code != 2 {
		t.Fatalf("list --open: want exit 2, got %d: %s", code, out.String())
	}
}

func TestCoreDigestMatchAndMismatch(t *testing.T) {
	// 1. Live examples/desk/form shaped.json must match the committed core source.
	if err := VerifyCoreDigest("..", "../shaped.json"); err != nil {
		t.Fatalf("committed core digest mismatch: %v", err)
	}

	// 2. Copy core source into a scratch directory; verify pass, then tamper and verify refusal.
	copyRoot := t.TempDir()
	for _, rel := range []string{"go.mod", "go.sum", "shaped.json"} {
		b, err := os.ReadFile(filepath.Join("..", rel))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(copyRoot, rel), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	err := filepath.WalkDir(".", func(p string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if p != "." && strings.HasPrefix(d.Name(), ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dst := filepath.Join(copyRoot, "server", p)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		if !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCoreDigest(copyRoot, ""); err != nil {
		t.Fatalf("clean copy failed VerifyCoreDigest: %v", err)
	}

	// Tamper with server/main.go in the copy: VerifyCoreDigest and runCLI must refuse.
	tampered := filepath.Join(copyRoot, "server", "main.go")
	origMain, err := os.ReadFile(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tampered, append(origMain, []byte("\n// tampered\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyCoreDigest(copyRoot, ""); err == nil || !strings.Contains(err.Error(), "digest mismatch") {
		t.Fatalf("tampered server/main.go: want digest mismatch, got %v", err)
	}

	base := startServer(t)
	addr := strings.TrimPrefix(base, "http://")
	t.Setenv("FORM_APP_ROOT", copyRoot)
	t.Setenv("FORM_PIN_MANIFEST", filepath.Join("..", "shaped.json"))
	var out bytes.Buffer
	if code := runCLI([]string{"list", "-addr", addr}, &out); code != 1 || !strings.Contains(out.String(), "digest mismatch") {
		t.Fatalf("runCLI with tampered FORM_APP_ROOT: want exit 1 digest mismatch, got %d: %s", code, out.String())
	}
}

func TestSKILLMdMatchesEmbeddedFence(t *testing.T) {
	skillRaw, err := os.ReadFile("../SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	bshRaw, err := os.ReadFile("../form.bsh")
	if err != nil {
		t.Fatal(err)
	}
	const startMarker = "~~~skill as guide\n"
	const endMarker = "\n~~~\n"
	_, after, ok := strings.Cut(string(bshRaw), startMarker)
	if !ok {
		t.Fatal("form.bsh lacks ~~~skill as guide fence")
	}
	fenceBody, _, ok := strings.Cut(after, endMarker)
	if !ok {
		t.Fatal("form.bsh lacks closing ~~~ fence")
	}
	want := strings.TrimRight(strings.ReplaceAll(string(skillRaw), "\r\n", "\n"), "\n")
	got := strings.TrimRight(strings.ReplaceAll(fenceBody, "\r\n", "\n"), "\n")
	if got != want {
		t.Fatalf("SKILL.md and form.bsh embedded skill fence drifted")
	}
}
