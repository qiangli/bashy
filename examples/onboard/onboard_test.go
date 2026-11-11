// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the onboard Bash# command (Sprint 604 C3). Every verb
// runs against a fake outpost (ONBOARD_OUTPOST_BIN) and a fake bashy
// (ONBOARD_BASHY_BIN) that record argv and answer the state probes from
// the environment; a real host is never touched. The satellite-member
// script is reached through the fake bashy too (`--bashsharp <script>
// <verb>`), so each step is asserted as one satellite-member verb call.
package onboard

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
	buildDir, err = os.MkdirTemp("", "onboard-*")
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

// fakeOutpost records argv. `status --local-presence` reports the daemon
// when $FAKE_DAEMON=1; `status --json` reports the pair-once cloudbox
// identity when $FAKE_PAIRED=1.
const fakeOutpost = `#!/bin/sh
echo "$*" >> "$FAKE_LOG"
case "$1 $2" in
  "status --local-presence")
    if [ "$FAKE_DAEMON" = 1 ]; then echo '{"running":true,"addr":"127.0.0.1:1"}'; else echo '{"running":false,"addr":"127.0.0.1:1"}'; fi ;;
  "status --json")
    if [ "$FAKE_PAIRED" = 1 ]; then
      echo '{"state":"paired once as member-host (identity only)","config":{"cloudbox":{"host":"member-host","has_token":true}}}'
    else
      echo '{"state":"dormant (unpaired)","agent_name":""}'
    fi ;;
esac
exit 0
`

// fakeBashy records argv; the two state probes answer from $FAKE_INVITES
// and $FAKE_SATS (JSON bodies). A satellite-member call arrives as
// `--bashsharp <script> <verb> ...` and is only recorded.
const fakeBashy = `#!/bin/sh
echo "$*" >> "$FAKE_BASHY_LOG"
case "$*" in
  "member invitations --json") if [ -n "$FAKE_INVITES" ]; then printf '%s\n' "$FAKE_INVITES"; else echo '{"invitations":[]}'; fi ;;
  "outpost satellite list --json") if [ -n "$FAKE_SATS" ]; then printf '%s\n' "$FAKE_SATS"; else echo '{"satellites":[]}'; fi ;;
esac
exit 0
`

// fakeTray stands in for bashy-tray: it only prints the install line.
const fakeTray = `#!/bin/sh
echo "bashy-tray: autostart installed for $0"
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
	fakeTrayBin := filepath.Join(root, "bashy-tray")
	if err := os.WriteFile(fakeTrayBin, []byte(fakeTray), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, bin: bashyBin(t), log: filepath.Join(root, "calls.log"),
		bashyLog: filepath.Join(root, "bashy-calls.log"),
		script:   filepath.Join(moduleRoot(t), "examples", "onboard", "onboard.bsh")}
	f.env = []string{"HOME=" + root, "BASHY_HINTS=off", "FAKE_LOG=" + f.log,
		"FAKE_BASHY_LOG=" + f.bashyLog,
		"ONBOARD_OUTPOST_BIN=" + fakeOutpostBin,
		"ONBOARD_BASHY_BIN=" + fakeBashyBin,
		"ONBOARD_TRAY_BIN=" + fakeTrayBin,
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

// calls is what the fake outpost received.
func (f *fixture) calls() string {
	b, _ := os.ReadFile(f.log)
	return string(b)
}

// bashyCalls is what the fake bashy received (probes + satellite-member verbs).
func (f *fixture) bashyCalls() string {
	b, _ := os.ReadFile(f.bashyLog)
	return string(b)
}

const (
	paired     = "FAKE_PAIRED=1"
	daemonUp   = "FAKE_DAEMON=1"
	oneInvite  = `FAKE_INVITES={"invitations":[{"id":"inv_91c2","from_email":"owner@example.com","status":"pending"}]}`
	twoInvites = `FAKE_INVITES={"invitations":[{"id":"inv_1","from_email":"a@example.com","status":"pending"},{"id":"inv_2","from_email":"b@example.com","status":"pending"}]}`
	oneSat     = `FAKE_SATS={"satellites":[{"id":"sat_1","name":"home","endpoint":"https://sat.example","state":"active","attached":false,"local":false}]}`
	attached   = `FAKE_SATS={"satellites":[{"id":"sat_1","name":"home","endpoint":"https://sat.example","state":"active","attached":true,"local":true}]}`
	twoSats    = `FAKE_SATS={"satellites":[{"id":"sat_1","name":"home","endpoint":"https://a.example","state":"active","attached":false,"local":false},{"id":"sat_2","name":"work","endpoint":"https://b.example","state":"active","attached":false,"local":false}]}`
)

func wantAll(t *testing.T, what, out string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("%s missing %q:\n%s", what, w, out)
		}
	}
}

// --dry-run prints every step's verb call and runs none of them.
func TestDryRunPrintsVerbCallsOnly(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil, "--dry-run")
	if code != 0 {
		t.Fatalf("--dry-run: exit %d\n%s", code, out)
	}
	wantAll(t, "dry-run", out,
		"would run: satellite-member pair --code <code>",
		"would run: satellite-member accept <invitation-id>",
		"would run: satellite-member attach <satellite>",
		"would probe: ",
		"bashy-tray install")
	if got := f.bashyCalls(); got != "" {
		t.Errorf("dry-run called bashy: %q", got)
	}
	if got := f.calls(); strings.Contains(got, "status") {
		t.Errorf("dry-run probed outpost: %q", got)
	}
	// The flag is accepted anywhere on the line, for one verb too.
	if out, code := f.run(nil, "pair", "--name", "box", "--dry-run"); code != 0 || !strings.Contains(out, "would run: satellite-member pair --code <code> --name box") {
		t.Errorf("pair --dry-run: exit %d\n%s", code, out)
	}
}

// check reports installed / daemon / paired and stops for the installer.
func TestCheckReportsState(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil, "check")
	if code != 0 {
		t.Fatalf("check: exit %d\n%s", code, out)
	}
	wantAll(t, "unpaired check", out, "outpost: installed", "daemon: not running", "paired: no", "onboard pair")
	out, code = f.run([]string{paired, daemonUp}, "check")
	if code != 0 {
		t.Fatalf("paired check: exit %d\n%s", code, out)
	}
	wantAll(t, "paired check", out, "daemon: running", "paired: yes (once, own account")
	out, code = f.run([]string{"ONBOARD_OUTPOST_BIN=" + filepath.Join(t.TempDir(), "missing-outpost")}, "check")
	if code != 0 {
		t.Fatalf("not-installed check: exit %d\n%s", code, out)
	}
	wantAll(t, "not-installed check", out, "outpost: not installed", "install.sh", "rerun `bashy onboard` to continue")
}

// The sequence stops at the first human step (sign in with the user's own
// account, mint a code) and never pairs on its own.
func TestSequenceStopsAtPairForHuman(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil)
	if code != 0 {
		t.Fatalf("onboard: exit %d\n%s", code, out)
	}
	wantAll(t, "pair stop", out, "sign in to the cloudbox portal with your own account", "Generate invite code", "bashy onboard pair --code <code>", "rerun `bashy onboard` to continue")
	if got := f.bashyCalls(); strings.Contains(got, "pair") || strings.Contains(got, "register") {
		t.Errorf("sequence paired on its own: %q", got)
	}
	if strings.Contains(out, "accept") && strings.Contains(out, "satellite-member accept") {
		t.Errorf("sequence went past the pair step:\n%s", out)
	}
}

// pair --code is one satellite-member pair call; a paired machine skips it.
func TestPairDelegatesToSatelliteMember(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "pair", "--code", "C0DE", "--name", "box"); code != 0 {
		t.Fatalf("pair: exit %d\n%s", code, out)
	}
	if got := f.bashyCalls(); !strings.Contains(got, "satellite-member.bsh pair --code C0DE --name box") {
		t.Errorf("pair argv = %q", got)
	}
	out, code := f.run([]string{paired}, "pair", "--code", "C0DE")
	if code != 0 || !strings.Contains(out, "already paired, skipped") {
		t.Errorf("paired pair: exit %d\n%s", code, out)
	}
	if got := f.bashyCalls(); strings.Count(got, "pair --code") != 1 {
		t.Errorf("paired machine paired again: %q", got)
	}
}

// Paired, one invitation, one satellite: the sequence accepts, attaches,
// installs the tray autostart and ends with the status.
func TestSequenceAcceptsAndAttaches(t *testing.T) {
	f := newFixture(t)
	out, code := f.run([]string{paired, daemonUp, oneInvite, oneSat})
	if code != 0 {
		t.Fatalf("onboard: exit %d\n%s", code, out)
	}
	got := f.bashyCalls()
	wantAll(t, "bashy argv", got, "member invitations --json", "satellite-member.bsh accept inv_91c2", "outpost satellite list --json", "satellite-member.bsh attach sat_1")
	if strings.Contains(got, "satellite-member.bsh pair") {
		t.Errorf("paired machine paired again: %q", got)
	}
	wantAll(t, "sequence output", out, "tray: autostart installed", "onboard: done", "paired: yes")
}

// Everything done: every step says so and nothing is called twice.
func TestSequenceSkipsDoneSteps(t *testing.T) {
	f := newFixture(t)
	out, code := f.run([]string{paired, daemonUp, attached})
	if code != 0 {
		t.Fatalf("onboard: exit %d\n%s", code, out)
	}
	wantAll(t, "skips", out, "pair: already paired, skipped", "accept: already a member", "attach: already attached, skipped", "attached: yes (sat_1)")
	if got := f.bashyCalls(); strings.Contains(got, "satellite-member.bsh") {
		t.Errorf("done sequence still acted: %q", got)
	}
}

// No invitation and nothing to attach to: stop and tell the user to ask
// the owner for an invitation.
func TestSequenceStopsForInvitation(t *testing.T) {
	f := newFixture(t)
	out, code := f.run([]string{paired, daemonUp})
	if code != 0 {
		t.Fatalf("onboard: exit %d\n%s", code, out)
	}
	wantAll(t, "invitation stop", out, "no pending invitation", "Ask the satellite owner", "bashy member invite <your-email>", "rerun `bashy onboard` to continue")
	if got := f.bashyCalls(); strings.Contains(got, "satellite-member.bsh") {
		t.Errorf("acted without an invitation: %q", got)
	}
}

// Several invitations or satellites: list them, stop, and let the user
// name one; a named one is one verb call.
func TestChoicesStopAndExplicitIDsAct(t *testing.T) {
	f := newFixture(t)
	out, code := f.run([]string{paired, twoInvites}, "accept")
	if code != 0 || !strings.Contains(out, "several pending invitations") || !strings.Contains(out, "bashy onboard accept <id>") {
		t.Errorf("two invitations: exit %d\n%s", code, out)
	}
	if out, code := f.run([]string{paired, twoInvites}, "accept", "inv_2", "--circle", "family"); code != 0 {
		t.Fatalf("accept id: exit %d\n%s", code, out)
	}
	if got := f.bashyCalls(); !strings.Contains(got, "satellite-member.bsh accept inv_2 --circle family") {
		t.Errorf("accept argv = %q", got)
	}
	out, code = f.run([]string{paired, twoSats}, "attach")
	if code != 0 || !strings.Contains(out, "several satellites") || !strings.Contains(out, "bashy onboard attach <id>") {
		t.Errorf("two satellites: exit %d\n%s", code, out)
	}
	if out, code := f.run([]string{paired, twoSats}, "attach", "sat_2"); code != 0 {
		t.Fatalf("attach id: exit %d\n%s", code, out)
	}
	if got := f.bashyCalls(); !strings.Contains(got, "satellite-member.bsh attach sat_2") {
		t.Errorf("attach argv = %q", got)
	}
}

// status is one screen of every step's state.
func TestStatus(t *testing.T) {
	f := newFixture(t)
	out, code := f.run([]string{paired, daemonUp, oneInvite, oneSat}, "status")
	if code != 0 {
		t.Fatalf("status: exit %d\n%s", code, out)
	}
	wantAll(t, "status", out, "outpost: installed", "daemon: running", "paired: yes", "invitations: 1 pending", "attached: no (candidates: 1", "tray: bashy-tray installed")
}

// --help renders the embedded skill (bashy onboard --help); --skill is the
// bare fence; the fence speaks in owner/member roles for any user.
func TestHelpRendersGenericSkill(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil, "--help")
	if code != 0 {
		t.Fatalf("--help: exit %d\n%s", code, out)
	}
	wantAll(t, "--help", out, "name: onboard", "requires: \"has=bashy\"", "# onboard", "Usage: onboard [--dry-run]")
	if out, code := f.run(nil, "--skill"); code != 0 || !strings.HasPrefix(out, "---\nname: onboard") {
		t.Errorf("--skill: exit %d\n%s", code, out)
	}
	b, err := os.ReadFile(f.script)
	if err != nil {
		t.Fatal(err)
	}
	body := strings.ToLower(string(b))
	wantAll(t, "skill", body, "~~~skill as guide", "owner", "member", "own account", "mutual", "rerun", "--dry-run", "satellite-member")
	for _, bad := range []string{"student", "teacher", "class"} {
		if strings.Contains(body, bad) {
			t.Errorf("skill uses the audience-specific word %q", bad)
		}
	}
}

// Without bashy-tray the tray step says so and skips, exit 0.
func TestTraySkipsWhenNotInstalled(t *testing.T) {
	f := newFixture(t)
	out, code := f.run([]string{"ONBOARD_TRAY_BIN=bashy-tray-missing-xyz"}, "tray")
	if code != 0 {
		t.Fatalf("tray: exit %d\n%s", code, out)
	}
	wantAll(t, "tray skip", out, "bashy-tray is not installed", "skipped")
}
