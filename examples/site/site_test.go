// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package site

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce sync.Once
	buildBin  string
	buildErr  error
	buildDir  string
)

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func TestMain(m *testing.M) {
	var err error
	if buildDir, err = os.MkdirTemp("", "site-*"); err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "bashy")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("bashy repository root (cmd/bashy) not found above test file")
		}
		dir = parent
	}
}

func bashyBin(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bin := filepath.Join(buildDir, "bashy")
		cmd := exec.CommandContext(ctx, "./scripts/go-product.sh", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err, string(out)}
			return
		}
		buildBin = bin
	})
	if buildErr != nil {
		t.Fatalf("build bashy: %v", buildErr)
	}
	return buildBin
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestSiteBuild(t *testing.T) {
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "site")
	sampleDir := filepath.Join(example, "sample")
	publicDir := filepath.Join(sampleDir, "public")

	cmd := exec.Command("./site.bsh", "build", sampleDir)
	cmd.Dir = example
	cmd.Env = append(os.Environ(), "BASHY_HINTS=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("site.bsh build failed: %v\n%s", err, string(out))
	}

	for _, checkFile := range []string{
		"index.html",
		"about/index.html",
		"services/index.html",
		"contact/index.html",
		"pricing/index.html",
		"admin/index.html",
		"admin/config.yml",
		"admin/sveltia-cms.min.js",
		"admin/editor/index.html",
		".nojekyll",
	} {
		path := filepath.Join(publicDir, checkFile)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected output file %s not found: %v", checkFile, err)
		}
		if info.Size() == 0 && checkFile != ".nojekyll" {
			t.Fatalf("output file %s is empty", checkFile)
		}
	}

	// Verify Sveltia CMS configuration
	cfgContent, err := os.ReadFile(filepath.Join(publicDir, "admin", "config.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfgContent), "collections:") {
		t.Fatalf("admin/config.yml missing collections definition:\n%s", string(cfgContent))
	}
}

func TestSiteServe(t *testing.T) {
	if testing.Short() {
		t.Skip("builds bashy binary")
	}
	bin := bashyBin(t)
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "site")

	home := t.TempDir()
	port := freePort(t)
	env := append(os.Environ(),
		"HOME="+home,
		"BASHY_HOME="+filepath.Join(home, ".bashy"),
		"XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"BASHY_COMMANDS_DIR="+filepath.Join(home, "commands"),
		"BASHY_APPS_DIR="+filepath.Join(home, "apps"),
		"BASHY_APPS_PATH=",
		"BASHY_BIN="+bin,
		"CONSOLE_PORT="+strconv.Itoa(port),
		"PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = example
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("bashy %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return string(out)
	}

	// Build sample first
	run("./site.bsh", "build", "sample")

	// Serve as app "company-site"
	cmd := exec.Command("./site.bsh", "serve", "sample", "company-site")
	cmd.Dir = example
	cmd.Env = env
	outBytes, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("site.bsh serve: %v\n%s", err, string(outBytes))
	}
	out := string(outBytes)

	wantURL := "http://127.0.0.1:" + strconv.Itoa(port) + "/company-site/"
	if !strings.Contains(out, wantURL) {
		t.Fatalf("serve output lacks expected URL %s:\n%s", wantURL, out)
	}

	// Check app registry record
	var app struct {
		Name, Type, Source string
	}
	appJSON := run("app", "show", "company-site", "--json")
	if err := json.Unmarshal([]byte(appJSON), &app); err != nil {
		t.Fatalf("unmarshal app show: %v\n%s", err, appJSON)
	}
	wantSource := filepath.ToSlash(filepath.Join(example, "sample", "public"))
	if app.Name != "company-site" || app.Type != "site" || filepath.ToSlash(app.Source) != wantSource {
		t.Fatalf("app show = %+v, want company-site site %s", app, wantSource)
	}
}

func TestSiteNew(t *testing.T) {
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "site")
	tmp := t.TempDir()

	// Hugo Template
	hugoDir := filepath.Join(tmp, "hugo-test")
	cmd := exec.Command("./site.bsh", "new", hugoDir, "--title", "Test Corp")
	cmd.Dir = example
	cmd.Env = append(os.Environ(), "BASHY_HINTS=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("site.bsh new failed: %v\n%s", err, string(out))
	}
	if _, err := os.Stat(filepath.Join(hugoDir, "hugo.toml")); err != nil {
		t.Fatalf("hugo.toml missing in new site")
	}
	if _, err := os.Stat(filepath.Join(hugoDir, "public", "index.html")); err != nil {
		t.Fatalf("public/index.html missing after site new")
	}
	if _, err := os.Stat(filepath.Join(hugoDir, "public", "admin", "sveltia-cms.min.js")); err != nil {
		t.Fatalf("public/admin/sveltia-cms.min.js missing after site new")
	}
}

func TestSiteSmokeChecks(t *testing.T) {
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "site")
	sampleDir := filepath.Join(example, "sample")

	// Ensure built
	cmdBuild := exec.Command("./site.bsh", "build", sampleDir)
	cmdBuild.Dir = example
	cmdBuild.Env = append(os.Environ(), "BASHY_HINTS=off")
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, string(out))
	}

	// Run smoke checks: must PASS
	cmdCheck := exec.Command("./checks/smoke.bsh")
	cmdCheck.Dir = sampleDir
	if out, err := cmdCheck.CombinedOutput(); err != nil {
		t.Fatalf("smoke check failed on valid site: %v\n%s", err, string(out))
	}

	// Delete index.html: must FAIL
	idxPath := filepath.Join(sampleDir, "public", "index.html")
	backup := idxPath + ".bak"
	_ = os.Rename(idxPath, backup)
	defer func() {
		_ = os.Rename(backup, idxPath)
	}()

	cmdFail := exec.Command("./checks/smoke.bsh")
	cmdFail.Dir = sampleDir
	if err := cmdFail.Run(); err == nil {
		t.Fatalf("smoke check expected to fail when index.html missing, but succeeded")
	}
}

func TestSiteShapingLifecycle(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("drives the shaped app fork and principal isolation proof")
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git required for shaped fork proof")
	}
	bin := bashyBin(t)
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "site")
	sampleDir := filepath.Join(example, "sample")

	f := newForkHome(t, bin, gitBin)
	f.bashy("app", "add", "site", "--type", "shaped", "--source", sampleDir)

	repo := filepath.Join(f.home, "src", "site")
	if _, err := os.Stat(repo); err != nil {
		t.Fatalf("managed repo not found at %s: %v", repo, err)
	}

	// Alice shapes the site by adding a personal pricing section
	aliceRevision := f.fork("site", repo, "alice-pricing-section", "add pricing section on my copy")
	f.bashy("src", "activate", "site", aliceRevision, "--principal", "device:alice")

	if got := f.active("site", "device:alice"); got != aliceRevision {
		t.Fatalf("after activate: alice active = %s, want %s", got, aliceRevision)
	}

	// Principal Isolation: Bob does NOT see Alice's revision
	bobActive := f.active("site", "device:bob")
	if bobActive == aliceRevision {
		t.Fatalf("isolation failure: bob has alice's revision %s", bobActive)
	}

	// Undo returns Alice to previous version
	f.bashy("src", "undo", "site", "--principal", "device:alice")
	if got := f.active("site", "device:alice"); got == aliceRevision {
		t.Fatalf("after undo: alice still has shaped revision %s", got)
	}
}

func TestLicensesAndStarsRecorded(t *testing.T) {
	type pick struct {
		name    string
		license string
		stars   int
	}

	picks := []pick{
		{"gohugoio/hugo", "Apache-2.0", 90461},
		{"sveltia/sveltia-cms", "MIT", 3087},
		{"measuredco/puck", "MIT", 13696},
		{"GrapesJS/grapesjs", "BSD-3-Clause", 26404},
	}

	for _, p := range picks {
		if p.stars < 1000 {
			t.Errorf("%s has %d stars, want >= 1000", p.name, p.stars)
		}
		if p.license != "Apache-2.0" && p.license != "MIT" && p.license != "BSD-3-Clause" {
			t.Errorf("%s has license %s, want permissive", p.name, p.license)
		}
	}
}

type forkHome struct {
	t      *testing.T
	bin    string
	git    string
	home   string
	env    []string
	lastPR string
}

func newForkHome(t *testing.T, bin, git string) *forkHome {
	root := t.TempDir()
	caches, err := exec.Command("go", "env", "GOMODCACHE", "GOCACHE").Output()
	if err != nil {
		t.Fatalf("go env: %v", err)
	}
	goenv := strings.Split(strings.TrimSpace(string(caches)), "\n")
	home := filepath.Join(root, "bashy")
	env := append(os.Environ(),
		"BASHY_HOME="+home,
		"HOME="+root,
		"XDG_CONFIG_HOME="+filepath.Join(root, ".config"),
		"BASHY_PRINCIPAL=device:alice",
		"BASHY_BIN="+bin,
		"GOMODCACHE="+goenv[0], "GOCACHE="+goenv[1],
		"GIT_AUTHOR_NAME=alice", "GIT_AUTHOR_EMAIL=alice@example.invalid",
		"GIT_COMMITTER_NAME=alice", "GIT_COMMITTER_EMAIL=alice@example.invalid",
		"PATH="+filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	return &forkHome{t: t, bin: bin, git: git, home: home, env: env}
}

func (f *forkHome) run(dir, bin string, args ...string) string {
	f.t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("%s %s: %v\n%s", filepath.Base(bin), strings.Join(args, " "), err, out)
	}
	return string(out)
}

func (f *forkHome) bashy(args ...string) string {
	return f.run(filepath.Dir(f.home), f.bin, args...)
}

var openedPR = regexp.MustCompile(`opened PR (\S+):`)

func (f *forkHome) fork(app, repo, branch, title string) string {
	f.t.Helper()
	f.bashy("src", "branch", app, branch)
	file := filepath.Join(repo, "FORK.txt")
	body, _ := os.ReadFile(file)
	if err := os.WriteFile(file, append(body, title+"\n"...), 0o644); err != nil {
		f.t.Fatal(err)
	}
	f.run(repo, f.git, "add", "FORK.txt")
	f.run(repo, f.git, "commit", "-q", "-m", "shape: "+title)
	head := strings.TrimSpace(f.run(repo, f.git, "rev-parse", "HEAD"))

	m := openedPR.FindStringSubmatch(f.bashy("src", "pr", "open", app, branch, "--title", title))
	if m == nil {
		f.t.Fatalf("src pr open %s: no PR id reported", branch)
	}
	f.lastPR = m[1]
	out := f.bashy("src", "pr", "verify", f.lastPR)
	if !strings.Contains(out, "verdict: pass") {
		f.t.Fatalf("src pr verify %s: want verdict pass\n%s", f.lastPR, out)
	}
	return head
}

func (f *forkHome) active(app, principal string) string {
	f.t.Helper()
	var h struct {
		Active string `json:"active"`
	}
	out := f.bashy("src", "history", app, "--user", principal, "--json")
	if err := json.Unmarshal([]byte(out), &h); err != nil {
		f.t.Fatalf("src history --json: %v\n%s", err, out)
	}
	return h.Active
}
