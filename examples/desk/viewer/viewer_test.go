package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	kit "desk-kit"
	"desk-kit/contracttest"
)

func startServer(t *testing.T, breakM string) (string, string) {
	t.Helper()
	old := breakMode
	breakMode = breakM
	t.Cleanup(func() { breakMode = old })
	dir := t.TempDir()
	a := &app{dir: dir}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	a.baseURL = srv.URL
	return srv.URL, dir
}

func cliFor(base, dir string) func(...string) (int, string) {
	return func(args ...string) (int, string) {
		var out bytes.Buffer
		argv := append([]string{"--addr", strings.TrimPrefix(base, "http://"), "--dir", dir}, args...)
		code := runCLI(argv, strings.NewReader(""), &out)
		return code, out.String()
	}
}

func post(t *testing.T, base string, body any) (int, map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(body)
	r, err := http.Post(base+"/api/action", "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	var v map[string]any
	json.NewDecoder(r.Body).Decode(&v)
	return r.StatusCode, v
}

func get(t *testing.T, url string) (int, string, http.Header) {
	t.Helper()
	r, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b), r.Header
}

func errMsg(v map[string]any) string {
	e, _ := v["error"].(map[string]any)
	s, _ := e["message"].(string)
	return s
}

// --- contract -------------------------------------------------------------

func TestDeskContract(t *testing.T) {
	base, dir := startServer(t, "")
	contracttest.Run(t, contracttest.Config{BaseURL: base, Tool: tool, ViewOnly: true, CLI: cliFor(base, dir)})
}

func TestContractRedOnBreak(t *testing.T) {
	for _, b := range []string{"accept-set", "open-emits-content", "allow-secret"} {
		base, dir := startServer(t, b)
		fails := contracttest.Check(contracttest.Config{BaseURL: base, Tool: tool, ViewOnly: true, CLI: cliFor(base, dir)})
		if len(fails) == 0 {
			t.Errorf("break %q: view-only contract stayed green, want red", b)
			continue
		}
		t.Logf("break %q correctly fails %d point(s), e.g.: %s", b, len(fails), fails[0])
	}
}

// A non-view-only probe proves the variant matters: the editing contract
// (set → 200) is red against the viewer.
func TestEditingContractIsRedForViewer(t *testing.T) {
	base, _ := startServer(t, "")
	fails := contracttest.Check(contracttest.Config{BaseURL: base, Tool: tool})
	found := false
	for _, f := range fails {
		if strings.HasPrefix(f, "put:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("viewer must refuse set; editing contract failures: %v", fails)
	}
}

func TestViewOnlyVerbsRefused(t *testing.T) {
	base, dir := startServer(t, "")
	cli := cliFor(base, dir)
	if code, out := cli("add", "b1", "--body", "# hi"); code != 0 {
		t.Fatalf("add: %d %s", code, out)
	}
	for _, verb := range []string{"set", "edit", "put", "new", "get"} {
		code, out := cli(verb, "b1", "--body", "x")
		if code != 2 {
			t.Errorf("%s: want exit 2, got %d (%s)", verb, code, out)
		}
		code, v := post(t, base, map[string]any{"action": verb, "id": "b1", "body": "x"})
		if code != http.StatusBadRequest {
			t.Errorf("HTTP %s: want 400, got %d %v", verb, code, v)
		}
	}
	_, out := cli("set", "b1")
	if !strings.Contains(out, "view-only") {
		t.Errorf("set refusal must say view-only, got %q", out)
	}
	code, out := cli("show", "b1")
	if code != 0 || !strings.Contains(out, "# hi") {
		t.Errorf("content unchanged after refused edits: %d %q", code, out)
	}
}

func TestOpenPrintsIDURLStatusOnly(t *testing.T) {
	base, dir := startServer(t, "")
	cli := cliFor(base, dir)
	cli("add", "b1", "--body", "secret-ish canary body")
	code, out := cli("open", "b1")
	if code != 0 || strings.Contains(out, "canary") {
		t.Fatalf("open: %d %q", code, out)
	}
	f := strings.Fields(out)
	if len(f) != 3 || f[0] != "b1" || !strings.Contains(f[1], "?id=b1") || f[2] != kit.StatusFilled {
		t.Fatalf("open must print id url status, got %q", out)
	}
}

// --- templates: data separate from template, never HTML from the agent ---

func sampleData() map[string]string {
	return map[string]string{
		"status":       `{"title":"Sprint status","summary":"All **green**.","done":["K0"],"next":["V1"],"risks":["none"],"asks":["review"],"metrics":[{"label":"pass","value":86}]}`,
		"plan-summary": `{"goal":"Ship the *viewer*.","steps":[{"title":"Model","status":"done","owner":"v1"},{"title":"Page","detail":"Kit shell.","status":"doing"}],"risks":["time"],"decisions":["view-only"]}`,
		"recap":        `{"period":"2026-10-30","summary":"Good day.","highlights":["viewer"],"metrics":[{"label":"tests","value":20}],"links":[{"label":"repo","url":"https://example.com/x"},{"label":"bad","url":"javascript:alert(1)"}]}`,
		"close-out":    `{"outcome":"Done.","delivered":[{"item":"viewer","evidence":"tests"}],"gates":[{"name":"go test","result":"pass"},{"name":"browser","result":"skip","note":"no pw"}],"followups":["G1 docs"]}`,
	}
}

func TestShippedTemplatesRender(t *testing.T) {
	names := templateNames(t.TempDir())
	want := []string{"close-out", "plan-summary", "recap", "status"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("templates = %v, want %v", names, want)
	}
	for name, data := range sampleData() {
		md, _, err := fillTemplate(t.TempDir(), name, json.RawMessage(data), 0)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if !strings.HasPrefix(md, "# ") || strings.Contains(md, "<no value>") {
			t.Errorf("%s: unexpected markdown:\n%s", name, md)
		}
	}
	md, _, _ := fillTemplate(t.TempDir(), "recap", json.RawMessage(sampleData()["recap"]), 0)
	if !strings.Contains(md, "](<https://example.com/x>)") || strings.Contains(md, "](javascript") {
		t.Errorf("recap links: only http(s) become links:\n%s", md)
	}
	md, _, _ = fillTemplate(t.TempDir(), "status", json.RawMessage(sampleData()["status"]), 0)
	if !strings.Contains(md, "```chart widget=bar") || !strings.Contains(md, `"value":86`) {
		t.Errorf("status metrics must become a chart fence:\n%s", md)
	}
}

// hostile carries raw HTML, script, Markdown emphasis, an embed and a fence
// break-out in plain-text fields.
const hostile = "<script>alert(1)</script> **bold** ![x](leak.csv) <img src=x onerror=alert(1)>\n```\n# heading"

// escapeViolations is the neutralisation check: what in rendered HTML shows
// a data value was interpreted instead of shown as text.
func escapeViolations(page string) []string {
	var out []string
	for _, bad := range []*regexp.Regexp{
		regexp.MustCompile(`<script>alert`), regexp.MustCompile(`<[^>]*\sonerror`),
		regexp.MustCompile(`<strong>bold`), regexp.MustCompile(`(src|href)="[^"]*leak\.csv`),
		regexp.MustCompile(`<img`), regexp.MustCompile(`<h1[^>]*>heading`),
	} {
		if bad.MatchString(page) {
			out = append(out, "interpreted: "+bad.String())
		}
	}
	if !strings.Contains(page, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		out = append(out, "raw HTML in data is not shown as text")
	}
	return out
}

func appendHostile(t *testing.T, base, dir string) string {
	t.Helper()
	cli := cliFor(base, dir)
	data, _ := json.Marshal(map[string]any{
		"title":   "Report",
		"summary": "Prose <script>alert(1)</script> with `a<b` code.",
		"done":    []string{hostile},
	})
	if code, out := cli("add", "h1", "--template", "status", "--data", string(data)); code != 0 {
		t.Fatalf("add: %d %s", code, out)
	}
	_, page, _ := get(t, base+"/?id=h1")
	return page
}

func TestDataNeutralised(t *testing.T) {
	base, dir := startServer(t, "")
	page := appendHostile(t, base, dir)
	if v := escapeViolations(page); len(v) > 0 {
		t.Fatalf("data not neutralised: %v", v)
	}
	if !strings.Contains(page, "<code>a&lt;b</code>") {
		t.Errorf("prose code spans keep their literal text")
	}
	if v := contracttest.StyleViolations(page, nil); len(v) > 0 {
		t.Errorf("kit conformance on a report page: %v", v)
	}
}

func TestDataNeutralisedRedWithoutEscaping(t *testing.T) {
	base, dir := startServer(t, "no-escape")
	page := appendHostile(t, base, dir)
	v := escapeViolations(page)
	if len(v) == 0 {
		t.Fatal("no-escape: neutralisation check stayed green, want red")
	}
	// The renderer is the second layer: even unescaped, no script runs.
	if strings.Contains(page, "<script>alert") || strings.Contains(page, "onerror") {
		t.Fatal("kit renderer let raw HTML through")
	}
	t.Logf("no-escape correctly red: %v", v)
}

func badDataCases() map[string]string {
	return map[string]string{
		`{"done":["x"]}`:             "data.summary: required",
		`{"summary":42}`:             "data.summary: want a string",
		`{"summary":"ok","bogus":1}`: "data.bogus: unknown field",
		`{"summary":"ok","metrics":[{"label":"a","value":"high"}]}`: "data.metrics[0].value: want a number",
		`[1,2]`:                          "data: want an object",
		`{"summary":"ok","api_key":"x"}`: "Secret",
	}
}

func TestBadDataRejected(t *testing.T) {
	base, dir := startServer(t, "")
	cli := cliFor(base, dir)
	cli("add", "b1")
	for data, want := range badDataCases() {
		code, out := cli("append", "b1", "--template", "status", "--data", data)
		if code != 2 || !strings.Contains(out, want) {
			t.Errorf("data %s: want exit 2 with %q, got %d %q", data, want, code, out)
		}
	}
	code, out := cli("append", "b1", "--template", "status", "--data", "{not json")
	if code != 2 || !strings.Contains(out, "not valid JSON") {
		t.Errorf("malformed data: %d %q", code, out)
	}
	code, out = cli("append", "b1", "--template", "nope", "--data", "{}")
	if code != 2 || !strings.Contains(out, "unknown template") {
		t.Errorf("unknown template: %d %q", code, out)
	}
	code, out = cli("append", "b1", "--data", `{"summary":"x"}`)
	if code != 2 || !strings.Contains(out, "--template") {
		t.Errorf("data without template: %d %q", code, out)
	}
	code, out = cli("show", "b1", "--json")
	if code != 0 || !strings.Contains(out, `"pages":0`) {
		t.Errorf("rejected data must not add pages: %s", out)
	}
}

func TestBadDataRedWithoutValidation(t *testing.T) {
	base, dir := startServer(t, "no-validate")
	cli := cliFor(base, dir)
	cli("add", "b1")
	accepted := 0
	for data := range badDataCases() {
		if code, _ := cli("append", "b1", "--template", "status", "--data", data); code == 0 {
			accepted++
		}
	}
	if accepted == 0 {
		t.Fatal("no-validate: bad data still rejected; the rejection test would not detect a missing validator")
	}
}

// --- live sprint card ---------------------------------------------------

const sprintGraph = `{"t":"header","schema_version":"bashy-graph-v1","source":"sprint:537"}
{"t":"node","kind":"sprint","type":"sprint-card","attrs":{"id":537,"title":"bashy desk","column":"doing","primary_goal":"One family of tools","owner":"s537-manager"}}
{"t":"node","kind":"goal","attrs":{"id":"chart","text":"chart widgets","checked":true,"wontfix":false}}
{"t":"node","kind":"goal","attrs":{"id":"viewer","text":"viewer <b>reads</b>","checked":false,"wontfix":false}}
{"t":"node","kind":"story","attrs":{"id":"fc5033e93edd"}}
{"t":"edge","src":"sprint:537","dst":"todo:fc5033e93edd","rel":"contains"}
`

func TestStatusReadsLiveSprintCard(t *testing.T) {
	old := sprintSource
	calls := 0
	sprintSource = func(n int) (map[string]any, error) {
		calls++
		if n != 537 {
			t.Errorf("sprint source called with %d", n)
		}
		return parseSprintGraph([]byte(sprintGraph))
	}
	t.Cleanup(func() { sprintSource = old })
	base, dir := startServer(t, "")
	cli := cliFor(base, dir)
	if code, out := cli("add", "--sprint", "537"); code != 0 || !strings.HasPrefix(out, "sprint-537 ") {
		t.Fatalf("sprint book: %d %q", code, out)
	}
	// The agent writes only the narrative; the card comes from the source.
	code, out := cli("append", "sprint-537", "--template", "status", "--data", `{"summary":"Viewer landed."}`)
	if code != 0 || calls != 1 {
		t.Fatalf("append status: %d %q (calls %d)", code, out, calls)
	}
	_, page, _ := get(t, base+"/?id=sprint-537")
	for _, want := range []string{"Sprint 537 status", "bashy desk", "goals 1/2 done", "1 stories", "One family of tools", "Viewer landed.", "viewer &lt;b&gt;reads&lt;/b&gt;"} {
		if !strings.Contains(page, want) {
			t.Errorf("status page lacks %q", want)
		}
	}
	// Without a sprint the status template asks for one, clearly.
	cli("add", "plain")
	code, out = cli("append", "plain", "--template", "status", "--data", `{"summary":"x"}`)
	if code != 0 {
		t.Fatalf("status without sprint is optional-sprint: %d %q", code, out)
	}
}

// --- book navigation and change highlight ---------------------------------

func threePages(t *testing.T, base, dir string) {
	t.Helper()
	cli := cliFor(base, dir)
	for i, body := range []string{
		"# Day 1\n\nAlpha paragraph.\n\nBeta paragraph.\n\n```go\nfunc a() {\n\n}\n```\n",
		"# Day 2\n\nAlpha paragraph.\n\nBeta paragraph, revised.\n\n```go\nfunc a() {\n\n}\n```\n",
		"# Day 3\n\nAlpha paragraph.\n",
	} {
		verb := []string{"append", "session-1"}
		if i == 0 {
			verb = []string{"add", "--session", "1"}
		}
		if code, out := cli(append(verb, "--body", body)...); code != 0 {
			t.Fatalf("page %d: %d %s", i+1, code, out)
		}
	}
}

func TestBookNavigation(t *testing.T) {
	base, dir := startServer(t, "")
	cliFor(base, dir)("add", "--session", "1", "--body", "# Day 1\n")
	cliFor(base, dir)("add", "s1", "--title", "x") // a different book id
	base2 := base
	threePagesBook := "session-1"
	cli := cliFor(base2, dir)
	cli("append", threePagesBook, "--body", "# Day 2\n")
	cli("append", threePagesBook, "--body", "# Day 3\n")

	code, page, _ := get(t, base+"/?id=session-1") // default: the latest page
	if code != 200 || !strings.Contains(page, "p3 of 3") || !strings.Contains(page, `rel="prev"`) || strings.Contains(page, `rel="next"`) {
		t.Fatalf("last page nav wrong: %d", code)
	}
	_, page, _ = get(t, base+"/?id=session-1&page=p2")
	for _, want := range []string{"p2 of 3", `rel="first"`, `rel="prev"`, `rel="next"`, `rel="last"`, "page=index", `name="page"`} {
		if !strings.Contains(page, want) {
			t.Errorf("p2 lacks %q", want)
		}
	}
	_, page, _ = get(t, base+"/?id=session-1&page=first")
	if !strings.Contains(page, "p1 of 3") || strings.Contains(page, `rel="prev"`) {
		t.Errorf("first page nav wrong")
	}
	_, page, _ = get(t, base+"/?id=session-1&page=index")
	if strings.Count(page, "&amp;page=") < 3 || !strings.Contains(page, "Day 3") {
		t.Errorf("index must list every page")
	}
	if code, _, _ := get(t, base+"/?id=session-1&page=p9"); code != 404 {
		t.Errorf("jump to a missing page: want 404, got %d", code)
	}
	for _, sel := range []string{"", "&page=index", "&page=p2"} {
		_, page, _ = get(t, base+"/?id=session-1"+sel)
		if v := contracttest.StyleViolations(page, nil); len(v) > 0 {
			t.Errorf("kit conformance %q: %v", sel, v)
		}
	}
	// show --page reads one page; the JSON envelope is versioned.
	code, out := cli("--json", "show", "session-1", "--page", "2")
	if code != 0 || !strings.Contains(out, `"schema_version":"bashy-viewer-v1"`) || !strings.Contains(out, "# Day 2") || strings.Contains(out, "# Day 3") {
		t.Errorf("show --page: %d %s", code, out)
	}
}

func TestChangedSincePrevious(t *testing.T) {
	base, dir := startServer(t, "")
	threePages(t, base, dir)
	_, p1, _ := get(t, base+"/?id=session-1&page=1")
	if strings.Contains(p1, `data-changed="true"`) || strings.Contains(p1, `data-vw-changes="`) {
		t.Errorf("first page has nothing to compare")
	}
	_, p2, _ := get(t, base+"/?id=session-1&page=2")
	// Heading and the revised paragraph changed; Alpha and the fence did not.
	if n := strings.Count(p2, `data-changed="true"`); n != 2 {
		t.Errorf("p2: want 2 changed blocks, got %d", n)
	}
	if !strings.Contains(p2, `data-vw-changes="2"`) || !strings.Contains(p2, "2 of 4 blocks new or changed, 2 removed") {
		t.Errorf("p2 change banner wrong")
	}
	if strings.Contains(between(p2, "Alpha paragraph", -60), `data-changed="true"`) {
		t.Errorf("unchanged Alpha paragraph must not be highlighted")
	}
	_, p3, _ := get(t, base+"/?id=session-1&page=3")
	if !strings.Contains(p3, "1 of 2 blocks new or changed, 3 removed") {
		t.Errorf("p3 banner: removals counted")
	}
}

func between(s, marker string, back int) string {
	i := strings.Index(s, marker)
	if i < 0 {
		return ""
	}
	j := i + back
	if j < 0 {
		j = 0
	}
	return s[j:i]
}

func TestSplitBlocksKeepsFencesAndContinuations(t *testing.T) {
	md := "# T\n\npara\n\n```\na\n\nb\n```\n\n- item\n\n  continued\n- next\n"
	got := splitBlocks(md)
	if len(got) != 4 {
		t.Fatalf("want 4 blocks, got %d: %q", len(got), got)
	}
	if !strings.Contains(got[2], "a\n\nb") || !strings.Contains(got[3], "continued") {
		t.Errorf("fence/continuation split: %q", got)
	}
}

// --- export, assets -------------------------------------------------------

func TestExportSelfContained(t *testing.T) {
	base, dir := startServer(t, "")
	threePages(t, base, dir)
	cli := cliFor(base, dir)
	png := []byte("\x89PNG\r\n\x1a\nfake")
	img := filepath.Join(t.TempDir(), "shot.png")
	os.WriteFile(img, png, 0644)
	if code, out := cli("append", "session-1", "--body", "# Shot\n\n![shot](shot.png)\n", "--asset", img); code != 0 {
		t.Fatalf("append asset: %d %s", code, out)
	}
	out := filepath.Join(t.TempDir(), "book.html")
	code, msg := cli("export", "session-1", "-o", out)
	if code != 0 {
		t.Fatalf("export: %d %s", code, msg)
	}
	raw, _ := os.ReadFile(out)
	s := string(raw)
	for _, want := range []string{kit.CSS(), "Day 1", "Day 3", `href="#p2"`, "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)} {
		if !strings.Contains(s, want) {
			t.Errorf("export lacks %.40q", want)
		}
	}
	if strings.Contains(s, `src="/assets`) || strings.Contains(s, `src="http`) {
		t.Errorf("export must not load anything from the server")
	}
	if v := contracttest.StyleViolations(s, nil); len(v) > 0 {
		t.Errorf("export kit conformance: %v", v)
	}
	// One page, via the HTTP action: written under the data dir.
	code, msg = cli("--json", "export", "session-1", "--page", "2")
	if code != 0 || !strings.Contains(msg, filepath.Join(dir, "exports", "session-1-p2.html")) {
		t.Errorf("export page action: %d %s", code, msg)
	}
}

func TestAssetEmbedsAndSandbox(t *testing.T) {
	base, dir := startServer(t, "")
	cli := cliFor(base, dir)
	csv := filepath.Join(t.TempDir(), "rows.csv")
	os.WriteFile(csv, []byte("name,value\nalpha,1\nbeta,2\n"), 0644)
	svg := filepath.Join(t.TempDir(), "evil.svg")
	os.WriteFile(svg, []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`), 0644)
	if code, out := cli("add", "a1", "--body", "# Data\n\n![rows](rows.csv)\n\n![x](../../a1.json)\n", "--asset", csv, "--asset", svg); code != 0 {
		t.Fatalf("add: %d %s", code, out)
	}
	_, page, _ := get(t, base+"/?id=a1")
	if !strings.Contains(page, "<td>alpha</td>") {
		t.Errorf("csv embed must render as a table")
	}
	if strings.Contains(page, `"owner"`) {
		t.Errorf("traversal embed leaked the book file")
	}
	code, _, h := get(t, base+"/assets/a1/1/evil.svg")
	if code != 200 || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("assets must be sandboxed: %d %q", code, h.Get("Content-Security-Policy"))
	}
	for _, p := range []string{"/assets/a1/1/../../a1.json", "/assets/a1/2/rows.csv", "/assets/nope/1/rows.csv"} {
		if code, _, _ := get(t, base+p); code == 200 {
			t.Errorf("%s: want refusal, got 200", p)
		}
	}
	code, out := cli("append", "a1", "--body", "x", "--asset", filepath.Join(t.TempDir(), "missing.png"))
	if code == 0 {
		t.Errorf("missing asset file must fail: %s", out)
	}
}

func TestCloseStopsAppend(t *testing.T) {
	base, dir := startServer(t, "")
	cli := cliFor(base, dir)
	cli("add", "c1", "--body", "# one")
	if code, out := cli("close", "c1"); code != 0 || !strings.Contains(out, kit.StatusClosed) {
		t.Fatalf("close: %d %s", code, out)
	}
	if code, out := cli("append", "c1", "--body", "# two"); code != 1 || !strings.Contains(out, "closed") {
		t.Errorf("append after close: %d %s", code, out)
	}
	if code, _, _ := get(t, base+"/?id=c1"); code != 200 {
		t.Errorf("a closed book stays readable")
	}
}

func TestParseSprintGraph(t *testing.T) {
	card, err := parseSprintGraph([]byte(sprintGraph))
	if err != nil {
		t.Fatal(err)
	}
	if card["title"] != "bashy desk" || card["stories"] != float64(1) || len(card["goals"].([]any)) != 2 {
		t.Errorf("card = %v", card)
	}
	if _, err := parseSprintGraph([]byte(`{"t":"header"}`)); err == nil {
		t.Error("missing card must be an error")
	}
}

func TestShapedTemplateOverrides(t *testing.T) {
	base, dir := startServer(t, "")
	os.MkdirAll(filepath.Join(dir, "templates"), 0700)
	os.WriteFile(filepath.Join(dir, "templates", "standup.md.tmpl"), []byte("# Standup {{.who}}\n\n{{.note}}\n"), 0600)
	os.WriteFile(filepath.Join(dir, "templates", "standup.schema.json"), []byte(`{"type":"object","required":["who"],"properties":{"who":{"type":"string"},"note":{"type":"string","format":"markdown"}}}`), 0600)
	cli := cliFor(base, dir)
	cli("add", "t1")
	if code, out := cli("append", "t1", "--template", "standup", "--data", `{"who":"v1","note":"*fine*"}`); code != 0 {
		t.Fatalf("shaped template: %d %s", code, out)
	}
	_, page, _ := get(t, base+"/?id=t1")
	if !strings.Contains(page, "Standup v1") || !strings.Contains(page, "<em>fine</em>") {
		t.Errorf("shaped template not used")
	}
	if code, out := cli("append", "t1", "--template", "standup", "--data", `{}`); code != 2 || !strings.Contains(out, "data.who: required") {
		t.Errorf("shaped schema not enforced: %d %s", code, out)
	}
}

// A status report is compared with the previous status report, not with a
// note in between; a page that is entirely new is not highlighted block by
// block.
func TestComparesSameKindOfPage(t *testing.T) {
	base, dir := startServer(t, "")
	cli := cliFor(base, dir)
	cli("add", "k1", "--template", "status", "--data", `{"title":"S","summary":"First.","done":["a"]}`)
	cli("append", "k1", "--body", "# A note\n\nUnrelated.\n")
	cli("append", "k1", "--template", "status", "--data", `{"title":"S","summary":"Second.","done":["a"]}`)
	_, p2, _ := get(t, base+"/?id=k1&page=2")
	if strings.Contains(p2, `data-vw-changes="`) {
		t.Errorf("the first note has no earlier note to compare with")
	}
	_, p3, _ := get(t, base+"/?id=k1&page=3")
	if !strings.Contains(p3, `page=1">p1</a>`) || strings.Count(p3, `data-changed="true"`) != 1 {
		t.Errorf("p3 must compare with p1 and highlight only the summary")
	}
	cli("append", "k1", "--template", "status", "--data", `{"title":"Other","summary":"Third.","next":["b"]}`)
	_, p4, _ := get(t, base+"/?id=k1&page=4")
	if strings.Contains(p4, `data-changed="true"`) || !strings.Contains(p4, "4 of 4 blocks new or changed") {
		t.Errorf("an all-new page reports the count without in-place highlight")
	}
}
