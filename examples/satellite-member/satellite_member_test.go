// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the satellite-member Bash# command. Every verb runs
// against a fake outpost (SATELLITE_MEMBER_OUTPOST_BIN) and a fake bashy
// (SATELLITE_MEMBER_BASHY_BIN) that record argv; a real host is never
// touched.
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

// fakeOutpost records argv. `status --json` reports the pair-once cloudbox
// identity when $FAKE_PAIRED=1; plain `status` reports a legacy tunnel
// pairing when $FAKE_LEGACY=1.
const fakeOutpost = `#!/bin/sh
echo "$*" >> "$FAKE_LOG"
case "$1" in
  status)
    if [ "$2" = "--json" ]; then
      if [ "$FAKE_PAIRED" = 1 ]; then
        echo "{\"state\":\"active\",\"config\":{\"cloudbox\":{\"host\":\"member-host\",\"owner_email\":\"member@example.com\",\"has_token\":true}}}"
      else
        echo "{\"state\":\"dormant (unpaired)\",\"agent_name\":\"\"}"
      fi
    elif [ "$FAKE_LEGACY" = 1 ]; then
      echo "  agent_name  member-host"
    fi ;;
esac
exit 0
`

// fakeBashy records argv for the account (`paired-host`, `member`) and the
// fleet front door (`outpost …`) verbs the script composes.
const fakeBashy = `#!/bin/sh
echo "$*" >> "$FAKE_BASHY_LOG"
exit 0
`

type fixture struct {
	t             *testing.T
	bin, script   string
	log, bashyLog string
	env           []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	fakeOutpostBin := filepath.Join(root, "outpost")
	if err := os.WriteFile(fakeOutpostBin, []byte(fakeOutpost), 0o755); err != nil {
		t.Fatal(err)
	}
	fakeBashyBin := filepath.Join(root, "bashy")
	if err := os.WriteFile(fakeBashyBin, []byte(fakeBashy), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, bin: bashyBin(t), log: filepath.Join(root, "calls.log"),
		bashyLog: filepath.Join(root, "bashy-calls.log"),
		script:   filepath.Join(moduleRoot(t), "examples", "satellite-member", "satellite-member.bsh")}
	f.env = []string{"HOME=" + root, "BASHY_HINTS=off", "FAKE_LOG=" + f.log,
		"FAKE_BASHY_LOG=" + f.bashyLog,
		"SATELLITE_MEMBER_OUTPOST_BIN=" + fakeOutpostBin,
		"SATELLITE_MEMBER_BASHY_BIN=" + fakeBashyBin,
		"PATH=" + root + string(os.PathListSeparator) + os.Getenv("PATH")}
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

// bashyCalls is what the fake bashy received (account + fleet verbs).
func (f *fixture) bashyCalls() string {
	b, _ := os.ReadFile(f.bashyLog)
	return string(b)
}

func TestCheckReportsPairing(t *testing.T) {
	f := newFixture(t)
	if out, _ := f.run(nil, "check"); !strings.Contains(out, "not paired") {
		t.Errorf("unpaired check: %q", out)
	}
	if out, code := f.run([]string{"FAKE_PAIRED=1"}, "check"); code != 0 || !strings.Contains(out, "paired with the cloudbox portal") {
		t.Errorf("pair-once check: exit %d %q", code, out)
	}
	if out, code := f.run([]string{"FAKE_LEGACY=1"}, "check"); code != 0 || !strings.Contains(out, "paired (tunnel)") {
		t.Errorf("legacy check: exit %d %q", code, out)
	}
}

func TestCodeMintsOwnAccountInvite(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "code"); code != 0 {
		t.Fatalf("code: exit %d %q", code, out)
	}
	if got := f.bashyCalls(); !strings.Contains(got, "paired-host invite") {
		t.Errorf("paired-host argv = %q", got)
	}
	if got := f.calls(); strings.Contains(got, "register") {
		t.Errorf("code must not pair: outpost argv = %q", got)
	}
}

func TestPairRegistersOnceWithOwnCode(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "pair", "--code", "C0DE", "--name", "box"); code != 0 {
		t.Fatalf("pair: exit %d %q", code, out)
	}
	if got := f.bashyCalls(); !strings.Contains(got, "outpost register --code C0DE --name box") {
		t.Errorf("register argv = %q", got)
	}
	if strings.Contains(f.bashyCalls(), "--server") {
		t.Errorf("pair-once never passes --server: %q", f.bashyCalls())
	}
	if _, code := f.run(nil, "pair"); code == 0 {
		t.Error("pair without --code succeeded")
	}
}

// A legacy satellite code (any --server) is refused unless the operator
// acknowledges owner node provisioning with --as-owner-node.
func TestPairRefusesServerWithoutOwnerNode(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil, "pair", "--code", "C0DE", "--server", "https://sat.example")
	if code == 0 {
		t.Fatalf("pair --server without --as-owner-node succeeded: %q", out)
	}
	for _, want := range []string{"refusing", "--as-owner-node", "owner of this machine"} {
		if !strings.Contains(out, want) {
			t.Errorf("refusal missing %q: %q", want, out)
		}
	}
	if got := f.bashyCalls(); strings.Contains(got, "register") {
		t.Errorf("refused pair still called register: %q", got)
	}
	if out, code := f.run(nil, "pair", "--code", "C0DE", "--server", "https://sat.example", "--as-owner-node"); code != 0 {
		t.Fatalf("pair --as-owner-node: exit %d %q", code, out)
	} else if got := f.bashyCalls(); !strings.Contains(got, "outpost register --code C0DE --server https://sat.example") {
		t.Errorf("owner-node register argv = %q", got)
	}
}

func TestAcceptListsAndAccepts(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "accept"); code != 0 {
		t.Fatalf("accept (list): exit %d %q", code, out)
	}
	if got := f.bashyCalls(); !strings.Contains(got, "member invitations") {
		t.Errorf("invitations argv = %q", got)
	}
	if out, code := f.run(nil, "accept", "inv_91c2", "--circle", "family"); code != 0 {
		t.Fatalf("accept id: exit %d %q", code, out)
	}
	if got := f.bashyCalls(); !strings.Contains(got, "member accept inv_91c2 --circle family") {
		t.Errorf("accept argv = %q", got)
	}
}

func TestAttachCallsSatelliteAttach(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "attach", "sat_1"); code != 0 {
		t.Fatalf("attach: exit %d %q", code, out)
	}
	if got := f.bashyCalls(); !strings.Contains(got, "outpost satellite attach sat_1") {
		t.Errorf("attach argv = %q", got)
	}
	if _, code := f.run(nil, "attach"); code == 0 {
		t.Error("attach without a satellite succeeded")
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
	if out, code := f.run([]string{"FAKE_LEGACY=1"}, "diag"); code != 0 || !strings.Contains(out, "agent_name") {
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
	for _, want := range []string{"~~~skill as guide", "name: satellite-member", "owner", "member", "own account", "paired once", "--as-owner-node", "mutual"} {
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

func TestShareCallsOutpostAppsShare(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "share", "board", "--email", "bob@example.com"); code != 0 || !strings.Contains(out, "shared board with bob@example.com") {
		t.Fatalf("share: exit %d %q", code, out)
	}
	if got := f.bashyCalls(); !strings.Contains(got, "outpost apps share board --email bob@example.com") {
		t.Errorf("bashy argv = %q", got)
	}
	if _, code := f.run(nil, "share", "board", "--email", "not-an-email"); code == 0 {
		t.Errorf("invalid email accepted")
	}
}
