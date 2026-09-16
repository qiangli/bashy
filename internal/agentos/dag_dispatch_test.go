package agentos

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
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
	if code := runDagDispatch([]string{"--file", path, "-n", "hello"}, &out, &errOut); code != 0 {
		t.Fatalf("dag -n hello exit = %d stderr=%s", code, errOut.String())
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
		"```bashpp",
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
