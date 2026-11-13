// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the clip Bash# command: the script is driven end to
// end through a freshly built bashy, the way the registry runs it. The
// render path is exercised with a stub ffmpeg on PATH, so the test needs
// no real media tools. It runs from the bashy repository checkout; in a
// vendored clone (the shaping verifier's isolated app source) there is no
// repo root to build bashy from, and the test skips with the reason.
package clipexample

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
)

var (
	buildOnce sync.Once
	buildBin  string
	buildErr  error
	buildDir  string
)

func TestMain(m *testing.M) {
	var err error
	buildDir, err = os.MkdirTemp("", "bashy-clip-*")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

// moduleRoot is the bashy repository root (cmd/bashy); this example is its
// own Go module, so the nearest go.mod is not it.
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
			t.Skip("bashy repository root not found (vendored clone); the script test runs from the checkout")
		}
		dir = parent
	}
}

func bashyBin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the stub ffmpeg fixture is a POSIX sh script")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("the build needs a go toolchain on PATH")
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

// run drives clip.bsh from dir with PATH prepended and returns stdout,
// stderr and the exit code.
func run(t *testing.T, dir, path string, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	script := filepath.Join(moduleRoot(t), "examples", "clip", "clip.bsh")
	cmd := exec.CommandContext(ctx, bashyBin(t), append([]string{"--bashsharp", script}, args...)...)
	cmd.Dir = dir
	env := os.Environ()
	env = append(env, "BASHY_HINTS=off", "BASHY_TELEMETRY_QUIET=1")
	if path != "" {
		env = append(env, "PATH="+path+string(os.PathListSeparator)+os.Getenv("PATH"))
	}
	cmd.Env = env
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

// stubTools writes an ffmpeg stub that logs its argv and creates the
// output file (the last .mp4/.gif argument), so queue → work → take runs
// the whole async path without real media tools.
func stubTools(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	stub := "#!/bin/sh\nout=\nfor a in \"$@\"; do case \"$a\" in *.mp4|*.gif) out=$a;; esac; done\n" +
		"printf 'stub ffmpeg %s\\n' \"$*\" >&2\n[ -z \"$out\" ] || printf x >\"$out\"\nexit 0\n"
	if err := os.WriteFile(filepath.Join(dir, "ffmpeg"), []byte(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func touchVideo(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte("fake video"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// The embedded skill and the script's help are the two faces agents read.
func TestSkillAndHelp(t *testing.T) {
	out, _, code := run(t, t.TempDir(), "", "--skill")
	if code != 0 || !strings.HasPrefix(out, "---\nname: clips\n") {
		t.Fatalf("--skill = %d %.80s", code, out)
	}
	if !strings.Contains(out, "clips work") {
		t.Fatal("skill does not teach the work verb")
	}
	out, _, code = run(t, t.TempDir(), "", "--help")
	if code != 0 || !strings.Contains(out, "clips") {
		t.Fatalf("--help = %d %s", code, out)
	}
}

// Queue returns at once with a job id; status shows it waiting; work
// renders it with the stubbed tool; take prints the artifact path and
// exits 0, while a still-queued job takes exit 3.
func TestQueueWorkTake(t *testing.T) {
	dir := t.TempDir()
	stub := stubTools(t)
	vid := touchVideo(t, "take.mp4")

	out, errs, code := run(t, dir, stub, "queue", "--from", vid, "--start", "2", "--end", "5", "--name", "intro", "--data", dir)
	if code != 0 {
		t.Fatalf("queue = %d %s %s", code, out, errs)
	}
	id := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
	if !strings.HasPrefix(id, "c") || len(id) < 5 {
		t.Fatalf("queue printed no job id: %q", out)
	}

	out, _, code = run(t, dir, stub, "status", id, "--data", dir)
	if code != 0 || !strings.Contains(out, "queued") {
		t.Fatalf("status after queue = %d %s", code, out)
	}

	// A waiting clip takes exit 3 from take: nothing wrong, not ready.
	_, _, code = run(t, dir, stub, "take", id, "--data", dir)
	if code != 3 {
		t.Fatalf("take while queued = %d, want 3", code)
	}

	out, _, code = run(t, dir, stub, "work", "--data", dir, "--verbose")
	if code != 0 || !strings.Contains(out, "1 clip") {
		t.Fatalf("work = %d %s", code, out)
	}

	out, errs, code = run(t, dir, stub, "take", id, "--data", dir)
	if code != 0 {
		t.Fatalf("take after work = %d %s %s", code, out, errs)
	}
	clip := strings.TrimSpace(out)
	if !strings.HasPrefix(clip, dir) || !strings.HasSuffix(clip, ".mp4") {
		t.Fatalf("take printed %q, want a path in the library", clip)
	}
	if _, err := os.Stat(clip); err != nil {
		t.Fatalf("taken clip missing: %v", err)
	}

	// status --json round-trips the record.
	out, _, code = run(t, dir, stub, "status", id, "--json", "--data", dir)
	if code != 0 || !strings.Contains(out, `"status": "done"`) {
		t.Fatalf("status --json = %d %s", code, out)
	}
}

// A source that does not validate is refused at queue time, before any id
// is minted.
func TestQueueRejectsBadSource(t *testing.T) {
	dir := t.TempDir()
	_, errs, code := run(t, dir, stubTools(t), "queue", "--from", filepath.Join(dir, "nope.mp4"), "--start", "1", "--end", "2", "--data", dir)
	if code == 0 || !strings.Contains(errs, "clips:") {
		t.Fatalf("queue bad source = %d %s", code, errs)
	}
}

// `queue screen` queues a screen job; flags after the positional parse.
// Off Linux the render fails with the platform reason — recorded on the
// job, visible in status, exit 1 from take.
func TestQueueScreenForm(t *testing.T) {
	dir := t.TempDir()
	stub := stubTools(t)
	out, errs, code := run(t, dir, stub, "queue", "screen", "--seconds", "2", "--data", dir)
	if code != 0 {
		t.Fatalf("queue screen = %d %s %s", code, out, errs)
	}
	id := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])
	out, _, _ = run(t, dir, stub, "status", id, "--data", dir)
	if !strings.Contains(out, "screen") || !strings.Contains(out, "queued") {
		t.Fatalf("status after queue screen = %s", out)
	}
	if _, _, code = run(t, dir, stub, "work", "--data", dir); code != 0 {
		t.Fatalf("work = %d", code)
	}
	_, errs, code = run(t, dir, stub, "take", id, "--data", dir)
	if runtime.GOOS == "linux" {
		if code != 0 || !strings.Contains(errs, "clips:") {
			t.Fatalf("take screen on linux = %d %s", code, errs)
		}
	} else if code != 1 || !strings.Contains(errs, "Linux") {
		t.Fatalf("take screen off-linux = %d %s, want exit 1 naming the platform", code, errs)
	}
}
