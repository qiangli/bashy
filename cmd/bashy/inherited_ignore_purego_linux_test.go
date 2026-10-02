//go:build linux

package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// The public Linux release and scratch profiles cross-compile with cgo off.
// They must still retain execve-inherited SIG_IGN in the one-file shell route.
func TestPureGoSingleBinaryInheritedIgnoredSignals(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the Linux one-file release profile")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bashy")
	build := exec.Command("go", "build", "-trimpath", "-ldflags=-w", "-o", bin, "./cmd/bashy")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build pure-Go Bashy: %v\n%s", err, out)
	}
	sh := filepath.Join(dir, "sh")
	if err := os.Symlink(bin, sh); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		sig  syscall.Signal
	}{
		{"ABRT", syscall.SIGABRT},
		{"ALRM", syscall.SIGALRM},
		{"PIPE", syscall.SIGPIPE},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command("/bin/sh", "-c", "ulimit -c 0; trap '' "+tc.name+`; exec "$BASHY_SH"`)
			cmd.Env = append(os.Environ(), "BASHY_SH="+sh, "PATH=/usr/bin:/bin")
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			time.Sleep(200 * time.Millisecond)
			signalErr := cmd.Process.Signal(tc.sig)
			_, writeErr := io.WriteString(stdin, "printf test_string\n")
			_ = stdin.Close()
			waitErr := cmd.Wait()
			if signalErr != nil || writeErr != nil || waitErr != nil || stdout.String() != "test_string" {
				t.Fatalf("signal=%v write=%v wait=%v stdout=%q stderr=%q",
					signalErr, writeErr, waitErr, stdout.String(), stderr.String())
			}
		})
	}
}
