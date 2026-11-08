package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
	srv := httptest.NewUnstartedServer(a)
	a.baseURL = "http://" + srv.Listener.Addr().String()
	srv.Start()
	t.Cleanup(srv.Close)
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
	if _, err := exec.LookPath("bashy"); err != nil {
		t.Skip("bashy not on PATH")
	}
	raw, err := renderPDFFallback("This is a heading", "Paragraph content inside PDF")
	if err != nil {
		t.Fatalf("renderPDFFallback: %v", err)
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

func TestChartFenceRendersSVGInExport(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	var out bytes.Buffer
	md := "# Chart Note\n\n```chart widget=bar title=\"Note Chart\"\nlabel,value\nAlpha,15\nBeta,30\n```\n"
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "add", "--id", "chart-note", "--title", "Chart Note", "--body", md}, &out); code != 0 {
		t.Fatalf("add chart-note: exit %d: %s", code, out.String())
	}

	out.Reset()
	outHTML := filepath.Join(t.TempDir(), "chart-note.html")
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "export", "chart-note", "--to", "html", "-o", outHTML}, &out); code != 0 {
		t.Fatalf("export html: exit %d: %s", code, out.String())
	}
	htmlBytes, err := os.ReadFile(outHTML)
	if err != nil {
		t.Fatal(err)
	}
	exported := string(htmlBytes)
	for _, want := range []string{"<svg", `class="desk-chart-title"`, `class="desk-series-1"`} {
		if !strings.Contains(exported, want) {
			t.Errorf("exported HTML lacks %q:\n%s", want, exported)
		}
	}
	if strings.Contains(exported, "chart renderer unavailable") {
		t.Errorf("exported HTML contains fallback error text")
	}
}

func TestExportPDFBrowserAndFallback(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	var out bytes.Buffer
	richBody := "# Rich Note — Café 100%\n\n| Item | Score |\n| --- | --- |\n| UTF-8 | 100 |\n\n```chart widget=bar\nname,value\nA,10\nB,25\n```\n"
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "add", "--id", "rich-pdf-note", "--title", "Rich PDF Note", "--body", richBody}, &out); code != 0 {
		t.Fatalf("add rich-pdf-note: exit %d: %s", code, out.String())
	}

	// 1. Default/browser PDF export
	out.Reset()
	browserPDF := filepath.Join(t.TempDir(), "browser.pdf")
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "export", "rich-pdf-note", "--to", "pdf", "-o", browserPDF}, &out); code != 0 {
		t.Fatalf("export pdf (default/browser): exit %d: %s", code, out.String())
	}
	bData, err := os.ReadFile(browserPDF)
	if err != nil || !bytes.HasPrefix(bData, []byte("%PDF-")) {
		t.Fatalf("default/browser PDF output invalid: err=%v, len=%d", err, len(bData))
	}

	// 2. Pure-Go fallback when CHROMIUM_PATH=/nonexistent
	t.Setenv("CHROMIUM_PATH", "/nonexistent")
	out.Reset()
	fallbackPDF := filepath.Join(t.TempDir(), "fallback.pdf")
	if code := runCLI([]string{"-addr", addr, "-data", dataDir, "export", "rich-pdf-note", "--to", "pdf", "-o", fallbackPDF}, &out); code != 0 {
		t.Fatalf("export pdf (fallback): exit %d: %s", code, out.String())
	}
	fData, err := os.ReadFile(fallbackPDF)
	if err != nil || !bytes.HasPrefix(fData, []byte("%PDF-")) {
		t.Fatalf("fallback PDF output invalid: err=%v, len=%d", err, len(fData))
	}
}

func TestNotepadSandboxE2E(t *testing.T) {
	srvURL, dataDir := startServer(t, "")
	addr := strings.TrimPrefix(srvURL, "http://")

	sb := t.TempDir()
	home := filepath.Join(sb, "home")
	xdgCache := filepath.Join(sb, "cache")
	xdgConfig := filepath.Join(sb, "config")
	xdgData := filepath.Join(sb, "data")
	bashyHome := filepath.Join(sb, "bashy")
	for _, d := range []string{home, xdgCache, xdgConfig, xdgData, bashyHome} {
		if err := os.MkdirAll(d, 0700); err != nil {
			t.Fatal(err)
		}
	}

	binDir := filepath.Join(xdgCache, "bashy", "desk-notepad")
	if err := os.MkdirAll(binDir, 0700); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(binDir, "notepad")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	buildCmd.Env = append(os.Environ(), "GOWORK=off", "CGO_ENABLED=0")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("go build notepad: %v: %s", err, out)
	}

	shell := "bash"
	if p, err := exec.LookPath("bashy"); err == nil {
		shell = p
	}
	runBsh := func(extraEnv []string, args ...string) string {
		t.Helper()
		argv := append([]string{"./notepad.bsh"}, args...)
		cmd := exec.Command(shell, argv...)
		cmd.Env = append(os.Environ(),
			"HOME="+home,
			"BASHY_HOME="+bashyHome,
			"XDG_CACHE_HOME="+xdgCache,
			"XDG_CONFIG_HOME="+xdgConfig,
			"XDG_DATA_HOME="+xdgData,
			"GIT_AUTHOR_EMAIL=alice@example.test",
			"GIT_COMMITTER_EMAIL=alice@example.test",
			"NOTEPAD_ADDR="+addr,
			"NOTEPAD_DATA="+dataDir,
		)
		cmd.Env = append(cmd.Env, extraEnv...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("notepad.bsh %v failed: %v\n%s", args, err, out)
		}
		return string(out)
	}

	addOut := runBsh(nil, "add", "--id", "sb-note", "--title", "Sandbox Note", "--body", "Initial sandbox body")
	if !strings.Contains(addOut, "sb-note") {
		t.Fatalf("unexpected add output: %s", addOut)
	}

	updatedBody := "# Updated Sandbox Note\n\nUpdated body text.\n\n```chart widget=bar\nlabel,value\nX,5\nY,15\n```\n"
	runBsh(nil, "set", "sb-note", "--title", "Updated Sandbox Note", "--body", updatedBody)

	showOut := runBsh(nil, "show", "sb-note")
	if !strings.Contains(showOut, "Updated Sandbox Note") || !strings.Contains(showOut, "Updated body text.") {
		t.Fatalf("unexpected show output: %s", showOut)
	}

	htmlOut := filepath.Join(sb, "sb-note.html")
	runBsh(nil, "export", "sb-note", "--to", "html", "-o", htmlOut)
	htmlBytes, err := os.ReadFile(htmlOut)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(htmlBytes), "<svg") || !strings.Contains(string(htmlBytes), "Updated body text.") {
		t.Fatalf("sandbox HTML export missing chart SVG or updated body")
	}

	pdfOut := filepath.Join(sb, "sb-note.pdf")
	runBsh([]string{"CHROMIUM_PATH=/nonexistent"}, "export", "sb-note", "--to", "pdf", "-o", pdfOut)
	pdfBytes, err := os.ReadFile(pdfOut)
	if err != nil || !bytes.HasPrefix(pdfBytes, []byte("%PDF-")) {
		t.Fatalf("sandbox PDF export invalid: err=%v, len=%d", err, len(pdfBytes))
	}
}
