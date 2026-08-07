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
}
