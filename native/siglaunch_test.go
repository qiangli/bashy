//go:build unix

package native_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLauncherPreservesInheritedSignalIgnore(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the shell and native launcher")
	}
	cc, err := exec.LookPath("cc")
	if err != nil {
		t.Skip("cc not installed")
	}
	dir := t.TempDir()
	root := filepath.Clean("..")
	launcher := filepath.Join(dir, "bash")
	payload := launcher + ".real"
	for _, command := range [][]string{
		{"go", "build", "-o", payload, "./cmd/bash"},
		// Keep flags identical to the shipped Make build. GCC's
		// -Wstringop-truncation diagnosis is optimization-sensitive.
		{cc, "-x", "c", "-std=c11", "-O2", "-Wall", "-Wextra", "-Werror", "-o", launcher, "./native/siglaunch.c.in"},
	} {
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", command, err, out)
		}
	}

	// A shell may execute a resolved path while passing the original bare
	// command word as argv[0].  PATH must never be consulted again by the
	// launcher: a Homebrew-like directory earlier than ~/.local/bin can carry
	// a different bash pair without stealing this launcher's adjacent payload.
	shadowBin := filepath.Join(dir, "opt", "homebrew", "bin")
	if err := os.MkdirAll(shadowBin, 0o755); err != nil {
		t.Fatal(err)
	}
	shadow := filepath.Join(shadowBin, "bash")
	if err := os.WriteFile(shadow, []byte("#!/bin/sh\nprintf shadow\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(launcher, "-c", "printf adjacent")
	cmd.Args[0] = "bash"
	cmd.Env = append(os.Environ(), "PATH="+shadowBin+string(os.PathListSeparator)+filepath.Dir(launcher))
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "adjacent" {
		t.Fatalf("bare argv0 with shadowing PATH: err=%v output=%q", err, out)
	}

	// Installation symlinks resolve to the real launcher's directory, so the
	// pair remains intact instead of looking for a nonexistent link.real.
	linkDir := filepath.Join(dir, "links")
	if err := os.Mkdir(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "bash")
	if err := os.Symlink(launcher, link); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(link, "-c", "printf symlink")
	out, err = cmd.CombinedOutput()
	if err != nil || string(out) != "symlink" {
		t.Fatalf("symlink launcher: err=%v output=%q", err, out)
	}

	// Exercise the exact CLI plumbing, not merely an interpreter constructed
	// with a synthetic File.Name. Bash keeps BASH_SOURCE empty for -c, and the
	// indexed default remains safe under nounset.
	cmd = exec.Command(launcher, "-uc", `printf '%s\n' "${BASH_SOURCE[0]:-d}"`)
	out, err = cmd.CombinedOutput()
	if err != nil || string(out) != "d\n" {
		t.Fatalf("bash -c BASH_SOURCE nounset default: err=%v output=%q", err, out)
	}
	cmd = exec.Command(launcher, "-uc", `f(){ printf '%s|%s|%s\n' "${#BASH_SOURCE[@]}" "${BASH_SOURCE[0]:-d}" "$0";}; f`)
	out, err = cmd.CombinedOutput()
	wantCommandFunction := "1|" + launcher + "|" + launcher + "\n"
	if err != nil || string(out) != wantCommandFunction {
		t.Fatalf("bash -c function BASH_SOURCE: err=%v output=%q want=%q", err, out, wantCommandFunction)
	}

	script := filepath.Join(dir, "source-stack.sh")
	scriptBody := `set -u
printf 'top=<%s> n=%s\n' "${BASH_SOURCE[0]:-d}" "${#BASH_SOURCE[@]}"
f() { printf 'func=<%s>|<%s> n=%s\n' "${BASH_SOURCE[0]:-d}" "${BASH_SOURCE[1]:-d}" "${#BASH_SOURCE[@]}"; }
f
`
	if err := os.WriteFile(script, []byte(scriptBody), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(launcher, script)
	out, err = cmd.CombinedOutput()
	want := "top=<" + script + "> n=1\nfunc=<" + script + ">|<" + script + "> n=2\n"
	if err != nil || string(out) != want {
		t.Fatalf("script BASH_SOURCE stack: err=%v output=%q want=%q", err, out, want)
	}

	// Include a Go-runtime fault signal as well as an ordinary async signal.
	// Both dispositions must remain immutable after trap action/reset attempts.
	for _, sig := range []string{"TERM", "USR1", "SEGV"} {
		t.Run(sig, func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c",
				`trap '' "$SIG"; exec "$LAUNCHER" -c 'trap "echo BAD" "$SIG"; trap - "$SIG"; kill -s "$SIG" $$; echo survived'`)
			cmd.Env = append(os.Environ(), "SIG="+sig, "LAUNCHER="+launcher)
			out, err := cmd.CombinedOutput()
			if err != nil || string(out) != "survived\n" {
				t.Fatalf("%s/%s: err=%v output=%q", runtime.GOOS, sig, err, out)
			}
		})
	}

	// Bash's upstream fixtures copy $THIS_SH without knowing that the shipped
	// Unix executable is a launcher/payload pair. The harness sideband keeps a
	// relocated launcher tied to the exact payload selected for the run.
	relocated := filepath.Join(dir, "relocated-shell")
	data, err := os.ReadFile(launcher)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(relocated, data, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(relocated, "-c", "printf relocated")
	cmd.Env = append(os.Environ(), "BASHY_SIGNAL_PAYLOAD="+payload)
	out, err = cmd.CombinedOutput()
	if err != nil || string(out) != "relocated" {
		t.Fatalf("relocated launcher: err=%v output=%q", err, out)
	}
}
