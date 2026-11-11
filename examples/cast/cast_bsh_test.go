// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the cast Bash# command (Sprint 603 M2b): the script is
// driven end to end through a freshly built bashy, the way the registry runs
// it, and the cast it writes is parsed back with the package it is built on.
package castexample

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/qiangli/bashy/examples/cast/cast"
)

var (
	buildOnce sync.Once
	buildBin  string
	buildErr  error
	buildDir  string
)

func TestMain(m *testing.M) {
	var err error
	buildDir, err = os.MkdirTemp("", "bashy-cast-*")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

// moduleRoot is the bashy repository root (the directory holding cmd/bashy);
// this example is its own Go module, so the nearest go.mod is not it.
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
		t.Skip("the child commands are POSIX sh fixtures")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the Go fence needs a go toolchain on PATH")
	}
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bin := filepath.Join(buildDir, "bashy")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err, string(out)}
			return
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

// run drives cast.bsh from dir and returns stdout, stderr and the exit code.
func run(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	script := filepath.Join(moduleRoot(t), "examples", "cast", "cast.bsh")
	cmd := exec.CommandContext(ctx, bashyBin(t), append([]string{"--bashsharp", script}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "BASHY_HINTS=off", "BASHY_TELEMETRY_QUIET=1")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return out.String(), errb.String(), code
}

// A recording of colour codes and multi-byte UTF-8 (the child pauses inside a
// rune so the halves land in separate reads) parses back intact, with the
// requested geometry and monotonic times, and the command's exit status.
func TestRecordColourAndSplitUTF8(t *testing.T) {
	dir := t.TempDir()
	child := `printf '\033[32mok\033[0m \342\234'; sleep 0.15; printf '\223 \360\237'; sleep 0.15; printf '\232\200\n'; exit 5`
	_, errs, code := run(t, dir, "record", "scene.cast", "--cols", "90", "--rows", "20", "--title", "Scene", "--", "sh", "-c", child)
	if code != 5 {
		t.Fatalf("exit %d, want the child's 5; stderr: %s", code, errs)
	}
	f, err := os.Open(filepath.Join(dir, "scene.cast"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h, events, err := cast.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	if h.Version != 2 || h.Width != 90 || h.Height != 20 || h.Title != "Scene" || h.Timestamp == 0 {
		t.Errorf("header = %+v", h)
	}
	if len(events) < 2 {
		t.Fatalf("want at least two events (the rune was split by a pause), got %d", len(events))
	}
	var all strings.Builder
	prev := -1.0
	for i, ev := range events {
		if ev.Code != "o" {
			t.Errorf("event %d code %q", i, ev.Code)
		}
		if ev.Time < prev {
			t.Errorf("event %d time %f < previous %f: not monotonic", i, ev.Time, prev)
		}
		prev = ev.Time
		if strings.ContainsRune(ev.Data, '�') {
			t.Errorf("event %d carries U+FFFD: %q", i, ev.Data)
		}
		all.WriteString(ev.Data)
	}
	if got, want := all.String(), "\033[32mok\033[0m ✓ \U0001F680\r\n"; got != want {
		t.Errorf("recorded bytes = %q, want %q", got, want)
	}
}

// OUT "-" streams the cast to stdout; a relative OUT and the child's cwd are
// the invoking directory; the idle limit caps a pause.
func TestRecordStdoutCwdAndIdleLimit(t *testing.T) {
	dir := t.TempDir()
	out, errs, code := run(t, dir, "record", "-", "--cols=40", "--rows=8", "--idle-limit", "0.1", "--", "sh", "-c", "pwd; sleep 0.5; echo after")
	if code != 0 {
		t.Fatalf("exit %d; stderr: %s", code, errs)
	}
	h, events, err := cast.Parse(strings.NewReader(out))
	if err != nil {
		t.Fatalf("stdout is not a cast: %v\n%s", err, out)
	}
	if h.Width != 40 || h.Height != 8 {
		t.Errorf("header = %+v, want 40x8 from --name=value flags", h)
	}
	var all strings.Builder
	for _, ev := range events {
		all.WriteString(ev.Data)
	}
	want, _ := filepath.EvalSymlinks(dir)
	if !strings.Contains(all.String(), want) || !strings.Contains(all.String(), "after") {
		t.Errorf("recorded %q, want the cwd %q and the post-sleep line", all.String(), want)
	}
	if last := events[len(events)-1].Time; last > 0.4 {
		t.Errorf("idle limit not applied: last event at %fs", last)
	}
	if _, _, code := run(t, dir, "record", "rel.cast", "--", "sh", "-c", "exit 3"); code != 3 {
		t.Errorf("exit %d, want the child's 3", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "rel.cast")); err != nil {
		t.Errorf("relative OUT not written in the invoking directory: %v", err)
	}
}

func TestUsageHelpAndSkill(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{},
		{"play", "x.cast"},
		{"record"},
		{"record", "x.cast"},
		{"record", "x.cast", "--cols", "0", "--", "true"},
		{"record", "x.cast", "--idle-limit", "-1", "--", "true"},
		{"record", "x.cast", "--bogus", "--", "true"},
		{"record", "x.cast", "extra", "--", "true"},
	} {
		if _, _, code := run(t, dir, args...); code != 2 {
			t.Errorf("%q: exit %d, want 2", args, code)
		}
	}
	if _, errs, code := run(t, dir, "record", "x.cast", "--", "definitely-not-a-program-cast-test"); code != 127 || !strings.Contains(errs, "cast:") {
		t.Errorf("unknown program: exit %d, stderr %q; want 127 and a cast: message", code, errs)
	}
	if out, _, code := run(t, dir, "--help"); code != 0 || !strings.Contains(out, "cast record OUT.cast") || !strings.Contains(out, "--idle-limit") {
		t.Errorf("--help: %d %q", code, out)
	}
	if out, _, code := run(t, dir, "--skill"); code != 0 || !strings.HasPrefix(out, "---\nname: cast\n") {
		t.Errorf("--skill: %d %q", code, out)
	}
}

// The one file carries the whole skill and the fence it runs on.
func TestSkillFence(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(moduleRoot(t), "examples", "cast", "cast.bsh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, want := range []string{"~~~skill as guide", "name: cast", "~~~go as go", `"github.com/qiangli/bashy/examples/cast/cast"`, "cast record OUT.cast", "--cols", "--rows", "--idle-limit", "--title", "ConPTY"} {
		if !strings.Contains(body, want) {
			t.Errorf("cast.bsh missing %q", want)
		}
	}
}
