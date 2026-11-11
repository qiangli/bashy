// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the satellite-provision Bash# command (Sprint 604 C4).
// Every verb runs against a fake bashy (SATELLITE_PROVISION_BASHY_BIN) that
// records argv; a provider, a master or a vault is never touched.
package satelliteprovision

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
	buildDir, err = os.MkdirTemp("", "satellite-provision-*")
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
		t.Skip("fake bashy is a POSIX shell fixture")
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

// fakeBashy records argv. `secret list PREFIX` answers from $FAKE_VAULT
// (space-separated names); `secret exec NAMES... -- CMD...` strips the
// wrapper and runs CMD (so the inner verb is recorded too); `satellite list
// --json` answers $FAKE_SATELLITES; `infra provision` prints an endpoint;
// `fetch -i` answers $FAKE_HTTP.
const fakeBashy = `#!/bin/sh
echo "$*" >> "$FAKE_LOG"
case "$1 $2" in
  "secret list")
    for n in $FAKE_VAULT; do case "$n" in "$3"*) echo "$n" ;; esac; done ;;
  "secret exec")
    shift 2
    while [ $# -gt 0 ] && [ "$1" != "--" ]; do shift; done
    shift
    exec "$@" ;;
  "satellite list")
    echo "{\"satellites\":[${FAKE_SATELLITES}]}" ;;
  "satellite show")
    echo "{\"id\":\"sat_1\",\"name\":\"$3\",\"endpoint\":\"https://a-b-c-d.sslip.io\"}" ;;
  "infra provision")
    echo "endpoint: https://a-b-c-d.sslip.io" ;;
  "infra list")
    echo "${FAKE_LEFT:-}" ;;
  "fetch -i")
    echo "${FAKE_HTTP:-503 Service Unavailable}" ;;
esac
exit 0
`

type fixture struct {
	t           *testing.T
	bin, script string
	log, fake   string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{t: t, bin: bashyBin(t), log: filepath.Join(root, "calls.log"),
		fake:   filepath.Join(root, "bashy"),
		script: filepath.Join(moduleRoot(t), "examples", "satellite-provision", "satellite-provision.bsh")}
	if err := os.WriteFile(f.fake, []byte(fakeBashy), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) run(extra []string, args ...string) (string, int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bin, append([]string{"--bashsharp", f.script}, args...)...)
	env := []string{"HOME=" + filepath.Dir(f.log), "BASHY_HINTS=off", "FAKE_LOG=" + f.log,
		"SATELLITE_PROVISION_BASHY_BIN=" + f.fake, "SATELLITE_PROVISION_POLL=0",
		"FAKE_VAULT=AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY OWNER_TOK DO_TOK",
		"PATH=" + filepath.Dir(f.fake) + string(os.PathListSeparator) + os.Getenv("PATH")}
	cmd.Env = append(append(os.Environ(), env...), extra...)
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

// calls is what the fake bashy received, one argv per line.
func (f *fixture) calls() string {
	b, _ := os.ReadFile(f.log)
	return string(b)
}

// shown normalizes the script's printed calls: bashy prints a path under
// the home directory as $HOME/..., and the fake lives at $HOME/bashy.
func (f *fixture) shown(out string) string {
	out = strings.ReplaceAll(out, "$HOME/bashy", f.fake)
	out = strings.ReplaceAll(out, "+ "+f.fake+" ", "+ ")
	return strings.ReplaceAll(out, " -- "+f.fake+" ", " -- FAKE ")
}

const planCall = "secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- " +
	"FAKE infra provision aws --satellite sat-a --region us-west-2 --size small_3_0 --image ubuntu_24_04"

// --dry-run prints the exact calls and runs nothing: plan, up and down.
func TestDryRunPrintsCallsAndRunsNothing(t *testing.T) {
	f := newFixture(t)
	cases := []struct {
		verb string
		want []string
	}{
		{"plan", []string{"+ " + planCall + " --budget-cap 30 --budget-per-instance 12 --sprint 604 --story 3110 --ttl 2h --dry-run"}},
		{"up", []string{"+ secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY OWNER_TOK=BASHY_FLEET_TOKEN -- " +
			"FAKE infra provision aws --satellite sat-a --region us-west-2 --size small_3_0 --image ubuntu_24_04 --budget-cap 30 --budget-per-instance 12 --sprint 604 --story 3110 --ttl 2h"}},
		{"down", []string{
			"+ secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- FAKE infra inspect aws --name sat-a --region us-west-2",
			"+ secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- FAKE infra destroy aws --name sat-a --region us-west-2 --yes",
			"+ secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- FAKE infra list --provider aws --region us-west-2",
		}},
	}
	for _, c := range cases {
		out, code := f.run(nil, c.verb, "--satellite", "sat-a", "--owner-token-secret", "OWNER_TOK",
			"--budget-cap", "30", "--budget-per-instance", "12", "--sprint", "604", "--story", "3110", "--ttl", "2h", "--dry-run")
		if code != 0 {
			t.Fatalf("%s --dry-run: exit %d\n%s", c.verb, code, out)
		}
		got := f.shown(out)
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s --dry-run output missing\n  %s\ngot:\n%s", c.verb, w, got)
			}
		}
		if strings.Contains(out, "secret exec") && strings.Contains(f.calls(), "infra") {
			t.Errorf("%s --dry-run ran a verb: %s", c.verb, f.calls())
		}
	}
}

// A mutating verb without --yes prints the calls, runs nothing, exits 1.
func TestMutatingVerbsRefuseWithoutYes(t *testing.T) {
	f := newFixture(t)
	for _, args := range [][]string{
		{"register", "--satellite", "sat-a", "--owner-token-secret", "OWNER_TOK"},
		{"up", "--satellite", "sat-a"},
		{"invite", "--satellite", "sat-a", "--email", "m@example.com"},
		{"share-llm", "--satellite", "sat-a", "--peer", "box"},
		{"down", "--satellite", "sat-a"},
	} {
		out, code := f.run(nil, args...)
		if code != 1 {
			t.Errorf("%v without --yes: exit %d, want 1\n%s", args, code, out)
		}
		for _, want := range []string{"refusing without --yes", "+ ", "nothing was run"} {
			if !strings.Contains(out, want) {
				t.Errorf("%v refusal missing %q:\n%s", args, want, out)
			}
		}
	}
	if got := f.calls(); strings.Contains(got, "infra") || strings.Contains(got, "invite") || strings.Contains(got, "llm") || strings.Contains(got, "/bin/sh") {
		t.Errorf("a refused verb still ran something:\n%s", got)
	}
}

// plan runs the provider dry-run under the vault credential names only.
func TestPlanRunsInfraProvisionDryRun(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil, "plan", "--satellite", "sat-a", "--budget-cap", "30")
	if code != 0 || !strings.Contains(out, "nothing was created") {
		t.Fatalf("plan: exit %d\n%s", code, out)
	}
	if got := f.calls(); !strings.Contains(got, "secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY -- "+f.fake+" infra provision aws --satellite sat-a --region us-west-2 --size small_3_0 --image ubuntu_24_04 --budget-cap 30 --dry-run") {
		t.Errorf("plan argv:\n%s", got)
	}
	if _, code := f.run(nil, "plan"); code != 2 {
		t.Error("plan without --satellite did not exit 2")
	}
	if _, code := f.run(nil, "plan", "--satellite", "bad name"); code != 2 {
		t.Error("plan with an invalid id did not exit 2")
	}
}

// check: vault names, the master answers, the name is free or taken.
func TestCheckVaultMasterAndName(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil, "check", "--satellite", "sat-a", "--owner-token-secret", "OWNER_TOK", "--master", "https://master.example")
	if code != 0 {
		t.Fatalf("check: exit %d\n%s", code, out)
	}
	for _, want := range []string{"vault: AWS_ACCESS_KEY_ID present", "vault: AWS_SECRET_ACCESS_KEY present", "vault: OWNER_TOK present", "name sat-a is free", "ready"} {
		if !strings.Contains(out, want) {
			t.Errorf("check missing %q:\n%s", want, out)
		}
	}
	if got := f.calls(); !strings.Contains(got, "secret exec OWNER_TOK=BASHY_FLEET_TOKEN -- "+f.fake+" satellite list --master https://master.example --json") {
		t.Errorf("satellite list argv:\n%s", got)
	}
	out, code = f.run([]string{`FAKE_SATELLITES={"id":"sat_9","name":"sat-a","endpoint":""}`}, "check", "--satellite", "sat-a")
	if code == 0 || !strings.Contains(out, "TAKEN") {
		t.Errorf("taken name not reported: exit %d\n%s", code, out)
	}
	out, code = f.run([]string{"FAKE_VAULT=AWS_ACCESS_KEY_ID"}, "check", "--satellite", "sat-a")
	if code == 0 || !strings.Contains(out, "AWS_SECRET_ACCESS_KEY MISSING") {
		t.Errorf("missing vault name not reported: exit %d\n%s", code, out)
	}
}

// register has no verb: it is the runbook's POST under secret exec, and the
// printed call names the vault entry, never a value.
func TestRegisterPostsUnderSecretExec(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil, "register", "--satellite", "sat-a", "--owner-token-secret", "OWNER_TOK", "--master", "https://master.example", "--dry-run")
	if code != 0 {
		t.Fatalf("register --dry-run: exit %d\n%s", code, out)
	}
	for _, want := range []string{"secret exec OWNER_TOK=T -- /bin/sh -c", "Bearer $T", "/api/v1/satellites", "sh https://master.example sat-a"} {
		if !strings.Contains(out, want) {
			t.Errorf("register call missing %q:\n%s", want, out)
		}
	}
	if _, code := f.run([]string{"BASHY_FLEET_TOKEN="}, "register", "--satellite", "sat-a", "--yes"); code != 2 {
		t.Error("register without a bearer source did not exit 2")
	}
}

// up --yes runs the live provision with both credentials, then polls the
// printed endpoint until it answers 200.
func TestUpProvisionsAndWaitsForEndpoint(t *testing.T) {
	f := newFixture(t)
	out, code := f.run([]string{"FAKE_HTTP=200 OK"}, "up", "--satellite", "sat-a", "--owner-token-secret", "OWNER_TOK", "--yes", "--wait", "5")
	if code != 0 || !strings.Contains(out, "https://a-b-c-d.sslip.io answers 200") {
		t.Fatalf("up: exit %d\n%s", code, out)
	}
	got := f.calls()
	if !strings.Contains(got, "secret exec AWS_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY OWNER_TOK=BASHY_FLEET_TOKEN -- "+f.fake+" infra provision aws --satellite sat-a") || strings.Contains(got, "--dry-run") {
		t.Errorf("up argv:\n%s", got)
	}
	if !strings.Contains(got, "fetch -i --timeout 10s https://a-b-c-d.sslip.io") {
		t.Errorf("up did not poll the endpoint:\n%s", got)
	}
	if out, code := f.run(nil, "up", "--satellite", "sat-a", "--yes", "--wait", "1"); code == 0 || !strings.Contains(out, "did not answer 200") {
		t.Errorf("up with a 503 endpoint: exit %d\n%s", code, out)
	}
}

// invite --email is a member invitation; --name an OWNER-SCOPED node code.
func TestInviteMemberOrOwnerNode(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "invite", "--satellite", "sat-a", "--email", "m@example.com", "--circle", "family", "--yes"); code != 0 || !strings.Contains(out, "invited m@example.com as a member") {
		t.Fatalf("invite --email: exit %d\n%s", code, out)
	}
	if got := f.calls(); !strings.Contains(got, "member invite m@example.com --circle family") {
		t.Errorf("member invite argv:\n%s", got)
	}
	if out, code := f.run(nil, "invite", "--satellite", "sat-a", "--name", "node1", "--yes"); code != 0 || !strings.Contains(out, "OWNER-SCOPED") || !strings.Contains(out, "Never hand it to a member") {
		t.Fatalf("invite --name: exit %d\n%s", code, out)
	}
	if got := f.calls(); !strings.Contains(got, "satellite invite --satellite https://a-b-c-d.sslip.io --name node1") {
		t.Errorf("satellite invite argv (URL from satellite show):\n%s", got)
	}
	if _, code := f.run(nil, "invite", "--satellite", "sat-a", "--yes"); code != 2 {
		t.Error("invite without --name/--email did not exit 2")
	}
	if _, code := f.run(nil, "invite", "--satellite", "sat-a", "--email", "nope", "--yes"); code != 2 {
		t.Error("invite with an invalid email did not exit 2")
	}
}

func TestShareLLMCallsLlmShare(t *testing.T) {
	f := newFixture(t)
	if out, code := f.run(nil, "share-llm", "--satellite", "sat-a", "--peer", "box", "--via", "cloudbox", "--yes"); code != 0 || !strings.Contains(out, "door shared with box") {
		t.Fatalf("share-llm: exit %d\n%s", code, out)
	}
	if got := f.calls(); !strings.Contains(got, "llm share box --satellite sat-a --via cloudbox") {
		t.Errorf("llm share argv:\n%s", got)
	}
	if _, code := f.run(nil, "share-llm", "--satellite", "sat-a", "--yes"); code != 2 {
		t.Error("share-llm without --peer did not exit 2")
	}
}

// down --yes inspects, destroys (which releases the static address) and
// lists what the provider still holds.
func TestDownDestroysAndListsRemaining(t *testing.T) {
	f := newFixture(t)
	out, code := f.run([]string{"FAKE_LEFT=NAME  STATE\nother-ip  unattached"}, "down", "--satellite", "sat-a", "--yes")
	if code != 0 {
		t.Fatalf("down: exit %d\n%s", code, out)
	}
	for _, want := range []string{"still held by aws", "other-ip  unattached", "still bills"} {
		if !strings.Contains(out, want) {
			t.Errorf("down output missing %q:\n%s", want, out)
		}
	}
	got := f.calls()
	for _, want := range []string{"infra inspect aws --name sat-a --region us-west-2", "infra destroy aws --name sat-a --region us-west-2 --yes", "infra list --provider aws --region us-west-2"} {
		if !strings.Contains(got, want) {
			t.Errorf("down argv missing %q:\n%s", want, got)
		}
	}
}

// DigitalOcean is accepted, warned about, and demands region + image.
func TestDigitalOceanIsUnverifiedAndExplicit(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil, "plan", "--provider", "digitalocean", "--satellite", "sat-a", "--do-token-secret", "DO_TOK", "--dry-run")
	if code != 2 || !strings.Contains(out, "NOT verified live") || !strings.Contains(out, "needs --region and --image") {
		t.Fatalf("digitalocean without region/image: exit %d\n%s", code, out)
	}
	out, code = f.run(nil, "plan", "--provider", "digitalocean", "--satellite", "sat-a", "--do-token-secret", "DO_TOK", "--region", "sfo3", "--image", "ubuntu-24-04-x64", "--dry-run")
	if code != 0 || !strings.Contains(out, "NOT verified live") {
		t.Fatalf("digitalocean plan --dry-run: exit %d\n%s", code, out)
	}
	if !strings.Contains(f.shown(out), "+ secret exec DO_TOK=DIGITALOCEAN_ACCESS_TOKEN -- FAKE infra provision digitalocean --satellite sat-a --region sfo3 --size s-1vcpu-1gb --image ubuntu-24-04-x64 --dry-run") {
		t.Errorf("digitalocean plan call:\n%s", out)
	}
}

// --help renders the skill; the skill speaks for any owner and their agent.
func TestHelpRendersGenericSkill(t *testing.T) {
	f := newFixture(t)
	out, code := f.run(nil, "--help")
	if code != 0 {
		t.Fatalf("--help: exit %d\n%s", code, out)
	}
	body := strings.ToLower(out)
	for _, want := range []string{"usage: satellite-provision", "name: satellite-provision", "requires: \"has=bashy\"", "budget", "--yes", "owner", "member", "static address", "not verified live", "never hand this code to a member"} {
		if !strings.Contains(body, want) {
			t.Errorf("--help missing %q", want)
		}
	}
	for _, bad := range []string{"student", "teacher", "class"} {
		if strings.Contains(body, bad) {
			t.Errorf("skill uses the audience-specific word %q", bad)
		}
	}
	if skill, code := f.run(nil, "--skill"); code != 0 || !strings.HasPrefix(skill, "---\nname: satellite-provision") {
		t.Errorf("--skill: exit %d\n%s", code, skill)
	}
}
