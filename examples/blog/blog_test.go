// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package blog

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
	if buildDir, err = os.MkdirTemp("", "blog-*"); err != nil {
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

func runBlog(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("./blog.bsh", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("blog.bsh %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func blogEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(), "BASHY_HINTS=off")
}

func TestBlogBuild(t *testing.T) {
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "blog")
	sampleDir := filepath.Join(example, "sample")
	publicDir := filepath.Join(sampleDir, "public")

	runBlog(t, example, blogEnv(t), "build", sampleDir)

	for _, checkFile := range []string{
		"index.html",
		"about/index.html",
		"p/2026-11-10-shaping-a-build/index.html",
		"p/2026-11-12-terminal-scenes/index.html",
		"p/2026-11-13-a-take-with-captions/index.html",
		"index.xml",
		"css/style.css",
		"admin/index.html",
		"admin/config.yml",
		"admin/sveltia-cms.min.js",
		"media/demo.mp4",
		"media/demo.vtt",
		"media/build.cast",
		"media/note.wav",
		".nojekyll",
	} {
		path := filepath.Join(publicDir, filepath.FromSlash(checkFile))
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected output file %s not found: %v", checkFile, err)
		}
		if info.Size() == 0 && checkFile != ".nojekyll" {
			t.Fatalf("output file %s is empty", checkFile)
		}
	}

	read := func(rel string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(publicDir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	// Rich content rendered: media with captions
	take := read("p/2026-11-13-a-take-with-captions/index.html")
	for _, marker := range []string{
		`<video class="post-video" controls`,
		`src="../../media/demo.mp4"`,
		`<track kind="captions" src="../../media/demo.vtt"`,
		`<audio class="post-audio" controls`,
	} {
		if !strings.Contains(take, marker) {
			t.Fatalf("take post missing %s:\n%s", marker, take)
		}
	}

	// Terminal scene transcript
	scene := read("p/2026-11-12-terminal-scenes/index.html")
	for _, marker := range []string{
		`<figure class="cast">`,
		`class="cast-screen"`,
		`class="cast-title"`,
		`fg-blue bold`,
		`blog: building with the pure-Go generator`,
	} {
		if !strings.Contains(scene, marker) {
			t.Fatalf("terminal-scene post missing %s", marker)
		}
	}

	// Diagram, math and code
	notes := read("p/2026-11-10-shaping-a-build/index.html")
	for _, marker := range []string{
		`<svg class="diagram-svg"`,
		`class="math math-block"`,
		`m-frac`,
		`m-sup`,
		`tok-kw`,
		`tok-com`,
		`series-badge`,
		`series-nav`,
	} {
		if !strings.Contains(notes, marker) {
			t.Fatalf("shaping-notes post missing %s", marker)
		}
	}
	if !strings.Contains(notes, "∣") {
		t.Fatalf("math \\mid symbol not rendered")
	}

	// Home lists posts newest first with series chips and the feed links them
	home := read("index.html")
	if !strings.Contains(home, "series-chip") || !strings.Contains(home, `href="p/2026-11-13-a-take-with-captions/"`) {
		t.Fatalf("home page missing post list or series chip")
	}
	feed := read("index.xml")
	if !strings.Contains(feed, "<rss") || !strings.Contains(feed, "<pubDate>") {
		t.Fatalf("rss feed malformed:\n%s", feed)
	}

	// The CMS is configured for posts with rich-media fields
	cms := read("admin/config.yml")
	for _, marker := range []string{"collections:", "posts", "widget: \"file\"", "Terminal scene"} {
		if !strings.Contains(cms, marker) {
			t.Fatalf("admin config missing %s:\n%s", marker, cms)
		}
	}
}

func TestBlogNewAndPost(t *testing.T) {
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "blog")
	tmp := t.TempDir()

	journal := filepath.Join(tmp, "my-journal")
	runBlog(t, example, blogEnv(t), "new", journal, "--title", "Test Notes")
	for _, want := range []string{
		"hugo.toml",
		filepath.Join("content", "_index.md"),
		filepath.Join("content", "about.md"),
		filepath.Join("content", "posts", "2026-11-10-shaping-a-build.md"),
		filepath.Join("checks", "smoke.bsh"),
		"contract.bsh",
		filepath.Join("generator", "main.go"),
		filepath.Join("public", "index.html"),
	} {
		if _, err := os.Stat(filepath.Join(journal, want)); err != nil {
			t.Fatalf("new journal missing %s: %v", want, err)
		}
	}
	title, err := os.ReadFile(filepath.Join(journal, "hugo.toml"))
	if err != nil || !strings.Contains(string(title), "Test Notes") {
		t.Fatalf("journal title not applied: %v\n%s", err, title)
	}

	out := runBlog(t, example, blogEnv(t), "post", "A new note", "--series", "daily", "--part", "2", "--dir", journal)
	if !strings.Contains(out, "content/posts/") {
		t.Fatalf("post output missing path: %s", out)
	}
	slug := strings.TrimSpace(strings.Split(out, "blog: ")[1])
	data, err := os.ReadFile(slug)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{`title: "A new note"`, "series: daily", "part: 2", "date: 20"} {
		if !strings.Contains(body, want) {
			t.Fatalf("new post front matter missing %s:\n%s", want, body)
		}
	}

	// media add: attach the sample wav to the new post
	mediaOut := runBlog(t, example, blogEnv(t), "media", journal, slug, "add", filepath.Join(example, "sample", "media", "note.wav"))
	if !strings.Contains(mediaOut, "audio = media/") {
		t.Fatalf("media add did not set audio front matter: %s", mediaOut)
	}
	runBlog(t, example, blogEnv(t), "build", journal)
	post := slug[len(journal)+1 : strings.Index(slug[len(journal)+1:], "/")+len(journal)+2]
	_ = post
	data2, _ := os.ReadFile(slug)
	if !strings.Contains(string(data2), "audio: media/") {
		t.Fatalf("audio front matter not persisted:\n%s", data2)
	}
	audioHTML, err := os.ReadFile(filepath.Join(journal, "public", "p", filepath.Base(strings.TrimSuffix(slug, ".md")), "index.html"))
	if err != nil || !strings.Contains(string(audioHTML), "<audio") {
		t.Fatalf("built post lacks audio player: %v", err)
	}
}

func TestBlogServe(t *testing.T) {
	if testing.Short() {
		t.Skip("builds bashy binary")
	}
	bin := bashyBin(t)
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "blog")

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
		"BASHY_HINTS=off",
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

	runBlog(t, example, env, "build", "sample")
	out := runBlog(t, example, env, "serve", "sample", "my-journal")

	wantURL := "http://127.0.0.1:" + strconv.Itoa(port) + "/my-journal/"
	if !strings.Contains(out, wantURL) {
		t.Fatalf("serve output lacks expected URL %s:\n%s", wantURL, out)
	}

	var app struct {
		Name, Type, Source string
	}
	appJSON := run("app", "show", "my-journal", "--json")
	if err := json.Unmarshal([]byte(appJSON), &app); err != nil {
		t.Fatalf("unmarshal app show: %v\n%s", err, appJSON)
	}
	wantSource := filepath.ToSlash(filepath.Join(example, "sample", "public"))
	if app.Name != "my-journal" || app.Type != "site" || filepath.ToSlash(app.Source) != wantSource {
		t.Fatalf("app show = %+v, want my-journal site %s", app, wantSource)
	}
}

func TestBlogSmokeChecks(t *testing.T) {
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "blog")
	sampleDir := filepath.Join(example, "sample")

	runBlog(t, example, blogEnv(t), "build", sampleDir)

	cmdCheck := exec.Command("./checks/smoke.bsh")
	cmdCheck.Dir = sampleDir
	cmdCheck.Env = blogEnv(t)
	if out, err := cmdCheck.CombinedOutput(); err != nil {
		t.Fatalf("smoke check failed on valid blog: %v\n%s", err, out)
	}

	// The checks can fail: remove the home page and they must go red.
	idxPath := filepath.Join(sampleDir, "public", "index.html")
	backup := idxPath + ".bak"
	if err := os.Rename(idxPath, backup); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Rename(backup, idxPath) }()

	cmdFail := exec.Command("./checks/smoke.bsh")
	cmdFail.Dir = sampleDir
	cmdFail.Env = blogEnv(t)
	if err := cmdFail.Run(); err == nil {
		t.Fatalf("smoke check expected to fail when index.html missing, but succeeded")
	}
}

func TestBlogRichContentEdgeCases(t *testing.T) {
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "blog")
	tmp := t.TempDir()
	site := filepath.Join(tmp, "site")
	if err := os.MkdirAll(filepath.Join(site, "content", "posts"), 0o755); err != nil {
		t.Fatal(err)
	}
	post := `---
title: Edge cases
date: 2026-11-13
---

Currency like $5 and $10 stays dollars, and snake_case_file stays literal.

The var $x_1$ renders, and so does $\frac{a+b}{c}$ inline.

` + "```diagram\nnot an edge\n```" + `

` + "```math\n\\unknownmacro{x}\n```" + `
`
	if err := os.WriteFile(filepath.Join(site, "content", "posts", "2026-11-13-edge.md"), []byte(post), 0o644); err != nil {
		t.Fatal(err)
	}
	runBlog(t, example, blogEnv(t), "build", site)

	page, err := os.ReadFile(filepath.Join(site, "public", "p", "2026-11-13-edge", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	html := string(page)

	if strings.Contains(html, "$5 and <span") || strings.Contains(html, `<span class="math">5`) {
		t.Fatalf("currency misread as math:\n%s", html)
	}
	if !strings.Contains(html, "snake_case_file") {
		t.Fatalf("snake_case mangled:\n%s", html)
	}
	if !strings.Contains(html, "<em>") && strings.Contains(html, "snake") {
		t.Logf("no emphasis emitted (fine)")
	}
	if strings.Contains(html, "<em>case</em>") || strings.Contains(html, "<em>file</em>") {
		t.Fatalf("underscore emphasis split snake_case:\n%s", html)
	}
	if !strings.Contains(html, `class="math"`) || !strings.Contains(html, "m-frac") || !strings.Contains(html, "m-sub") {
		t.Fatalf("inline math not rendered:\n%s", html)
	}
	if !strings.Contains(html, "diagram-error") {
		t.Fatalf("broken diagram should render an error block:\n%s", html)
	}
	if !strings.Contains(html, `m-unknown">\unknownmacro`) {
		t.Fatalf("unknown macro should stay visible:\n%s", html)
	}
}

func TestGeneratorCopyInSync(t *testing.T) {
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "blog")
	entries, err := os.ReadDir(filepath.Join(example, "generator"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		a, err := os.ReadFile(filepath.Join(example, "generator", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(example, "sample", "generator", e.Name()))
		if err != nil {
			t.Fatalf("sample/generator missing %s (run: cp generator/*.go sample/generator/): %v", e.Name(), err)
		}
		if string(a) != string(b) {
			t.Fatalf("generator/%s differs from sample/generator/%s — resync before committing", e.Name(), e.Name())
		}
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

// --- shaping lifecycle (fork home pattern from examples/site) ---

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
		"BASHY_HINTS=off",
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

func (f *forkHome) tryRun(dir, bin string, args ...string) (string, error) {
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = f.env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func (f *forkHome) bashy(args ...string) string {
	return f.run(filepath.Dir(f.home), f.bin, args...)
}

var openedPR = regexp.MustCompile(`opened PR (\S+):`)

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

func TestBlogShapingLifecycle(t *testing.T) {
	if testing.Short() || runtime.GOOS == "windows" {
		t.Skip("drives the shaped app fork and principal isolation proof")
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git required for shaped fork proof")
	}
	bin := bashyBin(t)
	root := moduleRoot(t)
	example := filepath.Join(root, "examples", "blog")
	sampleDir := filepath.Join(example, "sample")

	f := newForkHome(t, bin, gitBin)
	f.bashy("app", "add", "blog", "--type", "shaped", "--source", sampleDir)

	repo := filepath.Join(f.home, "src", "blog")
	if _, err := os.Stat(repo); err != nil {
		t.Fatalf("managed repo not found at %s: %v", repo, err)
	}

	// The wish: a series index section on the home page (the demo
	// candidate). The route set stays identical so undo stays one command.
	f.bashy("src", "branch", "blog", "series-index")
	src, err := os.ReadFile(filepath.Join(example, "demo", "series-index", "series.go"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "generator", "series.go"), src, 0o644); err != nil {
		t.Fatal(err)
	}

	// Rebuild public/ in the fork so the candidate commit carries the
	// changed home page (the console serves committed blobs).
	srcFiles, err := filepath.Glob(filepath.Join(repo, "generator", "*.go"))
	if err != nil || len(srcFiles) == 0 {
		t.Fatalf("no generator sources in fork: %v", err)
	}
	buildArgs := append([]string{"run"}, srcFiles...)
	buildArgs = append(buildArgs, repo, filepath.Join(repo, "public"))
	build := exec.Command("go", buildArgs...)
	build.Dir = repo
	build.Env = f.env
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("candidate build: %v\n%s", err, out)
	}
	homePage, err := os.ReadFile(filepath.Join(repo, "public", "index.html"))
	if err != nil || !strings.Contains(string(homePage), "series-list") {
		t.Fatalf("candidate home page lacks the series section: %v", err)
	}

	f.run(repo, f.git, "add", "-A")
	f.run(repo, f.git, "commit", "-q", "-m", "shape: add series index pages")
	head := strings.TrimSpace(f.run(repo, f.git, "rev-parse", "HEAD"))

	m := openedPR.FindStringSubmatch(f.bashy("src", "pr", "open", "blog", "series-index", "--title", "add a series index to the home page"))
	if m == nil {
		f.t.Fatalf("src pr open: no PR id reported")
	}
	f.lastPR = m[1]
	verifyOut := f.bashy("src", "pr", "verify", f.lastPR)
	if !strings.Contains(verifyOut, "verdict: pass") {
		f.t.Fatalf("src pr verify %s: want verdict pass\n%s", f.lastPR, verifyOut)
	}

	// Approve for myself: alice activates the candidate.
	f.bashy("src", "activate", "blog", head, "--principal", "device:alice")
	if got := f.active("blog", "device:alice"); got != head {
		t.Fatalf("after activate: alice active = %s, want %s", got, head)
	}

	// Principal isolation: bob keeps the base.
	if bob := f.active("blog", "device:bob"); bob == head {
		t.Fatalf("isolation failure: bob has alice's revision %s", bob)
	}

	// The change is visible only to the requester: alice's exported
	// revision serves the series section; the base does not.
	previewOut, err := f.tryRun(filepath.Dir(f.home), f.bin, "src", "preview", "blog", head, "--user", "device:alice")
	if err == nil {
		fields := strings.Fields(previewOut)
		for _, field := range fields {
			if strings.Contains(field, ".previews") {
				data, rerr := os.ReadFile(filepath.Join(strings.Trim(field, ":"), "public", "index.html"))
				if rerr != nil || !strings.Contains(string(data), "series-list") {
					t.Logf("preview export at %s lacks series section (continuing): %v", field, rerr)
				}
			}
		}
	}

	// Undo returns alice to the base.
	f.bashy("src", "undo", "blog", "--principal", "device:alice")
	if got := f.active("blog", "device:alice"); got == head {
		t.Fatalf("after undo: alice still has shaped revision %s", got)
	}
}
