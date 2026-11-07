// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the satellite-member Bash# command. Every verb runs
// against a fake outpost (SATELLITE_MEMBER_OUTPOST_BIN) that records argv;
// a real host is never touched.
package satellitemember

import (
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
	buildDir, err = os.MkdirTemp("", "satellite-member-*")
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
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func bashyBin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake outpost is a POSIX shell fixture")
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

// fakeOutpost records argv; `status` reports paired when $FAKE_PAIRED=1.
const fakeOutpost = `#!/bin/sh
echo "$*" >> "$FAKE_LOG"
case "$1" in
  status)
    if [ "$FAKE_PAIRED" = 1 ]; then echo "  agent_name  member-host"; else echo "  agent_name  "; fi ;;
esac
exit 0
`

type fixture struct {
	t                *testing.T
	bin, script, log string
	env              []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	fake := filepath.Join(root, "outpost")
	if err := os.WriteFile(fake, []byte(fakeOutpost), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, bin: bashyBin(t), log: filepath.Join(root, "calls.log"),
		script: filepath.Join(moduleRoot(t), "examples", "satellite-member", "satellite-member.bsh")}
	f.env = []string{"HOME=" + root, "BASHY_HINTS=off", "FAKE_LOG=" + f.log,
		"SATELLITE_MEMBER_OUTPOST_BIN=" + fake, "PATH=" + root + string(os.PathListSeparator) + os.Getenv("PATH")}
	return f
}

func (f *fixture) run(extra []string, args ...string) (string, int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bin, append([]string{"--bashsharp", f.script}, args...)...)
	cmd.Env = append(append(os.Environ(), f.env...), extra...)
	cmd.Dir = filepath.Dir(f.log)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatalf("run %v: %v", args, err)
	}
	return string(out), code
}

func (f *fixture) calls() string {
	b, _ := os.ReadFile(f.log)
	return string(b)
}

func TestCheckReportsPairing(t *testing.T) {
	f := newFixture(t)
	if out, _ := f.run(nil, "check"); !strings.Contains(out, "not paired") {
		t.Errorf("unpaired check: %q", out)
	}
	if out, code := f.run([]string{"FAKE_PAIRED=1"}, "check"); code != 0 || !strings.Contains(out, "installed and paired") {
		t.Errorf("paired check: exit %d %q", code, out)
	}
}

func TestPairForwardsCodeServerName(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "pair", "--code", "C0DE", "--server", "https://portal.example", "--name", "box"); code != 0 {
		t.Fatalf("pair: exit %d %q", code, out)
	}
	if got := f.calls(); !strings.Contains(got, "register --code C0DE --server https://portal.example --name box") {
		t.Errorf("register argv = %q", got)
	}
	if _, code := f.run(nil, "pair"); code == 0 {
		t.Error("pair without --code succeeded")
	}
}

func TestHardenTurnsBuiltinsOff(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "harden"); code != 0 {
		t.Fatalf("harden: exit %d %q", code, out)
	}
	got := f.calls()
	for _, want := range []string{"builtins set", "--service podman=off", "--service ollama-pool=off", "--service meet=off", "--shell off", "--desktop off", "--clipboard off"} {
		if !strings.Contains(got, want) {
			t.Errorf("harden argv missing %q: %q", want, got)
		}
	}
}

func TestPublishRequiresLoginAndIdentity(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "publish", "board", "--port", "18093"); code != 0 {
		t.Fatalf("publish: exit %d %q", code, out)
	}
	if got := f.calls(); !strings.Contains(got, "apps add board --url http://127.0.0.1:18093 --require-login --trust-cloud-identity") {
		t.Errorf("apps add argv = %q", got)
	}
	for _, bad := range [][]string{{"publish", "x|y", "--port", "1"}, {"publish", "board", "--port", "8o"}} {
		if _, code := f.run(nil, bad...); code == 0 {
			t.Errorf("%v succeeded", bad)
		}
	}
}

func TestDiagPrintsStatus(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run([]string{"FAKE_PAIRED=1"}, "diag"); code != 0 || !strings.Contains(out, "agent_name") {
		t.Errorf("diag: exit %d %q", code, out)
	}
}

// The skill speaks in owner/member roles for any user and their agent.
func TestSkillFenceIsGeneric(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(moduleRoot(t), "examples", "satellite-member", "satellite-member.bsh"))
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(b))
	for _, want := range []string{"~~~skill as guide", "name: satellite-member", "owner", "member", "own account"} {
		if !strings.Contains(body, want) {
			t.Errorf("skill missing %q", want)
		}
	}
	for _, bad := range []string{"student", "teacher", "class"} {
		if strings.Contains(body, bad) {
			t.Errorf("skill uses the audience-specific word %q", bad)
		}
	}
}
