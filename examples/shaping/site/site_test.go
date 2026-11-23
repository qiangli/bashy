// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the generic <owner>/site Bash# command (site.bsh) and its
// shaped-app lifecycle under `bashy shape verify` in the verifier's scrubbed
// environment.
package site

import (
	"context"
	"encoding/json"
	"html"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

func bashyBin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell test fixtures")
	}
	bin, err := exec.LookPath("bashy")
	if err != nil {
		t.Skip("bashy is not on PATH")
	}
	return bin
}

func scriptPath(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "site.bsh")
}

const fakeGo = `#!/bin/sh
echo "$*" >> "$FAKE_GO_LOG"
dir="" out=""
while [ $# -gt 0 ]; do
  case "$1" in
    -C) dir="$2"; shift 2 ;;
    -o) out="$2"; shift 2 ;;
    *) shift ;;
  esac
done
[ -n "$dir" ] && [ -n "$out" ] || { echo "fake go: unexpected args: $*" >&2; exit 1; }
case "$out" in /*) ;; *) out="$dir/$out" ;; esac
mkdir -p "$(dirname "$out")"
printf '#!/bin/sh\nfor i in 1 2 3 4 5 6; do sleep 5; done\n' > "$out"
chmod +x "$out"
`

type harness struct {
	t     *testing.T
	bin   string
	root  string
	state string
	repo  string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	root := t.TempDir()
	return &harness{
		t:     t,
		bin:   bashyBin(t),
		root:  root,
		state: filepath.Join(root, "state"),
		repo:  filepath.Join(root, "sample-site"),
	}
}

func cleanEnv() []string {
	var keep []string
	for _, kv := range os.Environ() {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if strings.HasPrefix(k, "WEAVE_") || strings.HasPrefix(k, "SITE_") ||
			strings.HasPrefix(k, "CLAUDE") || strings.HasPrefix(k, "CODEX") ||
			strings.HasPrefix(k, "OPENCODE") || strings.HasPrefix(k, "GEMINI") ||
			strings.HasPrefix(k, "ANTIGRAVITY") || strings.HasPrefix(k, "YCODE") ||
			k == "AI_AGENT" ||
			(strings.HasPrefix(k, "BASHY_") && k != "BASHY_BIN") {
			continue
		}
		keep = append(keep, kv)
	}
	return keep
}

func (h *harness) runSite(extra []string, args ...string) (string, int) {
	h.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.bin, append([]string{"--bashsharp", scriptPath(h.t)}, args...)...)
	base := []string{
		"HOME=" + filepath.Join(h.root, "home"),
		"BASHY_HINTS=off",
		"SITE_STATE=" + h.state,
	}
	if err := os.MkdirAll(filepath.Join(h.root, "home"), 0o755); err != nil {
		h.t.Fatal(err)
	}
	cmd.Env = append(cleanEnv(), append(base, extra...)...)
	cmd.Dir = h.root
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		h.t.Fatalf("run %v: %v", args, err)
	}
	return string(out), code
}

func wantContains(t *testing.T, what, got string, subs ...string) {
	t.Helper()
	for _, s := range subs {
		if !strings.Contains(got, s) {
			t.Errorf("%s missing %q:\n%s", what, s, got)
		}
	}
}

func TestInitWritesRepoSkillAndPreservesExistingFiles(t *testing.T) {
	h := newHarness(t)

	// --dry-run and --dry-run --json touch nothing on disk.
	out, code := h.runSite(nil, "init", h.repo, "--title", "Acme Studio", "--origin", "https://acme.example", "--dry-run")
	if code != 0 {
		t.Fatalf("init --dry-run: %d\n%s", code, out)
	}
	wantContains(t, "init --dry-run", out, "would create:", "acme-studio-site")
	if _, err := os.Stat(h.repo); !os.IsNotExist(err) {
		t.Fatalf("dry-run created %s", h.repo)
	}

	out, code = h.runSite(nil, "init", h.repo, "--title", "Acme Studio", "--origin", "https://acme.example", "--dry-run", "--json")
	if code != 0 {
		t.Fatalf("init --dry-run --json: %d\n%s", code, out)
	}
	var dry map[string]any
	if err := json.Unmarshal([]byte(out), &dry); err != nil || dry["dry_run"] != true || dry["skill"] != "acme-studio-site" {
		t.Fatalf("unexpected dry-run JSON: %v\n%s", err, out)
	}

	// Real init with --json.
	out, code = h.runSite(nil, "init", h.repo, "--title", "Acme Studio", "--origin", "https://acme.example/", "--json")
	if code != 0 {
		t.Fatalf("init --json: %d\n%s", code, out)
	}
	var initRes map[string]any
	if err := json.Unmarshal([]byte(out), &initRes); err != nil || initRes["ok"] != true || initRes["origin"] != "https://acme.example" {
		t.Fatalf("unexpected init JSON: %v\n%s", err, out)
	}

	for _, rel := range []string{
		"go.mod", "main.go", "render.go", "server.go", "check.go", "main_test.go",
		"standalone_deps_test.go", "shaped.json", "contract.bsh", "site.json",
		"content/home.md", "content/about.md",
		"checks/render-determinism.bsh", "checks/unique-ids.bsh",
		"checks/links-resolve.bsh", "checks/stale-base.bsh",
		"SKILL.md", "skills/acme-studio-site/SKILL.md", "bin/site.bsh",
	} {
		b, err := os.ReadFile(filepath.Join(h.repo, filepath.FromSlash(rel)))
		if err != nil || len(b) == 0 {
			t.Errorf("init missing %s: %v", rel, err)
		}
		// Nothing dhnt-specific in generated sample content or config.
		if strings.HasPrefix(rel, "content/") || rel == "site.json" || strings.HasSuffix(rel, "SKILL.md") {
			if strings.Contains(strings.ToLower(string(b)), "dhnt.io") || strings.Contains(strings.ToLower(string(b)), "dahenito") {
				t.Errorf("%s leaked dhnt-specific copy:\n%s", rel, b)
			}
		}
	}

	skillMD, _ := os.ReadFile(filepath.Join(h.repo, "skills", "acme-studio-site", "SKILL.md"))
	wantContains(t, "SKILL.md", string(skillMD),
		"name: acme-studio-site",
		"Accept: text/markdown",
		"Accept: application/json",
		"/index.md",
		"/about.json",
		`"url":`, `"section_id":`, `"revision":`, `"section_sha256":`, `"quote":`,
	)

	// Emitted SKILL.md passes `bashy skill add` and `bashy skill verify` in an isolated store.
	skillStore := filepath.Join(h.root, "skills-store")
	if err := os.MkdirAll(skillStore, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(h.bin, "skill", "add", filepath.Join(h.repo, "skills", "acme-studio-site"))
	cmd.Env = append(cleanEnv(), "HOME="+filepath.Join(h.root, "home"), "BASHY_SKILLS_DIR="+skillStore, "BASHY_HINTS=off")
	if addOut, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bashy skill add failed: %v\n%s", err, addOut)
	}
	verifyCmd := exec.Command(h.bin, "skill", "verify", "acme-studio-site")
	verifyCmd.Env = append(cleanEnv(), "HOME="+filepath.Join(h.root, "home"), "BASHY_SKILLS_DIR="+skillStore, "BASHY_HINTS=off")
	if verOut, err := verifyCmd.CombinedOutput(); err != nil || !strings.Contains(string(verOut), "valid: true") {
		t.Fatalf("bashy skill verify failed: %v\n%s", err, verOut)
	}

	// Re-init preserves existing files.
	customAbout := "---\ntitle: Custom About\npath: /about\n---\n\n## Custom {#custom}\n\nKeep me.\n"
	if err := os.WriteFile(filepath.Join(h.repo, "content", "about.md"), []byte(customAbout), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code = h.runSite(nil, "init", h.repo, "--title", "Other", "--origin", "https://other.example")
	if code != 0 {
		t.Fatalf("re-init: %d\n%s", code, out)
	}
	wantContains(t, "re-init", out, "keep: site.json (exists)", "keep: content/ (exists)")
	gotAbout, _ := os.ReadFile(filepath.Join(h.repo, "content", "about.md"))
	if string(gotAbout) != customAbout {
		t.Fatal("re-init overwrote content/about.md")
	}

	// Refuses unescaped quotes.
	if out, code := h.runSite(nil, "init", filepath.Join(h.root, "bad"), "--title", `Bad "Quote"`, "--origin", "https://bad.example"); code == 0 {
		t.Fatalf("quote accepted:\n%s", out)
	}
}

var (
	testHTMLSecRe = regexp.MustCompile(`<section\s+id="([^"]+)"`)
	testHTMLHref  = regexp.MustCompile(`<a\s+[^>]*href="([^"]+)"`)
	testMDSecRe   = regexp.MustCompile(`(?m)^## .+ \{#([a-z0-9][a-z0-9_-]*)\}$`)
	testMDLinkRe  = regexp.MustCompile(`\[[^\]]+\]\(([^)]+)\)`)
)

func TestRenderCheckAndServeVerbs(t *testing.T) {
	h := newHarness(t)
	if out, code := h.runSite(nil, "init", h.repo, "--title", "Sample Org", "--origin", "https://sample.example"); code != 0 {
		t.Fatalf("init: %d\n%s", code, out)
	}

	// render --dry-run and --dry-run --json.
	if out, code := h.runSite(nil, "render", h.repo, "--format", "md", "/about", "--dry-run"); code != 0 || !strings.Contains(out, "would run:") {
		t.Fatalf("render --dry-run: %d\n%s", code, out)
	}
	if out, code := h.runSite(nil, "render", h.repo, "/about", "--dry-run", "--json"); code != 0 || !strings.Contains(out, `"dry_run":true`) {
		t.Fatalf("render --dry-run --json: %d\n%s", code, out)
	}

	// Render all three formats on /about and verify identical section ids and links.
	htmlOut, code := h.runSite(nil, "render", h.repo, "--format", "html", "/about")
	if code != 0 {
		t.Fatalf("render html: %d\n%s", code, htmlOut)
	}
	mdOut, code := h.runSite(nil, "render", h.repo, "--format", "md", "/about")
	if code != 0 {
		t.Fatalf("render md: %d\n%s", code, mdOut)
	}
	jsonOut, code := h.runSite(nil, "render", h.repo, "--format", "json", "/about")
	if code != 0 {
		t.Fatalf("render json: %d\n%s", code, jsonOut)
	}

	var htmlIDs, mdIDs, jsonIDs []string
	for _, m := range testHTMLSecRe.FindAllStringSubmatch(htmlOut, -1) {
		htmlIDs = append(htmlIDs, m[1])
	}
	for _, m := range testMDSecRe.FindAllStringSubmatch(mdOut, -1) {
		mdIDs = append(mdIDs, m[1])
	}
	var parsed struct {
		Path     string `json:"path"`
		Revision string `json:"revision"`
		Nav      []struct {
			Href string `json:"href"`
		} `json:"nav"`
		Sections []struct {
			ID   string `json:"id"`
			HTML string `json:"html"`
		} `json:"sections"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &parsed); err != nil {
		t.Fatalf("decode render json: %v\n%s", err, jsonOut)
	}
	var jsonLinks []string
	for _, n := range parsed.Nav {
		jsonLinks = append(jsonLinks, n.Href)
	}
	for _, s := range parsed.Sections {
		jsonIDs = append(jsonIDs, s.ID)
		for _, m := range testHTMLHref.FindAllStringSubmatch(s.HTML, -1) {
			jsonLinks = append(jsonLinks, html.UnescapeString(m[1]))
		}
	}
	if !slices.Equal(htmlIDs, mdIDs) || !slices.Equal(htmlIDs, jsonIDs) {
		t.Fatalf("section ids differ: html=%v md=%v json=%v", htmlIDs, mdIDs, jsonIDs)
	}
	var htmlLinks, mdLinks []string
	for _, m := range testHTMLHref.FindAllStringSubmatch(htmlOut, -1) {
		htmlLinks = append(htmlLinks, html.UnescapeString(m[1]))
	}
	// Strip YAML front matter before extracting Markdown links.
	mdBody := mdOut
	if idx := strings.Index(mdOut[4:], "\n---"); idx >= 0 {
		mdBody = mdOut[4+idx+4:]
	}
	for _, m := range testMDLinkRe.FindAllStringSubmatch(mdBody, -1) {
		mdLinks = append(mdLinks, m[1])
	}
	if !slices.Equal(htmlLinks, mdLinks) || !slices.Equal(htmlLinks, jsonLinks) {
		t.Fatalf("links differ: html=%v md=%v json=%v", htmlLinks, mdLinks, jsonLinks)
	}

	// check (--dry-run, --json, matching --element, and stale --element).
	if out, code := h.runSite(nil, "check", h.repo, "--dry-run", "--json"); code != 0 || !strings.Contains(out, `"dry_run":true`) {
		t.Fatalf("check --dry-run --json: %d\n%s", code, out)
	}
	chkOut, code := h.runSite(nil, "check", h.repo, "--page", "/about", "--element", "section=team revision="+parsed.Revision, "--json")
	if code != 0 {
		t.Fatalf("check matching element: %d\n%s", code, chkOut)
	}
	var chkJSON map[string]any
	if err := json.Unmarshal([]byte(chkOut), &chkJSON); err != nil || chkJSON["ok"] != true {
		t.Fatalf("unexpected check json: %v\n%s", err, chkOut)
	}
	staleOut, code := h.runSite(nil, "check", h.repo, "--page", "/about", "--element", "section=team revision=deadbeef0000", "--json")
	if code == 0 || !strings.Contains(staleOut, "stale, re-read") {
		t.Fatalf("expected stale-base failure with 'stale, re-read', got exit %d:\n%s", code, staleOut)
	}

	// serve --dry-run, fakeGo serve + stop, and live loopback HTTP smoke.
	if out, code := h.runSite(nil, "serve", h.repo, "--dry-run", "--json"); code != 0 || !strings.Contains(out, `"dry_run":true`) {
		t.Fatalf("serve --dry-run --json: %d\n%s", code, out)
	}
	fakeGoPath := filepath.Join(h.root, "fake-go")
	if err := os.WriteFile(fakeGoPath, []byte(fakeGo), 0o755); err != nil {
		t.Fatal(err)
	}
	goLog := filepath.Join(h.root, "go.log")
	stubBin := filepath.Join(h.repo, ".build", "site")
	t.Cleanup(func() { _ = exec.Command("pkill", "-f", stubBin).Run() })
	srvOut, code := h.runSite([]string{"SITE_GO_BIN=" + fakeGoPath, "FAKE_GO_LOG=" + goLog}, "serve", h.repo, "--name", "stub-site", "--json")
	if code != 0 || !strings.Contains(srvOut, `"ok":true`) {
		t.Fatalf("serve via fakeGo: %d\n%s", code, srvOut)
	}
	if out, code := h.runSite(nil, "stop", "--name", "stub-site", "--json"); code != 0 || !strings.Contains(out, `"stopped":"stub-site"`) {
		t.Fatalf("stop stub-site: %d\n%s", code, out)
	}
	_ = os.Remove(stubBin)

	// Real loopback HTTP serve smoke.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	t.Cleanup(func() { h.runSite(nil, "stop", "--name", "live-smoke") })
	if out, code := h.runSite(nil, "serve", h.repo, "--name", "live-smoke", "--port", strconv.Itoa(port)); code != 0 {
		t.Fatalf("live serve: %d\n%s", code, out)
	}
	addr := "http://127.0.0.1:" + strconv.Itoa(port)
	var resp *http.Response
	for range 40 {
		resp, err = http.Get(addr + "/about.json")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not answer on %s: %v", addr, err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("ETag") != `"`+parsed.Revision+`"` || resp.Header.Get("Vary") != "Accept" {
		t.Fatalf("unexpected live response: status=%d ETag=%q Vary=%q", resp.StatusCode, resp.Header.Get("ETag"), resp.Header.Get("Vary"))
	}
}

func TestBashyShapeVerifyPassesInScrubbedEnvironmentAndRefusesStaleBase(t *testing.T) {
	h := newHarness(t)
	if out, code := h.runSite(nil, "init", h.repo, "--title", "Sample Site", "--origin", "https://sample.example"); code != 0 {
		t.Fatalf("init: %d\n%s", code, out)
	}

	// Read base revision of /about before shaping.
	jsonOut, code := h.runSite(nil, "render", h.repo, "--format", "json", "/about")
	if code != 0 {
		t.Fatalf("render /about json: %d\n%s", code, jsonOut)
	}
	var baseAbout struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &baseAbout); err != nil || baseAbout.Revision == "" {
		t.Fatalf("parse base revision: %v\n%s", err, jsonOut)
	}

	// Disposable HOME + BASHY_HOME outside any git repository so shaping
	// tracking and managed src checkouts stay completely hermetic.
	userHome := filepath.Join(h.root, "shape-user-home")
	bashyHome := filepath.Join(h.root, "shape-bashy-home")
	if err := os.MkdirAll(userHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(bashyHome, 0o755); err != nil {
		t.Fatal(err)
	}

	runBashy := func(args ...string) (string, string, int) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, h.bin, args...)
		cmd.Dir = userHome
		cmd.Env = append(cleanEnv(),
			"HOME="+userHome,
			"BASHY_HOME="+bashyHome,
			"BASHY_HINTS=off",
			// Plant a parent secret to confirm verify scrubs parent env.
			"SHAPE_PARENT_SECRET=must-never-leak",
		)
		var stdout, stderr strings.Builder
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		rc := 0
		if ee, ok := err.(*exec.ExitError); ok {
			rc = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("bashy %v: %v", args, err)
		}
		return stdout.String(), stderr.String(), rc
	}

	if out, errOut, rc := runBashy("app", "add", "sample-site", "--type", "shaped", "--source", h.repo, "--label", "Sample Site"); rc != 0 {
		t.Fatalf("app add: exit %d\n%s\n%s", rc, out, errOut)
	}

	// 1. Valid candidate with matching base revision in .wish passes `bashy shape verify`
	// in the verifier's scrubbed environment, sealing all 4 frozen checks.
	goodWS := filepath.Join(h.root, "ws-good")
	if out, rc := execCopyNoGit(h.repo, goodWS); rc != 0 {
		t.Fatalf("copy good ws: %s", out)
	}
	aboutPath := filepath.Join(goodWS, "content", "about.md")
	origAbout, err := os.ReadFile(aboutPath)
	if err != nil {
		t.Fatal(err)
	}
	updatedAbout := strings.Replace(string(origAbout), "Maintained by the site owner", "Maintained and shaped by the site owner", 1)
	if err := os.WriteFile(aboutPath, []byte(updatedAbout), 0o644); err != nil {
		t.Fatal(err)
	}
	wishProvenance := "page: /about\nelement: section=team revision=" + baseAbout.Revision + "\nrequest: Update team copy\n"
	if err := os.WriteFile(filepath.Join(goodWS, ".wish"), []byte(wishProvenance), 0o644); err != nil {
		t.Fatal(err)
	}

	wishOut, wishErr, rc := runBashy("shape", "wish", "sample-site", "Update team copy",
		"--from", goodWS, "--page", "/about", "--element", "section=team revision="+baseAbout.Revision,
		"--user", "device:owner", "--json")
	if rc != 0 {
		t.Fatalf("shape wish (good): exit %d\n%s\n%s", rc, wishOut, wishErr)
	}
	var cand struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(wishOut), &cand); err != nil || cand.ID == "" {
		t.Fatalf("parse wish json: %v\n%s", err, wishOut)
	}

	verOut, verErr, rc := runBashy("shape", "verify", "sample-site", cand.ID, "--user", "device:owner", "--json")
	if rc != 0 {
		t.Fatalf("shape verify (good) failed: exit %d\n%s\n%s", rc, verOut, verErr)
	}
	var verified struct {
		Status  string `json:"status"`
		Verdict struct {
			Pass     bool   `json:"pass"`
			Evidence string `json:"evidence"`
		} `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(verOut), &verified); err != nil || !verified.Verdict.Pass || verified.Status != "verified" {
		t.Fatalf("unexpected verify result: %v\n%s", err, verOut)
	}
	evBytes, err := os.ReadFile(verified.Verdict.Evidence)
	if err != nil {
		t.Fatalf("read sealed evidence: %v", err)
	}
	evCompact := strings.ReplaceAll(strings.ReplaceAll(string(evBytes), " ", ""), "\n", "")
	for _, chk := range []string{
		"checks/links-resolve.bsh",
		"checks/render-determinism.bsh",
		"checks/stale-base.bsh",
		"checks/unique-ids.bsh",
	} {
		want := `"name":"` + chk + `","status":"pass"`
		if !strings.Contains(evCompact, want) {
			t.Errorf("sealed evidence missing %s:\n%s", want, evBytes)
		}
	}

	// 2. Stale revision in wish is refused by the frozen stale-base check with 'stale, re-read'.
	staleWS := filepath.Join(h.root, "ws-stale")
	if out, rc := execCopyNoGit(h.repo, staleWS); rc != 0 {
		t.Fatalf("copy stale ws: %s", out)
	}
	if err := os.WriteFile(filepath.Join(staleWS, "content", "about.md"), []byte(updatedAbout), 0o644); err != nil {
		t.Fatal(err)
	}
	staleWish := "page: /about\nelement: section=team revision=000000000000\nrequest: Stale edit\n"
	if err := os.WriteFile(filepath.Join(staleWS, ".wish"), []byte(staleWish), 0o644); err != nil {
		t.Fatal(err)
	}
	staleWishOut, staleWishErr, rc := runBashy("shape", "wish", "sample-site", "Stale edit",
		"--from", staleWS, "--page", "/about", "--element", "section=team revision=000000000000",
		"--user", "device:owner", "--json")
	if rc != 0 {
		t.Fatalf("shape wish (stale): exit %d\n%s\n%s", rc, staleWishOut, staleWishErr)
	}
	var staleCand struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(staleWishOut), &staleCand); err != nil || staleCand.ID == "" {
		t.Fatalf("parse stale wish json: %v\n%s", err, staleWishOut)
	}
	staleVerOut, _, rc := runBashy("shape", "verify", "sample-site", staleCand.ID, "--user", "device:owner", "--json")
	if rc == 0 {
		t.Fatalf("expected stale-base verification failure, got pass:\n%s", staleVerOut)
	}
	var staleVerified struct {
		Verdict struct {
			Pass     bool   `json:"pass"`
			Evidence string `json:"evidence"`
		} `json:"verdict"`
	}
	if err := json.Unmarshal([]byte(staleVerOut), &staleVerified); err != nil || staleVerified.Verdict.Pass {
		t.Fatalf("parse stale verify output: %v\n%s", err, staleVerOut)
	}
	staleEv, err := os.ReadFile(staleVerified.Verdict.Evidence)
	if err != nil {
		t.Fatalf("read stale sealed evidence: %v", err)
	}
	wantContains(t, "stale evidence", string(staleEv), "checks/stale-base.bsh", "stale, re-read")
}

func execCopyNoGit(src, dst string) (string, int) {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err.Error(), 1
	}
	cmd := exec.Command("cp", "-R", src+"/.", dst+"/")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), 1
	}
	_ = os.RemoveAll(filepath.Join(dst, ".git"))
	return "", 0
}
