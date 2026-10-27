// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the outpost-profile Bash# command (S514.6).
//
// The script under test only drives `outpost register --out` and
// `outpost restart`. It must NEVER run against the live outpost on the
// dev box: every test below points HOME and XDG_CONFIG_HOME at a temp
// dir and puts a fake `outpost` executable first on PATH. No
// podman/docker/containers are involved.
//
// The fake is a POSIX shell fixture, which the OS cannot execute on
// Windows, so the exec tests skip there (the Windows path-conversion
// branch is covered by TestWindowsOutPathConversion on unix via a fake
// uname/cygpath). TestSkillDocMatchesScript runs on every OS.
package outpostprofile

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

// requireExecEnv skips when this host cannot run the POSIX fixtures:
// the fake outpost is an extensionless shell script.
func requireExecEnv(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake outpost is a POSIX shell fixture, not executable on Windows")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("no go toolchain on PATH")
	}
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found above test file")
		}
		dir = parent
	}
}

var (
	buildOnce sync.Once
	buildBin  string
	buildErr  error
)

// The built binary must outlive any single test: t.TempDir() inside
// sync.Once is removed when the first test finishes, so the build lands
// in a package-scoped dir that TestMain removes at the end.
var buildDir string

func TestMain(m *testing.M) {
	var err error
	buildDir, err = os.MkdirTemp("", "outpost-profile-bashy-*")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

// bashyBin builds ./cmd/bashy once and returns the binary path.
func bashyBin(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bin := filepath.Join(buildDir, "bashy")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err: err, out: string(out)}
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

// fixture is one isolated HOME/XDG_CONFIG_HOME plus a fake outpost.
type fixture struct {
	t      *testing.T
	root   string
	script string
	bin    string
	env    []string
	log    string
}

// fakeOutpost logs its argv and emulates `register --out` (writes a
// server-tagged agent.json so profiles differ) and `restart`.
//
// In the Windows simulation the script hands outpost a converted path
// while asserting on the shell spelling; the fake writes both,
// emulating the MSYS equivalence (/c/... and C:/... name one file).
const fakeOutpost = `#!/bin/sh
echo "$@" >> "$FAKE_LOG"
out=""; server=""; prev=""
for a in "$@"; do
  case "$prev" in
    --out) out="$a" ;;
    --server) server="$a" ;;
  esac
  prev="$a"
done
case "$1" in
  register)
    [ -n "$out" ] || { echo "fake outpost: register needs --out" >&2; exit 1; }
    mkdir -p "$(dirname "$out")"
    printf '{"fake":true,"server":"%s"}\n' "$server" > "$out"
    case "$out" in C:*)
      stripped=${out#C:}
      mkdir -p "$(dirname "$stripped")"
      printf '{"fake":true,"server":"%s"}\n' "$server" > "$stripped"
      ;;
    esac
    ;;
  restart) : ;;
  *) echo "fake outpost: unknown command $1" >&2; exit 1 ;;
esac
`

func newFixture(t *testing.T, extraBin map[string]string) *fixture {
	t.Helper()
	requireExecEnv(t)
	root := t.TempDir()
	fakebin := filepath.Join(root, "fakebin")
	if err := os.MkdirAll(fakebin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fakebin, "outpost"), []byte(fakeOutpost), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range extraBin {
		if err := os.WriteFile(filepath.Join(fakebin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	script := filepath.Join(moduleRoot(t), "examples", "outpost-profile", "outpost-profile.bsh")
	f := &fixture{
		t:      t,
		root:   root,
		script: script,
		bin:    bashyBin(t),
		log:    filepath.Join(root, "calls.log"),
	}
	f.env = []string{
		"HOME=" + filepath.Join(root, "home"),
		"XDG_CONFIG_HOME=" + filepath.Join(root, "xcfg"),
		"BASHY_HOME=" + filepath.Join(root, "bhome"),
		"BASHY_COMMANDS_DIR=" + filepath.Join(root, "commands"),
		"BASHY_COMMANDS_PATH=",
		"BASHY_HINTS=off",
		"FAKE_LOG=" + f.log,
		"PATH=" + fakebin + string(os.PathListSeparator) + os.Getenv("PATH"),
	}
	return f
}

// run executes the script through the built bashy binary.
func (f *fixture) run(args ...string) (stdout, stderr string, code int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bin, append([]string{"--bashsharp", f.script}, args...)...)
	cmd.Env = append(os.Environ(), f.env...)
	// Pin the cwd: the Windows simulation writes a converted C: path
	// that must land in scratch, never in the repo checkout.
	cmd.Dir = f.root
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			f.t.Fatalf("run %v: %v", args, err)
		}
	}
	return outBuf.String(), errBuf.String(), code
}

func (f *fixture) calls() []string {
	f.t.Helper()
	raw, err := os.ReadFile(f.log)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		f.t.Fatal(err)
	}
	var lines []string
	for _, l := range strings.Split(string(raw), "\n") {
		if l != "" {
			lines = append(lines, l)
		}
	}
	return lines
}

func (f *fixture) xcfg() string {
	for _, e := range f.env {
		if v, ok := strings.CutPrefix(e, "XDG_CONFIG_HOME="); ok {
			return v
		}
	}
	f.t.Fatal("XDG_CONFIG_HOME missing from fixture env")
	return ""
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// add runs `outpost register --out <profile>/agent.json`.
func TestAddRunsRegisterWithOut(t *testing.T) {
	f := newFixture(t, nil)
	out, _, code := f.run("add", "alpha", "--code", "CODE-ALPHA-1", "--server", "https://sat-alpha.example")
	if code != 0 {
		t.Fatalf("add: exit %d", code)
	}
	if strings.Contains(out, "CODE-ALPHA-1") {
		t.Fatalf("pairing code leaked into stdout: %q", out)
	}
	want := filepath.Join(f.xcfg(), "matrix", "profiles", "alpha", "agent.json")
	if got := readFile(t, want); !strings.Contains(got, "https://sat-alpha.example") {
		t.Fatalf("profile file %q = %q, want server tag", want, got)
	}
	calls := f.calls()
	if len(calls) != 1 {
		t.Fatalf("outpost calls = %v, want one register", calls)
	}
	for _, wantArg := range []string{
		"register",
		"--server https://sat-alpha.example",
		"--code CODE-ALPHA-1",
		"--out " + want,
	} {
		if !strings.Contains(calls[0], wantArg) {
			t.Errorf("register call %q missing %q", calls[0], wantArg)
		}
	}
}

// use copies the profile into the active agent.json (backing up the
// current one into its own profile) and runs outpost restart.
func TestUseSwapsActiveBackupAndRestart(t *testing.T) {
	f := newFixture(t, nil)
	base := filepath.Join(f.xcfg(), "matrix")
	writeFile(t, filepath.Join(base, "profiles", "alpha", "agent.json"), "{\"sat\":\"alpha\"}\n")
	writeFile(t, filepath.Join(base, "profiles", "beta", "agent.json"), "{\"sat\":\"beta\"}\n")
	writeFile(t, filepath.Join(base, "agent.json"), "{\"sat\":\"alpha\"}\n")

	out, _, code := f.run("use", "beta")
	if code != 0 {
		t.Fatalf("use: exit %d", code)
	}
	if got := readFile(t, filepath.Join(base, "agent.json")); got != "{\"sat\":\"beta\"}\n" {
		t.Fatalf("active file = %q, want beta content", got)
	}
	if got := readFile(t, filepath.Join(base, "profiles", "alpha", "agent.json")); got != "{\"sat\":\"alpha\"}\n" {
		t.Fatalf("alpha backup = %q, want pre-switch active content", got)
	}
	if !strings.Contains(out, "now using profile beta") {
		t.Errorf("use output %q missing activation line", out)
	}
	calls := f.calls()
	if len(calls) != 1 || calls[0] != "restart" {
		t.Fatalf("outpost calls = %v, want exactly [restart]", calls)
	}
}

// list marks the profile identical to the active file with *.
func TestListMarksActive(t *testing.T) {
	f := newFixture(t, nil)
	base := filepath.Join(f.xcfg(), "matrix")
	writeFile(t, filepath.Join(base, "profiles", "alpha", "agent.json"), "{\"sat\":\"alpha\"}\n")
	writeFile(t, filepath.Join(base, "profiles", "beta", "agent.json"), "{\"sat\":\"beta\"}\n")
	writeFile(t, filepath.Join(base, "agent.json"), "{\"sat\":\"beta\"}\n")

	out, _, code := f.run("list")
	if code != 0 {
		t.Fatalf("list: exit %d", code)
	}
	// Trim only the trailing newline: leading spaces mark inactivity.
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 || lines[0] != "  alpha" || lines[1] != "* beta" {
		t.Fatalf("list = %q, want [\"  alpha\" \"* beta\"]", out)
	}
}

// use of an unknown profile fails without touching outpost or files.
func TestUseUnknownProfileFails(t *testing.T) {
	f := newFixture(t, nil)
	base := filepath.Join(f.xcfg(), "matrix")
	writeFile(t, filepath.Join(base, "profiles", "alpha", "agent.json"), "{\"sat\":\"alpha\"}\n")
	writeFile(t, filepath.Join(base, "agent.json"), "{\"sat\":\"alpha\"}\n")

	_, errOut, code := f.run("use", "gamma")
	if code == 0 {
		t.Fatal("use gamma: exit 0, want nonzero")
	}
	if !strings.Contains(errOut, "no such profile") {
		t.Errorf("use gamma stderr %q missing reason", errOut)
	}
	if calls := f.calls(); len(calls) != 0 {
		t.Errorf("outpost calls = %v, want none", calls)
	}
	if got := readFile(t, filepath.Join(base, "agent.json")); got != "{\"sat\":\"alpha\"}\n" {
		t.Errorf("active file changed to %q", got)
	}
}

// Invalid profile names are refused before outpost runs.
func TestAddRejectsBadName(t *testing.T) {
	for _, name := range []string{"a/b", "..", "-x", ".hidden", `a\b`} {
		f := newFixture(t, nil)
		_, _, code := f.run("add", name, "--code", "C", "--server", "https://s.example")
		if code != 2 {
			t.Errorf("add %q: exit %d, want 2 (usage)", name, code)
		}
		if calls := f.calls(); len(calls) != 0 {
			t.Errorf("add %q: outpost calls = %v, want none", name, calls)
		}
	}
}

// XDG_CONFIG_HOME wins over $HOME/.config.
func TestXDGConfigHomeHonored(t *testing.T) {
	f := newFixture(t, nil)
	custom := filepath.Join(f.root, "custom-cfg")
	f.env = append(f.env, "XDG_CONFIG_HOME="+custom)
	out, _, code := f.run("add", "x", "--code", "C", "--server", "https://s.example")
	if code != 0 {
		t.Fatalf("add: exit %d (%s)", code, out)
	}
	want := filepath.Join(custom, "matrix", "profiles", "x", "agent.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("profile not under XDG_CONFIG_HOME: %v", err)
	}
}

// On Windows the --out path handed to the native outpost.exe uses the
// Windows spelling. Simulated everywhere with OS=Windows_NT (what Git
// Bash/MSYS2/Cygwin all set) plus a fake cygpath. A fake uname would not
// work: bashy resolves uname in-process, ahead of PATH.
func TestWindowsOutPathConversion(t *testing.T) {
	f := newFixture(t, map[string]string{
		"cygpath": "#!/bin/sh\nlast=\"\"; for a in \"$@\"; do last=\"$a\"; done\nprintf 'C:%s\\n' \"$last\"\n",
	})
	f.env = append(f.env, "OS=Windows_NT")
	_, _, code := f.run("add", "winalpha", "--code", "C", "--server", "https://s.example")
	if code != 0 {
		t.Fatalf("add: exit %d", code)
	}
	calls := f.calls()
	if len(calls) != 1 {
		t.Fatalf("outpost calls = %v, want one register", calls)
	}
	var outArg string
	prev := ""
	for _, field := range strings.Fields(calls[0]) {
		if prev == "--out" {
			outArg = field
		}
		prev = field
	}
	if !strings.HasPrefix(outArg, "C:") || !strings.HasSuffix(outArg, "/matrix/profiles/winalpha/agent.json") {
		t.Fatalf("--out = %q, want C:-prefixed Windows spelling", outArg)
	}
}

// A drifted active file (credentials refreshed in place) is backed up
// into the recorded profile, not lost and not misattributed.
func TestUseBacksUpDriftedActiveViaMarker(t *testing.T) {
	f := newFixture(t, nil)
	base := filepath.Join(f.xcfg(), "matrix")
	writeFile(t, filepath.Join(base, "profiles", "alpha", "agent.json"), "{\"sat\":\"alpha\"}\n")
	writeFile(t, filepath.Join(base, "profiles", "beta", "agent.json"), "{\"sat\":\"beta\"}\n")
	writeFile(t, filepath.Join(base, ".active"), "alpha\n")
	writeFile(t, filepath.Join(base, "agent.json"), "{\"sat\":\"alpha-refreshed\"}\n")

	if _, _, code := f.run("use", "beta"); code != 0 {
		t.Fatalf("use: exit %d", code)
	}
	if got := readFile(t, filepath.Join(base, "profiles", "alpha", "agent.json")); got != "{\"sat\":\"alpha-refreshed\"}\n" {
		t.Fatalf("alpha backup = %q, want drifted active content", got)
	}
	if got := readFile(t, filepath.Join(base, "agent.json")); got != "{\"sat\":\"beta\"}\n" {
		t.Fatalf("active file = %q, want beta content", got)
	}
}

// First use with no active file yet just activates and restarts.
func TestUseWithoutActiveFile(t *testing.T) {
	f := newFixture(t, nil)
	base := filepath.Join(f.xcfg(), "matrix")
	writeFile(t, filepath.Join(base, "profiles", "beta", "agent.json"), "{\"sat\":\"beta\"}\n")

	if _, _, code := f.run("use", "beta"); code != 0 {
		t.Fatalf("use: exit %d", code)
	}
	if got := readFile(t, filepath.Join(base, "agent.json")); got != "{\"sat\":\"beta\"}\n" {
		t.Fatalf("active file = %q, want beta content", got)
	}
	if calls := f.calls(); len(calls) != 1 || calls[0] != "restart" {
		t.Fatalf("outpost calls = %v, want exactly [restart]", calls)
	}
}

// The doc recipe registers the command and dispatch reaches the script.
func TestRegisteredDispatch(t *testing.T) {
	f := newFixture(t, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	add := exec.CommandContext(ctx, f.bin,
		"commands", "add", "outpost-profile",
		"--set", "exec.0="+f.bin,
		"--set", "exec.1=--bashsharp",
		"--set", "exec.2="+f.script,
		"--set", "effects.0=read",
		"--set", "effects.1=write",
		"--set", "effects.2=exec",
	)
	add.Env = append(os.Environ(), f.env...)
	add.Dir = f.root
	if out, err := add.CombinedOutput(); err != nil {
		t.Fatalf("commands add: %v (%s)", err, out)
	}
	base := filepath.Join(f.xcfg(), "matrix")
	writeFile(t, filepath.Join(base, "profiles", "alpha", "agent.json"), "{\"sat\":\"alpha\"}\n")
	writeFile(t, filepath.Join(base, "profiles", "beta", "agent.json"), "{\"sat\":\"beta\"}\n")
	writeFile(t, filepath.Join(base, "agent.json"), "{\"sat\":\"alpha\"}\n")

	list := exec.CommandContext(ctx, f.bin, "outpost-profile", "list")
	list.Env = append(os.Environ(), f.env...)
	list.Dir = f.root
	out, err := list.CombinedOutput()
	if err != nil {
		t.Fatalf("dispatch list: %v (%s)", err, out)
	}
	if !strings.Contains(string(out), "* alpha") || !strings.Contains(string(out), "  beta") {
		t.Fatalf("dispatch list = %q, want alpha active", out)
	}
}

// The single Bash# file carries the whole skill: the ~~~skill fence
// names the command, documents every subcommand and the registration
// recipe. Runs on every OS: pure file reads, no fixtures executed.
func TestSkillFenceIsComplete(t *testing.T) {
	script, err := os.ReadFile(filepath.Join(moduleRoot(t), "examples", "outpost-profile", "outpost-profile.bsh"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(script)
	for _, want := range []string{
		"~~~skill as guide",
		"name: outpost-profile",
		"add <name> --code <code> --server <url>",
		"use <name>",
		"list",
		"commands add outpost-profile",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("skill fence missing %q", want)
		}
	}
	if strings.Contains(body, ".local") || strings.Contains(body, "noviwin") {
		t.Error("no hostnames belong in the command or its skill face")
	}
}

// --skill prints the fence guidance, the text-outer skill step.
func TestSkillFlagPrintsGuidance(t *testing.T) {
	f := newFixture(t, nil)
	out, _, code := f.run("--skill")
	if code != 0 {
		t.Fatalf("--skill: exit %d", code)
	}
	if !strings.Contains(out, "name: outpost-profile") || !strings.Contains(out, "outpost-profile use") {
		t.Fatalf("--skill output missing guidance: %q", out)
	}
}
