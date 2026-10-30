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
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"desk-kit/contracttest"
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
	contracttest.Run(t, contracttest.Config{BaseURL: startServer(t), Tool: "form"})
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
			id, _ := newForm(t, base, `{"action":"new","title":"s","definition":`+contactDef+`}`)
			code, raw = post(t, base, `{"action":"answer","id":"`+id+`","answers":{"api_key":"x"}}`, "")
		} else {
			code, raw = post(t, base, `{"action":"new","title":"s","definition":`+tc.def+`}`, "")
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
		code, raw := post(t, base, `{"action":"new","title":"s","definition":`+tc.def+`}`, "")
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
	id, key := newForm(t, base, `{"action":"new","title":"Contact","definition":`+contactDef+`}`)

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
	code, raw = post(t, base, `{"action":"get","id":"`+id+`"}`, key)
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
	id, _ := newForm(t, base, `{"action":"new","title":"Contact","definition":`+contactDef+`}`)
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
	newArgs := []string{"new", "-addr", addr, "--json", "--id", "cli-probe", "--title", "T",
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
	if code := runCLI([]string{"put", "-addr", addr, "--key", key, "--answer", "name=Ada", "cli-probe"}, &out); code != 0 {
		t.Fatalf("put: exit %d: %s", code, out.String())
	}
	out.Reset()
	if code := runCLI([]string{"get", "-addr", addr, "--key", key, "--json", "cli-probe"}, &out); code != 0 {
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

// Flags work after the id too (form get ID --json): argument order must
// never silently drop a flag.
func TestCLIFlagsAfterID(t *testing.T) {
	srvURL := startServer(t)
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer
	if code := runCLI([]string{"new", "-addr", addr, "--json", "--id", "order-probe",
		"--title", "T", "--def", "../examples/contact.json"}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}
	var created map[string]any
	_ = json.Unmarshal(out.Bytes(), &created)
	key, _ := created["ownerKey"].(string)
	out.Reset()
	if code := runCLI([]string{"get", "order-probe", "--key", key, "-addr", addr, "--json"}, &out); code != 0 {
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
	srvURL := startServer(t)
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer
	if code := runCLI([]string{"new", "-addr", addr, "--json", "--id", "wait-probe",
		"--def", "../examples/contact.json"}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}
	var created map[string]any
	_ = json.Unmarshal(out.Bytes(), &created)
	key, _ := created["ownerKey"].(string)

	// Pending form: wait gives up fast with a timeout, exit 1.
	out.Reset()
	if code := runCLI([]string{"wait", "-addr", addr, "--key", key,
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
	if code := runCLI([]string{"wait", "-addr", addr, "--key", key, "--json",
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
