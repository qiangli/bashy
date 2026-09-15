//go:build e2e

package agentos

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// `bashy awd DIR -- CMD` from the front door: runs CMD in DIR, returns CMD's
// own status, and never touches the caller's cwd. Sprint 185 — before it, the
// bare invocation died with `command not found` (127) because awd existed only
// as a builtin inside scripts.
func TestAwdE2EFrontDoor(t *testing.T) {
	bin := bashyBinary(t)
	dir := t.TempDir()
	env := []string{"BASHY_HINTS=off"}

	// pwd is logical (PWD as entered), so compare resolved paths.
	out, stderr, code := runBashyStdEnv(bin, env, "awd", dir, "--", "pwd")
	got, _ := filepath.EvalSymlinks(strings.TrimSpace(out))
	want, _ := filepath.EvalSymlinks(dir)
	if code != 0 || got == "" || got != want {
		t.Fatalf("awd pwd: code=%d out=%q stderr=%q", code, out, stderr)
	}

	// CMD's exit status is the front door's exit status.
	if _, _, code := runBashyStdEnv(bin, env, "awd", dir, "--", "sh", "-c", "exit 7"); code != 7 {
		t.Fatalf("awd exit propagation: code=%d, want 7", code)
	}

	// A missing directory is a failure with a diagnostic, not a silent 0.
	if _, stderr, code := runBashyStdEnv(bin, env, "awd", filepath.Join(dir, "missing"), "--", "pwd"); code == 0 || stderr == "" {
		t.Fatalf("awd missing dir: code=%d stderr=%q", code, stderr)
	}

	// No operands: usage, non-zero.
	if _, stderr, code := runBashyStdEnv(bin, env, "awd"); code == 0 || !strings.Contains(stderr, "usage: bashy awd") {
		t.Fatalf("awd usage: code=%d stderr=%q", code, stderr)
	}
}

// The Sprint 185 shape end to end: a task file kept OUTSIDE the project, a
// ```bashpp target declaring a `~~~py as py` fence and calling py.main(), run
// against the project through `bashy awd PROJECT -- bashy dag -f FILE target`.
// dag bodies run in the invoking cwd (make parity), so the fence's Python sees
// the project as its cwd. Needs a python3.
func TestAwdE2EDrivesDagPyFenceInAnotherDirectory(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	bin := bashyBinary(t)
	project := t.TempDir()
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "marker.txt"), []byte("here"), 0o644); err != nil {
		t.Fatal(err)
	}
	md := strings.Join([]string{
		"## Tasks",
		"",
		"### smoke",
		"Sources: marker.txt",
		"",
		"```bashpp",
		"~~~py as py",
		"def main() -> str:",
		"    import os",
		"    with open('marker.txt') as f:",
		"        return f.read() + ':' + os.path.basename(os.getcwd())",
		"~~~",
		"value := py.main()",
		`echo "smoke=$value"`,
		"```",
		"",
	}, "\n")
	task := filepath.Join(elsewhere, "python.dag.md")
	if err := os.WriteFile(task, []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	env := []string{"BASHY_HINTS=off", "DAG_CACHE_DIR=" + t.TempDir()}
	out, stderr, code := runBashyStdEnv(bin, env, "awd", project, "--", bin, "dag", "-f", task, "smoke")
	if code != 0 || !strings.Contains(out, "smoke=here:"+filepath.Base(project)) {
		t.Fatalf("awd dag py-fence: code=%d out=%q stderr=%q", code, out, stderr)
	}
}
