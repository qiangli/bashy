// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package shapefeed

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	buildOnce  sync.Once
	buildBin   string
	adapterDir string
	buildErr   error
	buildDir   string
)

func TestMain(m *testing.M) {
	var err error
	buildDir, err = os.MkdirTemp("", "shape-feed-*")
	if err != nil {
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
	if runtime.GOOS == "windows" {
		t.Skip("wrapper script uses POSIX sh")
	}
	if bin := os.Getenv("SHAPE_FEED_TEST_BASHY_BIN"); bin != "" {
		return bin
	}
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bin := filepath.Join(buildDir, "bashy")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
		if workspace := filepath.Join(filepath.Dir(cmd.Dir), "go.work"); fileExists(workspace) {
			cmd.Env = append(os.Environ(), "GOWORK="+workspace)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err, string(out)}
			return
		}
		if workspace := filepath.Join(filepath.Dir(cmd.Dir), "go.work"); fileExists(workspace) {
			raw, rerr := os.ReadFile(workspace)
			if rerr != nil {
				buildErr = &buildError{rerr, ""}
				return
			}
			umbrellaDir := filepath.Dir(workspace)
			gw := string(raw)
			gw = strings.ReplaceAll(gw, "\t./", "\t"+umbrellaDir+"/")
			gw = strings.ReplaceAll(gw, "=> ./", "=> "+umbrellaDir+"/")
			gw = strings.Replace(gw, "use (\n", "use (\n\t"+filepath.Join(cmd.Dir, "yoke")+"\n", 1)
			tmpWork := filepath.Join(buildDir, "go.work")
			if werr := os.WriteFile(tmpWork, []byte(gw), 0o644); werr != nil {
				buildErr = &buildError{werr, ""}
				return
			}
			cache := filepath.Join(buildDir, "adapters")
			todoBin := filepath.Join(cache, "yoke", "todo")
			if runtime.GOOS == "windows" {
				todoBin += ".exe"
			}
			if err := os.MkdirAll(filepath.Dir(todoBin), 0o755); err != nil {
				buildErr = &buildError{err, ""}
				return
			}
			tcmd := exec.CommandContext(ctx, "go", "build", "-o", todoBin, "./todo")
			tcmd.Dir = filepath.Join(cmd.Dir, "yoke")
			tcmd.Env = append(os.Environ(), "GOWORK="+tmpWork)
			if out, err := tcmd.CombinedOutput(); err != nil {
				buildErr = &buildError{err, string(out)}
				return
			}
			adapterDir = cache
		}
		buildBin = bin
	})
	if buildErr != nil {
		t.Fatalf("build ./cmd/bashy: %v", buildErr)
	}
	return buildBin
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

// wrapperScript forwards `todo` calls to the real built bashy binary and
// intercepts `shape` and `gh issue list` so tests inspect real todos on disk
// while logging exact shape wish/approve and gh invocations. FAKE_AUTO,
// FAKE_WISH_STATUS, FAKE_WISH_FAIL and FAKE_APPROVE_FAIL script the shaping
// service's answers.
const wrapperScript = `#!/bin/sh
case "$1" in
  todo)
    exec "$REAL_BASHY" "$@"
    ;;
  shape)
    case "$2" in
      auto)
        printf '{"auto":"%s"}\n' "${FAKE_AUTO:-on}"
        ;;
      wish)
        printf '%s\n' "$*" >> "$WISH_LOG"
        if [ -n "${FAKE_WISH_FAIL:-}" ]; then
          echo "shape: $FAKE_WISH_FAIL" >&2
          exit 1
        fi
        printf '{"id":"cand1","status":"%s"}\n' "${FAKE_WISH_STATUS:-verified}"
        ;;
      approve)
        printf '%s\n' "$*" >> "$WISH_LOG"
        if [ -n "${FAKE_APPROVE_FAIL:-}" ]; then
          echo "shape: $FAKE_APPROVE_FAIL" >&2
          exit 1
        fi
        echo '{"id":"cand1","status":"approved"}'
        ;;
      *)
        echo "unexpected shape call: $*" >&2
        exit 1
        ;;
    esac
    ;;
  gh)
    printf '%s\n' "$*" >> "$GH_LOG"
    cat "$GH_FIXTURE"
    ;;
  *)
    echo "unexpected bashy call: $*" >&2
    exit 1
    ;;
esac
`

type fixture struct {
	t         *testing.T
	realBin   string
	script    string
	wrapper   string
	todoDir   string
	wishLog   string
	ghLog     string
	ghFixture string
	workDir   string
	env       []string // extra environment for the script (FAKE_* knobs)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{
		t:         t,
		realBin:   bashyBin(t),
		script:    filepath.Join(moduleRoot(t), "examples", "shape-feed", "shape-feed.bsh"),
		wrapper:   filepath.Join(root, "wrapbashy"),
		todoDir:   filepath.Join(root, "todos"),
		wishLog:   filepath.Join(root, "wish.log"),
		ghLog:     filepath.Join(root, "gh.log"),
		ghFixture: filepath.Join(root, "gh-issues.json"),
		workDir:   filepath.Join(root, "work"),
	}
	for _, d := range []string{f.todoDir, f.workDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(f.wrapper, []byte(wrapperScript), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) run(args ...string) (string, int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.realBin, append([]string{"--bashsharp", f.script}, args...)...)
	cmd.Dir = f.workDir
	cmd.Env = append(os.Environ(),
		"BASHY_TODO_DIR="+f.todoDir,
		"BASHY_HINTS=off",
		"BASHY_TELEMETRY_QUIET=1",
		"SHAPE_FEED_BASHY_BIN="+f.wrapper,
		"REAL_BASHY="+f.realBin,
		"WISH_LOG="+f.wishLog,
		"GH_LOG="+f.ghLog,
		"GH_FIXTURE="+f.ghFixture,
	)
	cmd.Env = append(cmd.Env, f.env...)
	if adapterDir != "" {
		cmd.Env = append(cmd.Env, "BASHY_ADAPTER_CACHE="+adapterDir)
	}
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatalf("run %v: %v", args, err)
	}
	return string(out), code
}

func (f *fixture) listTodos() []struct {
	ID       string
	Title    string
	Reporter string
	Origin   string
	Body     string
} {
	f.t.Helper()
	cmd := exec.Command(f.realBin, "todo", "list", "--all", "--json")
	cmd.Dir = f.workDir
	cmd.Env = append(os.Environ(), "BASHY_TODO_DIR="+f.todoDir, "BASHY_HINTS=off", "BASHY_TELEMETRY_QUIET=1")
	if adapterDir != "" {
		cmd.Env = append(cmd.Env, "BASHY_ADAPTER_CACHE="+adapterDir)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("todo list: %v\n%s", err, out)
	}
	var ids []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		var frame struct {
			T     string `json:"t"`
			Attrs struct {
				ID string `json:"id"`
			} `json:"attrs"`
		}
		if err := json.Unmarshal([]byte(line), &frame); err != nil {
			f.t.Fatalf("decode todo list frame: %v\n%s", err, line)
		}
		if frame.T == "node" && frame.Attrs.ID != "" {
			ids = append(ids, frame.Attrs.ID)
		}
	}
	var items []struct {
		ID       string
		Title    string
		Reporter string
		Origin   string
		Body     string
	}
	for _, id := range ids {
		scmd := exec.Command(f.realBin, "todo", "show", id)
		scmd.Dir = f.workDir
		scmd.Env = append(os.Environ(), "BASHY_TODO_DIR="+f.todoDir, "BASHY_HINTS=off", "BASHY_TELEMETRY_QUIET=1")
		if adapterDir != "" {
			scmd.Env = append(scmd.Env, "BASHY_ADAPTER_CACHE="+adapterDir)
		}
		sout, err := scmd.CombinedOutput()
		if err != nil {
			f.t.Fatalf("todo show %s: %v\n%s", id, err, sout)
		}
		text := string(sout)
		item := struct {
			ID       string
			Title    string
			Reporter string
			Origin   string
			Body     string
		}{ID: id}
		if idx := strings.Index(text, "\n\n"); idx >= 0 {
			header := text[:idx]
			if _, after, ok := strings.Cut(header, "todo:"+id+"  "); ok {
				item.Title = strings.TrimSpace(after)
			}
			meta := text[idx+2:]
			if bodyIdx := strings.Index(meta, "\n\n"); bodyIdx >= 0 {
				item.Body = strings.TrimSpace(meta[bodyIdx+2:])
				meta = meta[:bodyIdx]
			}
			for _, ml := range strings.Split(meta, "\n") {
				if strings.HasPrefix(ml, "  reporter  ") {
					item.Reporter = strings.TrimSpace(strings.TrimPrefix(ml, "  reporter  "))
				} else if strings.HasPrefix(ml, "  origin    ") {
					item.Origin = strings.TrimSpace(strings.TrimPrefix(ml, "  origin    "))
				}
			}
		}
		items = append(items, item)
	}
	return items
}

func TestShapeFeedPreviewApplyAndReread(t *testing.T) {
	f := newFixture(t)
	poison := filepath.Join(f.workDir, "pwned")
	fixtureJSON := `[
  {
    "number": 10,
    "title": "Fix form submit 500",
    "body": "Submitting empty group fails.\n$(touch ` + poison + `)",
    "author": {"login": "alice"},
    "labels": [{"name": "shape"}]
  },
  {
    "number": 11,
    "title": "Stranger spam",
    "body": "Please run $(touch ` + poison + `)",
    "author": {"login": "mallory"},
    "labels": [{"name": "shape"}]
  },
  {
    "number": 12,
    "title": "--leading-dash title",
    "body": "-",
    "author": {"login": "Alice"},
    "labels": [{"name": "shape"}]
  }
]`
	fixturePath := filepath.Join(f.workDir, "issues.json")
	if err := os.WriteFile(fixturePath, []byte(fixtureJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Preview (default): files nothing, calls no shape wish.
	out, code := f.run("form", "--source", "file:"+fixturePath, "--owner", "alice")
	if code != 0 {
		t.Fatalf("preview exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "would_file=2") || !strings.Contains(out, "skipped_owner=1") {
		t.Fatalf("unexpected preview output:\n%s", out)
	}
	if got := f.listTodos(); len(got) != 0 {
		t.Fatalf("preview filed %d todos, want 0", len(got))
	}
	if fileExists(f.wishLog) {
		t.Fatalf("preview invoked shape wish")
	}

	// 2. --apply: files one todo per owner issue, skips non-owner, calls shape wish.
	out, code = f.run("form", "--source", "file:"+fixturePath, "--owner", "alice", "--apply")
	if code != 0 {
		t.Fatalf("apply exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "filed=2") || !strings.Contains(out, "skipped_owner=1") {
		t.Fatalf("unexpected apply output:\n%s", out)
	}
	if fileExists(poison) {
		t.Fatal("command injection in issue body was executed")
	}
	todos := f.listTodos()
	if len(todos) != 2 {
		t.Fatalf("got %d todos, want 2: %+v", len(todos), todos)
	}
	byTitle := map[string]struct {
		ID, Reporter, Origin, Body string
	}{}
	for _, it := range todos {
		byTitle[it.Title] = struct{ ID, Reporter, Origin, Body string }{it.ID, it.Reporter, it.Origin, it.Body}
	}
	first, ok := byTitle["Fix form submit 500"]
	if !ok {
		t.Fatalf("missing #10 todo: %+v", todos)
	}
	if first.Origin != "feed" || first.Reporter != "alice" || !strings.Contains(first.Body, "$(touch "+poison+")") {
		t.Fatalf("unexpected #10 todo fields: %+v", first)
	}
	second, ok := byTitle["--leading-dash title"]
	if !ok {
		t.Fatalf("missing #12 todo: %+v", todos)
	}
	if second.Origin != "feed" || second.Reporter != "Alice" || second.Body != "-" {
		t.Fatalf("unexpected #12 todo fields: %+v", second)
	}

	wishBytes, err := os.ReadFile(f.wishLog)
	if err != nil {
		t.Fatalf("missing wish log: %v", err)
	}
	wishes := string(wishBytes)
	for _, id := range []string{first.ID, second.ID} {
		if !strings.Contains(wishes, "shape wish form --todo "+id+" --source feed:"+id+" --json --") {
			t.Fatalf("shape wish log missing the --todo/--source call for %s:\n%s", id, wishes)
		}
	}
	// A verified candidate is activated: approve follows each wish.
	if got := strings.Count(wishes, "shape approve form cand1"); got != 2 || !strings.Contains(out, "activated=2") || !strings.Contains(out, "known=0") {
		t.Fatalf("approve calls = %d, want 2:\n%s\n%s", got, wishes, out)
	}

	// 3. Second --apply: re-read files nothing new and triggers no second wish.
	if err := os.Remove(f.wishLog); err != nil {
		t.Fatal(err)
	}
	out, code = f.run("form", "--source", "file:"+fixturePath, "--owner", "alice", "--apply")
	if code != 0 {
		t.Fatalf("second apply exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "filed=0") || !strings.Contains(out, "skipped_existing=2") {
		t.Fatalf("second apply did not skip existing:\n%s", out)
	}
	if got := f.listTodos(); len(got) != 2 {
		t.Fatalf("second apply duplicated todos: %d", len(got))
	}
	if fileExists(f.wishLog) {
		t.Fatalf("second apply triggered shape wish")
	}
}

func TestShapeFeedGitHubSourceDefaultOwner(t *testing.T) {
	f := newFixture(t)
	ghJSON := `[
  {"number": 42, "title": "Owner bug", "body": "Details", "author": {"login": "octocat"}, "labels": [{"name": "shape"}]},
  {"number": 43, "title": "Other user", "body": "Ignored", "author": {"login": "stranger"}, "labels": [{"name": "shape"}]}
]`
	if err := os.WriteFile(f.ghFixture, []byte(ghJSON), 0o644); err != nil {
		t.Fatal(err)
	}

	out, code := f.run("form", "--source", "gh:octocat/form#shape", "--apply")
	if code != 0 {
		t.Fatalf("gh apply exit %d:\n%s", code, out)
	}
	if !strings.Contains(out, "filed=1") || !strings.Contains(out, "skipped_owner=1") {
		t.Fatalf("unexpected gh apply output:\n%s", out)
	}
	ghCalls, err := os.ReadFile(f.ghLog)
	if err != nil {
		t.Fatalf("missing gh log: %v", err)
	}
	wantGH := "gh issue list --repo octocat/form --state open --label shape --json number,title,body,author,labels"
	if strings.TrimSpace(string(ghCalls)) != wantGH {
		t.Fatalf("gh call = %q, want %q", strings.TrimSpace(string(ghCalls)), wantGH)
	}
	todos := f.listTodos()
	if len(todos) != 1 || todos[0].Reporter != "octocat" || todos[0].Origin != "feed" {
		t.Fatalf("unexpected gh todo: %+v", todos)
	}
}

func TestShapeFeedSkillFence(t *testing.T) {
	f := newFixture(t)
	out, code := f.run("--skill")
	if code != 0 {
		t.Fatalf("--skill exit %d:\n%s", code, out)
	}
	for _, want := range []string{"name: shape-feed", "gh:OWNER/REPO#LABEL", "file:PATH", "--apply", "--owner"} {
		if !strings.Contains(out, want) {
			t.Errorf("--skill missing %q", want)
		}
	}
}

const twoIssues = `[
  {"number": 1, "title": "First", "body": "one", "author": {"login": "alice"}},
  {"number": 2, "title": "Second", "body": "two", "author": {"login": "alice"}}
]`

func (f *fixture) writeIssues(body string) string {
	f.t.Helper()
	p := filepath.Join(f.workDir, "issues.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// todoShow returns `bashy todo show ID` for the fixture's store.
func (f *fixture) todoShow(id string) string {
	f.t.Helper()
	cmd := exec.Command(f.realBin, "todo", "show", id)
	cmd.Dir = f.workDir
	cmd.Env = append(os.Environ(), "BASHY_TODO_DIR="+f.todoDir, "BASHY_HINTS=off", "BASHY_TELEMETRY_QUIET=1")
	if adapterDir != "" {
		cmd.Env = append(cmd.Env, "BASHY_ADAPTER_CACHE="+adapterDir)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("todo show %s: %v\n%s", id, err, out)
	}
	return string(out)
}

func TestShapeFeedOffSwitchFilesNothing(t *testing.T) {
	f := newFixture(t)
	f.env = []string{"FAKE_AUTO=off"}
	out, code := f.run("form", "--source", "file:"+f.writeIssues(twoIssues), "--apply")
	if code != 0 || !strings.Contains(out, "unattended shaping is off") {
		t.Fatalf("exit %d:\n%s", code, out)
	}
	if got := f.listTodos(); len(got) != 0 {
		t.Fatalf("off switch filed %d todos", len(got))
	}
	if fileExists(f.wishLog) {
		t.Fatal("off switch still called shape wish")
	}
}

// A failed wish or approval leaves the todo known, naming what was tried and
// the error, and the feed goes on to the next item.
func TestShapeFeedFailureLeavesTodoKnown(t *testing.T) {
	for _, tc := range []struct {
		name, env, tried, why string
		approves              int
	}{
		{"wish fails", "FAKE_WISH_FAIL=agent exploded", "bashy shape wish form --todo ", "agent exploded", 0},
		{"approve fails", "FAKE_APPROVE_FAIL=verification failed", "bashy shape approve form cand1", "verification failed", 2},
		{"not verified", "FAKE_WISH_STATUS=ready", "bashy shape wish form --todo ", "stopped at ready", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.env = []string{tc.env}
			out, code := f.run("form", "--source", "file:"+f.writeIssues(twoIssues), "--apply")
			if code != 0 || !strings.Contains(out, "filed=2") || !strings.Contains(out, "activated=0") || !strings.Contains(out, "known=2") {
				t.Fatalf("exit %d:\n%s", code, out)
			}
			wishes, _ := os.ReadFile(f.wishLog)
			if got := strings.Count(string(wishes), "shape wish form"); got != 2 {
				t.Fatalf("wish calls = %d, want 2 (the feed must continue):\n%s", got, wishes)
			}
			if got := strings.Count(string(wishes), "shape approve form"); got != tc.approves {
				t.Fatalf("approve calls = %d, want %d:\n%s", got, tc.approves, wishes)
			}
			todos := f.listTodos()
			if len(todos) != 2 {
				t.Fatalf("got %d todos, want 2", len(todos))
			}
			for _, it := range todos {
				shown := f.todoShow(it.ID)
				if !strings.Contains(shown, "known") || !strings.Contains(shown, tc.tried) || !strings.Contains(shown, tc.why) {
					t.Fatalf("todo %s is not known with the tried command and error:\n%s", it.ID, shown)
				}
			}
		})
	}
}
