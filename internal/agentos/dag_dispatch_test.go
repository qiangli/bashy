package agentos

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/shape/slot"
)

// The shipped `bashy dag` is the pkg/dag root WITH the capacity subcommand
// mounted. Sprint 163 found it exiting 1 with zero bytes on both streams for
// every target: cobra rejected the target as an unknown subcommand and
// SilenceErrors + a bare exit-code mapping hid the message. These tests drive
// the exact dispatch shape bashy ships.

func writeTestDAG(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	md := "## Tasks\n\n### hello\nSay hi.\n\n```bash\necho hi-from-dispatch\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "DAG.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(dir, "DAG.md")
}

func TestDagDispatchRunsATargetWithCapacityMounted(t *testing.T) {
	path := writeTestDAG(t)
	var out, errOut bytes.Buffer
	code := runDagDispatch([]string{"--file", path, "hello"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("dag hello exit = %d\nstdout=%s\nstderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "hi-from-dispatch") {
		t.Fatalf("target body did not run\nstdout=%s\nstderr=%s", out.String(), errOut.String())
	}
}

func TestDagDispatchDryRunWithCapacityMounted(t *testing.T) {
	path := writeTestDAG(t)
	var out, errOut bytes.Buffer
	if code := runDagDispatch([]string{"--file", path, "--dry-run", "hello"}, &out, &errOut); code != 0 {
		t.Fatalf("dag --dry-run hello exit = %d stderr=%s", code, errOut.String())
	}
	if !strings.Contains(out.String(), "hello") {
		t.Fatalf("plan did not name the target:\n%s", out.String())
	}
}

// A non-zero exit with an empty stderr is the absence-of-evidence failure
// mode; an error cobra swallows before RunE must still be reported.
func TestDagDispatchNeverExitsSilently(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runDagDispatch([]string{"--no-such-flag"}, &out, &errOut)
	if code == 0 {
		t.Fatal("an unknown flag must fail")
	}
	if !strings.Contains(errOut.String(), "no-such-flag") {
		t.Fatalf("the swallowed cobra error was not reported: stderr=%q", errOut.String())
	}
}

// dag's own errors are already emitted as envelopes by pkg/dag; they must not
// be printed a second time by the dispatcher.
func TestDagDispatchDoesNotDoubleReportDagErrors(t *testing.T) {
	path := writeTestDAG(t)
	var out, errOut bytes.Buffer
	code := runDagDispatch([]string{"--file", path, "nosuch"}, &out, &errOut)
	if code == 0 {
		t.Fatal("unknown target must fail")
	}
	if n := strings.Count(errOut.String(), "nosuch"); n != 1 {
		t.Fatalf("unknown target reported %d times, want exactly once:\n%s", n, errOut.String())
	}
	if strings.Contains(errOut.String(), "bashy dag:") {
		t.Fatalf("a *dag.Error must not also be printed by the dispatcher:\n%s", errOut.String())
	}
}

// A ```bashpp body may declare a `~~~py as py` fence and call py.main(); the
// value comes back into the shell body. This is the shipped dispatch shape
// (capacity mounted), in-process, against a temp task file. Bodies run in the
// invoking cwd, so the test enters the file's directory first.
func TestDagDispatchBashppPyFence(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash++ foreign Python fences are not supported on Windows")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("DAG_CACHE_DIR", filepath.Join(dir, ".cache"))
	md := strings.Join([]string{
		"## Tasks",
		"",
		"### smoke",
		"",
		"```bsh",
		"~~~py as py",
		"def main() -> str:",
		"    return 'launched-from-dispatch'",
		"~~~",
		"value := py.main()",
		`echo "value=$value"`,
		"```",
		"",
	}, "\n")
	path := filepath.Join(dir, "dag.md")
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runDagDispatch([]string{"--file", path, "smoke"}, &out, &errOut); code != 0 {
		t.Fatalf("dag smoke exit = %d\nstdout=%s\nstderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "value=launched-from-dispatch") {
		t.Fatalf("py.main() value did not reach the body\nstdout=%s\nstderr=%s", out.String(), errOut.String())
	}
}

func TestDagDispatchBashppNativeFences(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Bash++ C/C++ native fences require a Unix-like compiler environment")
	}
	if _, err := exec.LookPath("clang"); err != nil {
		t.Skip("clang not on PATH")
	}
	if _, err := exec.LookPath("clang++"); err != nil {
		t.Skip("clang++ not on PATH")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("DAG_CACHE_DIR", filepath.Join(dir, ".cache"))
	md := strings.Join([]string{
		"## Tasks", "", "### smoke", "", "```bashpp",
		"~~~c as cmod", "long long add(long long a, long long b) { return a+b; }", "~~~",
		"~~~cxx as cpp", "#include <string>", `std::string greet(const std::string& name) { return "hello "+name; }`, "~~~",
		"answer := cmod.add(20, 22)", "message := cpp.greet(world)", `echo "$answer:$message"`, "```", "",
	}, "\n")
	path := filepath.Join(dir, "dag.md")
	if err := os.WriteFile(path, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	if code := runDagDispatch([]string{"--file", path, "smoke"}, &out, &errOut); code != 0 {
		t.Fatalf("dag native smoke exit = %d\nstdout=%s\nstderr=%s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String()+errOut.String(), "42:hello world") {
		t.Fatalf("native values did not reach the body\nstdout=%s\nstderr=%s", out.String(), errOut.String())
	}
}

func writePoisonGo(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := "@echo UNEXPECTED GO INVOCATION: %* 1>&2\r\n@exit /b 99\r\n"
		if err := os.WriteFile(filepath.Join(dir, "go.bat"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	script := "#!/bin/sh\necho \"UNEXPECTED GO INVOCATION: $*\" >&2\nexit 99\n"
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func cleanEnvForFreshHome(freshHome, poisonBin string) []string {
	var env []string
	for _, k := range []string{"SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "TMP", "TEMP", "TMPDIR"} {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			env = append(env, k+"="+v)
		}
	}
	sysPath := "/usr/bin:/bin"
	if runtime.GOOS == "windows" {
		sysPath = os.Getenv("PATH")
	}
	env = append(env,
		"PATH="+poisonBin+string(os.PathListSeparator)+sysPath,
		"HOME="+freshHome,
		"USERPROFILE="+freshHome,
		"BASHY_HOME="+filepath.Join(freshHome, ".bashy"),
		"BASHY_HINTS=off",
		"BASHY_OFFLINE=1",
		"GOWORK=off",
	)
	return env
}

// Story #3317 (117bbf2cac2e): an installed (-trimpath, -buildvcs=false) bashy
// invoked from a directory outside any checkout with a fresh HOME and BASHY_HOME
// must run dag.md targets directly without invoking `go` or requiring go.mod.
func TestDagFreshHomeTrimmedBinaryRunsWithoutGo(t *testing.T) {
	root, ok := findBashySourceRoot(".")
	if !ok {
		t.Fatal("cannot locate bashy source root")
	}
	binDir := t.TempDir()
	bin := filepath.Join(binDir, "bashy")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	buildCmd := exec.Command("go", "build", "-buildvcs=false", "-trimpath", "-o", bin, "./cmd/bashy")
	buildCmd.Dir = root
	buildCmd.Env = append(os.Environ(), "GOWORK=off")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build trimmed cmd/bashy: %v\n%s", err, out)
	}

	poisonBin := t.TempDir()
	writePoisonGo(t, poisonBin)

	freshHome := filepath.Join(t.TempDir(), "h")
	if err := os.MkdirAll(freshHome, 0o755); err != nil {
		t.Fatal(err)
	}
	projDir := t.TempDir()
	dagBody := strings.Join([]string{
		"## Tasks",
		"",
		"### hello",
		"```bash",
		"set -e",
		"echo fresh-home-target-ran",
		"val=$(printf '{\"ok\":\"yes\"}' | jq -r '.ok')",
		"printf 'JQ=%s\\nBASHY=%s\\nBASHY_EXE=%s\\nINPROC=%s\\nEXEC=%s\\nSELF=%s\\n' \"$val\" \"$BASHY\" \"$BASHY_EXE\" \"${BASHY_ADAPTER_INPROC-<unset>}\" \"${BASHY_ADAPTER_EXEC-<unset>}\" \"${BASHY_SELF-<unset>}\"",
		"```",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(projDir, "dag.md"), []byte(dagBody), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "dag", "-f", "dag.md", "hello")
	cmd.Dir = projDir
	cmd.Env = cleanEnvForFreshHome(freshHome, poisonBin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	combined := stdout.String() + stderr.String()
	if err != nil {
		t.Fatalf("bashy dag on fresh HOME failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	if strings.Contains(combined, "go.mod file not found") || strings.Contains(combined, "UNEXPECTED GO INVOCATION") {
		t.Fatalf("bashy dag invoked go on fresh HOME:\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
	for _, want := range []string{
		"fresh-home-target-ran",
		"JQ=yes",
		"BASHY=" + bin,
		"BASHY_EXE=" + bin,
		"INPROC=<unset>",
		"EXEC=<unset>",
		"SELF=<unset>",
	} {
		if !strings.Contains(combined, want) {
			t.Errorf("missing %q in fresh-HOME dag output:\nstdout:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
		}
	}

	// Also verify the materialized embedded yoke/dag/dag.bsh adapter fallback
	// when invoked through runCommandAdapter on a fresh HOME with no cached binary.
	origRoots := adapterCandidateRoots
	origFetch := adapterSourceFetch
	origSelect := shapeSlotSelect
	fetches := 0
	adapterCandidateRoots = func() []string { return nil }
	adapterSourceFetch = func(string) error {
		fetches++
		return errors.New("unexpected network source fetch for self-contained dag.bsh")
	}
	shapeSlotSelect = func(string) (slot.Version, bool, error) { return slot.Version{}, false, nil }
	t.Cleanup(func() {
		adapterCandidateRoots = origRoots
		adapterSourceFetch = origFetch
		shapeSlotSelect = origSelect
	})

	freshHome2 := filepath.Join(t.TempDir(), "h2")
	if err := os.MkdirAll(freshHome2, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", freshHome2)
	t.Setenv("USERPROFILE", freshHome2)
	t.Setenv("BASHY_HOME", filepath.Join(freshHome2, ".bashy"))
	t.Setenv("XDG_CACHE_HOME", "")
	t.Setenv("BASHY_ADAPTER_CACHE", "")
	t.Setenv(adaptersRootEnv, "")
	t.Setenv("BASHY_SELF", bin)
	t.Setenv("BASHY_HINTS", "off")
	t.Setenv("BASHY_OFFLINE", "1")
	t.Setenv("GOWORK", "off")
	t.Setenv("PATH", poisonBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Chdir(projDir)
	t.Setenv("PWD", projDir)

	adpOut, adpErr, adpCode := captureCommandAdapterRun(t, "yoke", "dag", "dag", []string{"--no-journal", "--force", "-f", "dag.md", "hello"})
	if adpCode != 0 {
		t.Fatalf("embedded yoke/dag fresh-HOME fallback exited %d\nstdout:\n%s\nstderr:\n%s", adpCode, adpOut, adpErr)
	}
	if fetches != 0 {
		t.Fatalf("embedded yoke/dag triggered %d source fetches, want 0", fetches)
	}
	if !strings.Contains(adpOut+adpErr, "fresh-home-target-ran") {
		t.Fatalf("embedded yoke/dag fresh-HOME fallback did not run target:\nstdout:\n%s\nstderr:\n%s", adpOut, adpErr)
	}
}

func TestDispatchDagHonorsShapeSlotAndAdaptersRootOverride(t *testing.T) {
	origSelect := shapeSlotSelect
	origLaunch := shapeSlotLaunch
	t.Cleanup(func() {
		shapeSlotSelect = origSelect
		shapeSlotLaunch = origLaunch
	})

	slotBin := filepath.Join(t.TempDir(), "dag-shaped")
	if err := os.WriteFile(slotBin, []byte("slot"), 0o755); err != nil {
		t.Fatal(err)
	}
	var gotPath string
	var gotArgs []string
	shapeSlotSelect = func(name string) (slot.Version, bool, error) {
		if name == "dag" {
			return slot.Version{Path: slotBin}, true, nil
		}
		return slot.Version{}, false, nil
	}
	shapeSlotLaunch = func(path string, args []string, _ []string) int {
		gotPath = path
		gotArgs = append([]string(nil), args...)
		return 0
	}
	if code := dispatchDag("dag", []string{"--list"}); code != 0 || gotPath != slotBin || strings.Join(gotArgs, " ") != "dag --list" {
		t.Fatalf("shape slot dispatch: code=%d path=%q args=%v", code, gotPath, gotArgs)
	}
}
