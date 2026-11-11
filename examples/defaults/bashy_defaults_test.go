// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the bashy-defaults Bash# command (Sprint 588 D2).
//
// The script drives `bashy src fetch|upgrade` and `bashy commands show|add`.
// Every test points BASHY_HOME at a temp dir and BASHY_DEFAULTS_BASHY_BIN at
// a fake that records its argv, simulates the source pin and keeps a
// one-file-per-command registry, so a real registry is never touched.
package bashydefaults

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
	buildDir, err = os.MkdirTemp("", "bashy-defaults-*")
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

// fakeBashy: src fetch|upgrade lay out a pin with every default command and
// two kb pages; commands show NAME --json prints the stored exec path (exit 1
// when unregistered); commands add NAME ... records exec.2.
const fakeBashy = `#!/bin/sh
echo "$*" >> "$FAKE_LOG"
src="$BASHY_HOME/src/bashy"
case "$1 $2" in
  "src fetch"|"src upgrade")
    mkdir -p "$src/examples/outpost-profile" "$src/examples/satellite-member" "$src/examples/onboard" "$src/examples/satellite-provision" "$src/examples/defaults" "$src/kb/defaults/pages"
    : > "$src/examples/outpost-profile/outpost-profile.bsh"
    : > "$src/examples/satellite-member/satellite-member.bsh"
    : > "$src/examples/onboard/onboard.bsh"
    : > "$src/examples/satellite-provision/satellite-provision.bsh"
    : > "$src/examples/defaults/bashy-defaults.bsh"
    : > "$src/kb/defaults/pages/a.md"; : > "$src/kb/defaults/pages/b.md"
    ;;
  "commands show")
    [ -f "$FAKE_REG/$3" ] || exit 1
    printf '{"name":"%s","exec":["bashy","--bashsharp","%s"]}\n' "$3" "$(cat "$FAKE_REG/$3")"
    ;;
  "commands add")
    name=$3; shift 3; prev=""
    for a in "$@"; do
      case "$a" in exec.2=*) printf '%s' "${a#exec.2=}" > "$FAKE_REG/$name" ;; esac
    done
    ;;
  *) echo "fake bashy: unexpected $*" >&2; exit 1 ;;
esac
`

type fixture struct {
	t                           *testing.T
	bin, script, home, reg, log string
	fake                        string
	extraEnv                    []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	f := &fixture{
		t:      t,
		bin:    bashyBin(t),
		script: filepath.Join(moduleRoot(t), "examples", "defaults", "bashy-defaults.bsh"),
		home:   filepath.Join(root, "bhome"),
		reg:    filepath.Join(root, "reg"),
		log:    filepath.Join(root, "calls.log"),
		fake:   filepath.Join(root, "fakebashy"),
	}
	for _, d := range []string{f.home, f.reg} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(f.fake, []byte(fakeBashy), 0o755); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) run(args ...string) (string, int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bin, append([]string{"--bashsharp", f.script}, args...)...)
	cmd.Env = append(os.Environ(),
		"BASHY_HOME="+f.home, "HOME="+filepath.Dir(f.home), "BASHY_HINTS=off",
		"BASHY_DEFAULTS_BASHY_BIN="+f.fake, "FAKE_LOG="+f.log, "FAKE_REG="+f.reg)
	cmd.Env = append(cmd.Env, f.extraEnv...)
	cmd.Dir = filepath.Dir(f.home)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatalf("run %v: %v", args, err)
	}
	return string(out), code
}

func (f *fixture) registered(name string) string {
	b, _ := os.ReadFile(filepath.Join(f.reg, name))
	return string(b)
}

func (f *fixture) calls() string {
	b, _ := os.ReadFile(f.log)
	return string(b)
}

func (f *fixture) pin(rel string) string {
	return filepath.Join(f.home, "src", "bashy", filepath.FromSlash(rel))
}

// apply on a fresh install fetches the pin, registers every default command
// from it and reports the kb defaults; a second apply is a no-op fetch-wise.
func TestApplyFetchesRegistersAndReports(t *testing.T) {
	f := newFixture(t)
	out, code := f.run("apply")
	if code != 0 {
		t.Fatalf("apply: exit %d\n%s", code, out)
	}
	for _, want := range []string{"fetched the bashy source pin", "outpost-profile: registered", "satellite-member: registered", "onboard: registered", "satellite-provision: registered", "bashy-defaults: registered", "kb defaults: 2 page(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("apply output missing %q:\n%s", want, out)
		}
	}
	if got, want := f.registered("outpost-profile"), f.pin("examples/outpost-profile/outpost-profile.bsh"); got != want {
		t.Errorf("outpost-profile exec = %q, want %q", got, want)
	}
	if !strings.Contains(f.calls(), "--set effects.2=exec") {
		t.Errorf("effects not forwarded:\n%s", f.calls())
	}
	if !strings.Contains(f.calls(), "commands add satellite-provision --set exec.0=bashy --set exec.1=--bashsharp --set exec.2="+f.pin("examples/satellite-provision/satellite-provision.bsh")+" --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net") {
		t.Errorf("satellite-provision not registered with read,write,exec,net:\n%s", f.calls())
	}
	before := strings.Count(f.calls(), "src fetch")
	if out, code := f.run("apply"); code != 0 {
		t.Fatalf("second apply: exit %d\n%s", code, out)
	}
	if after := strings.Count(f.calls(), "src fetch"); after != before {
		t.Errorf("second apply fetched again (%d -> %d)", before, after)
	}
	if _, code := f.run("apply", "--upgrade"); code != 0 || !strings.Contains(f.calls(), "src upgrade bashy") {
		t.Errorf("--upgrade did not run src upgrade:\n%s", f.calls())
	}
}

// A command the user registered themselves under the same name is kept;
// --force replaces it.
func TestApplyKeepsOwnRegistrationUnlessForced(t *testing.T) {
	f := newFixture(t)
	if err := os.WriteFile(filepath.Join(f.reg, "outpost-profile"), []byte("/my/own/outpost-profile.bsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, code := f.run("apply")
	if code != 0 || !strings.Contains(out, "kept your own registration (/my/own/outpost-profile.bsh)") {
		t.Fatalf("apply did not keep the user's command: exit %d\n%s", code, out)
	}
	if got := f.registered("outpost-profile"); got != "/my/own/outpost-profile.bsh" {
		t.Fatalf("user registration overwritten: %q", got)
	}
	if out, code := f.run("apply", "--force"); code != 0 {
		t.Fatalf("apply --force: exit %d\n%s", code, out)
	}
	if got, want := f.registered("outpost-profile"), f.pin("examples/outpost-profile/outpost-profile.bsh"); got != want {
		t.Fatalf("--force did not replace: %q, want %q", got, want)
	}
}

// status reports the pin, each command's state and the kb page count.
func TestStatus(t *testing.T) {
	f := newFixture(t)
	if out, _ := f.run("status"); !strings.Contains(out, "source pin: missing") || !strings.Contains(out, "outpost-profile  missing") {
		t.Fatalf("status before apply:\n%s", out)
	}
	if _, code := f.run("apply"); code != 0 {
		t.Fatal("apply failed")
	}
	if err := os.WriteFile(filepath.Join(f.reg, "outpost-profile"), []byte("/elsewhere.bsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _ := f.run("status")
	// Paths under the home directory print as $HOME/..., so match the tail.
	for _, want := range []string{"bhome/src/bashy\n", "outpost-profile  own (/elsewhere.bsh)", "bashy-defaults   default", "kb defaults: 2 page(s)"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
}

// The one file carries the whole skill.
func TestSkillFence(t *testing.T) {
	b, err := os.ReadFile(filepath.Join(moduleRoot(t), "examples", "defaults", "bashy-defaults.bsh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	for _, want := range []string{"~~~skill as guide", "name: bashy-defaults", "bashy src fetch bashy", "apply", "status", "merge", "--force", "--upgrade", "kb/defaults", "bashy kb update", "bashy kb forget"} {
		if !strings.Contains(body, want) {
			t.Errorf("skill fence missing %q", want)
		}
	}
}

// merge registers missing defaults, keeps the user's own command, and lists
// only host pages that shadow a CHANGED default page, changing nothing.
func TestMergeReportsShadowedPages(t *testing.T) {
	f := newFixture(t)
	if _, code := f.run("apply"); code != 0 {
		t.Fatal("apply failed")
	}
	// The fake pin's default pages a.md and b.md are empty files.
	hostKB := filepath.Join(filepath.Dir(f.home), "hostkb")
	pages := filepath.Join(hostKB, "pages")
	if err := os.MkdirAll(pages, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "a.md"), []byte("my own notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pages, "b.md"), nil, 0o644); err != nil { // identical: not reported
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.reg, "outpost-profile"), []byte("/mine.bsh"), 0o644); err != nil {
		t.Fatal(err)
	}
	f.extraEnv = []string{"BASHY_KB_DIR=" + hostKB}
	out, code := f.run("merge")
	if code != 0 {
		t.Fatalf("merge: exit %d\n%s", code, out)
	}
	for _, want := range []string{"shadowed: a", "yours:   ", "a.md", "default: ", "1 shadowed page(s)", "kept your own registration (/mine.bsh)"} {
		if !strings.Contains(out, want) {
			t.Errorf("merge output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "shadowed: b") {
		t.Errorf("identical page reported as shadowed:\n%s", out)
	}
	if b, _ := os.ReadFile(filepath.Join(pages, "a.md")); string(b) != "my own notes" {
		t.Error("merge changed a page")
	}
	if got := f.registered("outpost-profile"); got != "/mine.bsh" {
		t.Errorf("merge overwrote the user's command: %q", got)
	}
}
