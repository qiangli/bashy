//go:build e2e

package agentos

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// TestAwdE2EConcurrentFrontDoorIsolation is the installed-product counterpart
// to sh's Runner-copy regression.  The outer shell launches the *front-door*
// form in concurrent Classic jobs and Bash++ go tasks; each front door then
// re-execs itself through dispatchAwd.  Thus this covers the complete boundary
// (shell copy -> executable -> awd builtin -> executable), rather than merely
// exercising the builtin in an in-process Runner.
//
// Keep this suitable for `go test -race -tags e2e -count=N`: the short sleep in
// the observed command makes both branches overlap, while unique temp paths
// keep repetitions independent.  `make test-awd-installed-stress` supplies an
// actual install as BASHY_E2E_BIN and repeats this test.
func TestAwdE2EConcurrentFrontDoorIsolation(t *testing.T) {
	bin := bashyBinary(t)
	root := t.TempDir()
	parent := filepath.Join(root, "parent")
	left := filepath.Join(root, "left")
	right := filepath.Join(root, "right")
	reports := filepath.Join(root, "reports")
	for _, dir := range []string{parent, left, right, reports} {
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// This command runs inside the innermost bashy process.  PWD/OLDPWD must
	// therefore be the awd branch's state, not a concurrent sibling's or the
	// state retained by the outer parent shell.
	observe := `printf '%s|%s|%s\n' "$1" "$PWD" "$OLDPWD" > "$2/$1"; sleep 0.02`
	awd := func(dir, name string) string {
		return strings.Join([]string{
			shellQuote(bin), "awd", shellQuote(dir), "--", shellQuote(bin), "-c",
			shellQuote(observe), "awd-observe", shellQuote(name), shellQuote(reports),
		}, " ")
	}
	parentState := `printf 'parent|%s|%s\n' "$PWD" "$OLDPWD"`

	tests := []struct {
		name string
		args []string
		src  string
	}{
		{
			name: "Classic async jobs",
			args: []string{"-c"},
			src: strings.Join([]string{
				"cd " + shellQuote(root),
				"pushd " + shellQuote(parent) + " >/dev/null",
				awd(left, "left") + " &",
				awd(right, "right") + " &",
				"wait",
				parentState,
			}, "\n"),
		},
		{
			name: "Bash++ go tasks",
			args: []string{"--bashpp", "-c"},
			src: strings.Join([]string{
				"cd " + shellQuote(root),
				"pushd " + shellQuote(parent) + " >/dev/null",
				"done := make(chan bool)",
				"func leftTask(done) { " + awd(left, "left") + " || return $?; done <- true; }",
				"func rightTask(done) { " + awd(right, "right") + " || return $?; done <- true; }",
				"go leftTask(done)",
				"go rightTask(done)",
				// EOF cancels unfinished tasks. Receive completion before
				// checking parent state and the external report files.
				"firstDone := <-done",
				"secondDone := <-done",
				parentState,
			}, "\n"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			args := append(append([]string{}, tc.args...), tc.src)
			out, stderr, code := runBashyStdEnv(bin, []string{"BASHY_HINTS=off"}, args...)
			if code != 0 {
				t.Fatalf("code=%d out=%q stderr=%q", code, out, stderr)
			}
			if got, want := strings.TrimSpace(out), "parent|"+parent+"|"+root; got != want {
				t.Fatalf("parent cwd/PWD/OLDPWD = %q, want %q", got, want)
			}
			for _, branch := range []string{"left", "right"} {
				got, err := os.ReadFile(filepath.Join(reports, branch))
				if err != nil {
					t.Fatalf("read %s report: %v", branch, err)
				}
				want := branch + "|" + filepath.Join(root, branch) + "|" + parent + "\n"
				if string(got) != want {
					t.Errorf("%s report = %q, want %q", branch, got, want)
				}
			}
		})
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

// Keep task redirection on the installed product path: the product's dry-run
// policy must not turn ordinary awd branch writes into forbidden custom opens.
func TestAwdE2EConcurrentTaskRedirection(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native cooperative task file opens are Unix-only")
	}
	bin := bashyBinary(t)
	root := t.TempDir()
	for _, name := range []string{"parent", "left", "right"} {
		if err := os.Mkdir(filepath.Join(root, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	parent := filepath.Join(root, "parent")
	source := strings.Join([]string{
		"cd " + shellQuote(root),
		"pushd " + shellQuote(parent) + " >/dev/null",
		`observe() { printf '%s|%s|%s|%s\n' "$1" "$PWD" "$OLDPWD" "${DIRSTACK[*]}" > state || return $?; sleep 0.02; }`,
		"done := make(chan bool)",
		"func leftTask(done) { awd " + shellQuote(filepath.Join(root, "left")) + " observe left || return $?; done <- true; }",
		"func rightTask(done) { awd " + shellQuote(filepath.Join(root, "right")) + " observe right || return $?; done <- true; }",
		"go leftTask(done)", "go rightTask(done)",
		"firstDone := <-done", "secondDone := <-done",
		`printf 'parent|%s|%s|%s\n' "$PWD" "$OLDPWD" "${DIRSTACK[*]}"`,
	}, "\n")
	out, stderr, code := runBashyStdEnv(bin, []string{"BASHY_HINTS=off"}, "--bashpp", "-c", source)
	want := "parent|" + parent + "|" + root + "|" + parent + " " + root + "\n"
	if code != 0 || out != want || stderr != "" {
		t.Fatalf("code=%d out=%q want=%q stderr=%q", code, out, want, stderr)
	}
	for _, name := range []string{"left", "right"} {
		dir := filepath.Join(root, name)
		data, err := os.ReadFile(filepath.Join(dir, "state"))
		want := name + "|" + dir + "|" + parent + "|" + dir + " " + root + "\n"
		if err != nil || string(data) != want {
			t.Fatalf("%s report=%q err=%v want=%q", name, data, err, want)
		}
	}
}
