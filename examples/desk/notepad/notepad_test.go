package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"desk-kit/contracttest"
)

func startServer(t *testing.T, breakM string) (string, string) {
	t.Helper()
	old := breakMode
	breakMode = breakM
	t.Cleanup(func() { breakMode = old })
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "items"), 0700); err != nil {
		t.Fatal(err)
	}
	a := &app{
		dir:   dir,
		notes: map[string]*note{},
	}
	srv := httptest.NewServer(a)
	t.Cleanup(srv.Close)
	a.baseURL = srv.URL
	return srv.URL, dir
}

func TestDeskContract(t *testing.T) {
	url, _ := startServer(t, "")
	contracttest.Run(t, contracttest.Config{BaseURL: url, Tool: "notepad", CLI: deskCLICallback(url)})
}

func TestContractRedOnBreak(t *testing.T) {
	for _, breakM := range []string{"open-emits-content", "no-csp", "allow-secret"} {
		url, _ := startServer(t, breakM)
		fails := contracttest.Check(contracttest.Config{BaseURL: url, Tool: "notepad"})
		if len(fails) == 0 {
			t.Errorf("break %q: contract test stayed green, want red", breakM)
			continue
		}
		t.Logf("break %q correctly fails %d contract point(s), e.g.: %s", breakM, len(fails), fails[0])
	}
}

func TestContractRedOnNonLoopbackURL(t *testing.T) {
	for _, base := range []string{"http://10.9.9.9:18095", "http://example.test/", "http://0.0.0.0:18095/"} {
		if v := contracttest.LoopbackViolation(base); !strings.Contains(v, "loopback") {
			t.Errorf("BaseURL %q: want a loopback violation, got %q", base, v)
		}
	}
	url, _ := startServer(t, "")
	if v := contracttest.LoopbackViolation(url); v != "" {
		t.Errorf("loopback server URL: want no violation, got %q", v)
	}
}

func TestConvertTextAndMarkdown(t *testing.T) {
	txt := "Hello plain text world\nSecond line"
	conv, fmtDetected, err := ConvertContent([]byte(txt), "test.txt")
	if err != nil {
		t.Fatalf("convert txt: %v", err)
	}
	if fmtDetected != FormatText || conv != txt {
		t.Errorf("txt: got %q (%s), want %q", conv, fmtDetected, txt)
	}

	md := "# Heading\n\n- item 1\n- item 2"
	conv, fmtDetected, err = ConvertContent([]byte(md), "test.md")
	if err != nil {
		t.Fatalf("convert md: %v", err)
	}
	if fmtDetected != FormatMD || conv != md {
		t.Errorf("md: got %q (%s), want %q", conv, fmtDetected, md)
	}
}

func TestConvertHTML(t *testing.T) {
	htmlInput := "<h1>Document Title</h1><p>This is a paragraph with <a href=\"https://example.com\">a link</a>.</p>"
	conv, fmtDetected, err := ConvertContent([]byte(htmlInput), "page.html")
	if err != nil {
		t.Fatalf("convert html: %v", err)
	}
	if fmtDetected != FormatHTML {
		t.Errorf("html format: got %s, want %s", fmtDetected, FormatHTML)
	}
	if !strings.Contains(conv, "# Document Title") || !strings.Contains(conv, "[a link](https://example.com)") {
		t.Errorf("html conversion result unexpected:\n%s", conv)
	}
}

func TestConvertJSON(t *testing.T) {
	uglyJSON := `{"name":"alice","age":30,"roles":["admin","editor"]}`
	conv, fmtDetected, err := ConvertContent([]byte(uglyJSON), "data.json")
	if err != nil {
		t.Fatalf("convert json: %v", err)
	}
	if fmtDetected != FormatJSON {
		t.Errorf("json format: got %s, want %s", fmtDetected, FormatJSON)
	}
	if !strings.Contains(conv, "  \"name\": \"alice\"") || !strings.Contains(conv, "\n") {
		t.Errorf("json pretty formatting unexpected:\n%s", conv)
	}
}

func TestConvertPDF(t *testing.T) {
	pdfSamplePath := "/Users/qiangli/go/pkg/mod/github.com/ledongthuc/pdf@v0.0.0-20250511090121-5959a4027728/examples/read_plain_text/pdf_test.pdf"
	raw, err := os.ReadFile(pdfSamplePath)
	if err != nil {
		t.Skipf("sample pdf not available at %s", pdfSamplePath)
	}
	conv, fmtDetected, err := ConvertContent(raw, "doc.pdf")
	if err != nil {
		t.Fatalf("convert pdf: %v", err)
	}
	if fmtDetected != FormatPDF {
		t.Errorf("pdf format: got %s, want %s", fmtDetected, FormatPDF)
	}
	if !strings.Contains(conv, "This is a heading") {
		t.Errorf("pdf conversion result missing text:\n%s", conv)
	}
}

func TestDetectFormatSniffing(t *testing.T) {
	if f := DetectFormat([]byte("%PDF-1.4 ..."), ""); f != FormatPDF {
		t.Errorf("sniff pdf: got %s, want %s", f, FormatPDF)
	}
	if f := DetectFormat([]byte(`{"k":"v"}`), ""); f != FormatJSON {
		t.Errorf("sniff json: got %s, want %s", f, FormatJSON)
	}
	if f := DetectFormat([]byte(`<!DOCTYPE html><html><body><p>Hello</p></body></html>`), ""); f != FormatHTML {
		t.Errorf("sniff html: got %s, want %s", f, FormatHTML)
	}
	if f := DetectFormat([]byte(`Just some words without html`), ""); f != FormatText {
		t.Errorf("sniff text: got %s, want %s", f, FormatText)
	}
}

func TestCLIOpenHidesContent(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")
	var out bytes.Buffer

	// Create note via CLI
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "--json", "--id", "note-secret-probe", "--title", "T", "--body", "secret-canary-content", "add"}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}
	var created map[string]any
	if err := json.Unmarshal(out.Bytes(), &created); err != nil {
		t.Fatalf("new: not JSON: %s", out.String())
	}
	if strings.Contains(out.String(), "secret-canary-content") {
		t.Fatalf("new re-emits content: %s", out.String())
	}

	// open hides content
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "open", "note-secret-probe"}, &out); code != 0 {
		t.Fatalf("open: exit %d: %s", code, out.String())
	}
	if strings.Contains(out.String(), "secret-canary-content") {
		t.Errorf("open re-emits content: %s", out.String())
	}
	if !strings.Contains(out.String(), "note-secret-probe") {
		t.Errorf("open lacks the id: %s", out.String())
	}

	// get returns content
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "show", "note-secret-probe"}, &out); code != 0 {
		t.Fatalf("get: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "secret-canary-content") {
		t.Errorf("get must return content: %s", out.String())
	}
}

func TestCLIPutFileConvertsContent(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	// Create note
	var out bytes.Buffer
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "--id", "import-note", "add"}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}

	// Put HTML file
	tmpHTML := filepath.Join(t.TempDir(), "test.html")
	if err := os.WriteFile(tmpHTML, []byte("<h2>Imported Subtitle</h2><p>Converted paragraph</p>"), 0644); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "set", "import-note", "-f", tmpHTML}, &out); code != 0 {
		t.Fatalf("put: exit %d: %s", code, out.String())
	}
	// Verify open hides content
	if strings.Contains(out.String(), "Imported Subtitle") {
		t.Errorf("put re-emits content: %s", out.String())
	}

	// Verify get returns markdown converted text
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "show", "import-note"}, &out); code != 0 {
		t.Fatalf("get: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "## Imported Subtitle") || !strings.Contains(out.String(), "Converted paragraph") {
		t.Errorf("get did not return converted markdown: %s", out.String())
	}

	// Put JSON file
	tmpJSON := filepath.Join(t.TempDir(), "test.json")
	if err := os.WriteFile(tmpJSON, []byte(`{"message":"hello world"}`), 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "set", "import-note", "-f", tmpJSON}, &out); code != 0 {
		t.Fatalf("put json: exit %d: %s", code, out.String())
	}
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "show", "import-note"}, &out); code != 0 {
		t.Fatalf("get: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "  \"message\": \"hello world\"") {
		t.Errorf("get did not return pretty json: %s", out.String())
	}
}

func TestCLIShareCloseAndConflict(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	var out bytes.Buffer
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "--id", "lifecycle-note", "add"}, &out); code != 0 {
		t.Fatalf("new: exit %d: %s", code, out.String())
	}

	// Share
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "--json", "share", "lifecycle-note"}, &out); code != 0 {
		t.Fatalf("share: exit %d: %s", code, out.String())
	}
	var shareObj map[string]any
	if err := json.Unmarshal(out.Bytes(), &shareObj); err != nil || shareObj["invite"] == nil {
		t.Fatalf("share invalid response: %s", out.String())
	}

	// Close
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "--json", "close", "lifecycle-note"}, &out); code != 0 {
		t.Fatalf("close: exit %d: %s", code, out.String())
	}
	var closeObj map[string]any
	if err := json.Unmarshal(out.Bytes(), &closeObj); err != nil || closeObj["status"] != "closed" {
		t.Fatalf("close invalid response: %s", out.String())
	}

	// Put after close must fail with conflict (exit 1)
	out.Reset()
	code := runCLI([]string{"-addr", addr, "-data", dataDir, "set", "lifecycle-note", "--body", "new body"}, &out)
	if code == 0 {
		t.Errorf("put after close should fail, got code 0: %s", out.String())
	}
	if !strings.Contains(out.String(), "closed") {
		t.Errorf("put after close error message: %s", out.String())
	}
}

func TestCLINewWithFileAndStdin(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	// 1. new with -f FILE
	tmpMD := filepath.Join(t.TempDir(), "meeting-notes.md")
	if err := os.WriteFile(tmpMD, []byte("# Sprint 537\n\nDiscussion notes"), 0644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "add", "--id", "note-from-file", "-f", tmpMD}, &out); code != 0 {
		t.Fatalf("new -f file: exit %d: %s", code, out.String())
	}
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "show", "note-from-file"}, &out); code != 0 {
		t.Fatalf("get: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "meeting-notes") || !strings.Contains(out.String(), "Sprint 537") {
		t.Errorf("get note-from-file content: %s", out.String())
	}

	// 2. put with -f - (stdin)
	inReader := strings.NewReader("<h3>Piped Content</h3><p>Piped paragraph</p>")
	out.Reset()
	if code := runCLIWithIO([]string{"-addr", addr, "-data", dataDir, "set", "note-from-file", "-f", "-", "--format", "html"}, inReader, &out); code != 0 {
		t.Fatalf("put -f -: exit %d: %s", code, out.String())
	}
	out.Reset()
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "show", "note-from-file"}, &out); code != 0 {
		t.Fatalf("get: exit %d: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "### Piped Content") || !strings.Contains(out.String(), "Piped paragraph") {
		t.Errorf("get note-from-file after stdin put: %s", out.String())
	}
}

func TestAPIConvertEndpoint(t *testing.T) {
	srvURL, _ := startServer(t, "")
	reqBody := `{"action":"convert","body":"<h1>Header</h1>","format":"html"}`
	res, err := http.Post(srvURL+"/api/action", "application/json", strings.NewReader(reqBody))
	if err != nil {
		t.Fatalf("POST /api/action convert: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		t.Fatalf("convert status: %d", res.StatusCode)
	}
	var resp map[string]any
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		t.Fatalf("decode convert: %v", err)
	}
	if text, ok := resp["text"].(string); !ok || !strings.Contains(text, "# Header") {
		t.Errorf("convert text: %v", resp["text"])
	}
}

// TestRenderEndpoint proves deskRender's server half: the mounted POST
// /api/render returns the shared renderer's sanitized HTML — script
// stripped, Markdown rendered, chart fences drawn with the C1 renderer.
func TestRenderEndpoint(t *testing.T) {
	srvURL, _ := startServer(t, "")
	body, _ := json.Marshal(map[string]string{"text": "# Preview\n\n<script>alert(1)</script>\n\n```chart widget=bar\nname,value\na,1\nb,2\n```\n"})
	req, err := http.NewRequest("POST", srvURL+"/api/render", bytes.NewReader(body))
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
	for _, want := range []string{`<div class="desk-content">`, "<h1", "<svg", `class="desk-chart-title"`, `class="desk-series-1"`} {
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
	get, err := http.Get(srvURL + "/api/render")
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	if get.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/render: got %d, want 405", get.StatusCode)
	}
}

// TestPageLayoutToggle proves the edit/split/view switcher: three mode
// buttons and both panes, so the preview toggle the live renderer feeds
// is always present.
func TestPageLayoutToggle(t *testing.T) {
	srvURL, _ := startServer(t, "")
	res, err := http.Get(srvURL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	page := string(raw)
	for _, want := range []string{`id="modeEdit"`, `id="modeSplit"`, `id="modeView"`, `id="npEdit"`, `id="npView"`, "notepad-mode", "data-desk-render"} {
		if !strings.Contains(page, want) {
			t.Errorf("page lacks layout toggle marker %q", want)
		}
	}
}

func TestPageScriptNoPromptOrLocationSearch(t *testing.T) {
	srvURL, _ := startServer(t, "")
	res, err := http.Get(srvURL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatalf("reading body: %v", err)
	}
	htmlStr := string(body)
	if strings.Contains(htmlStr, "prompt(") {
		t.Errorf("page script contains blocking prompt(, want auto-generated id")
	}
	if strings.Contains(htmlStr, "window.location.search") {
		t.Errorf("page script contains window.location.search assignment, want history.replaceState without reload")
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
