//go:build !windows

// Command bashysignalprobe verifies a built Bashy artifact's inherited-ignore
// behavior through its sh alias, including the pre-script signal window.
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: bashysignalprobe BASHY")
		os.Exit(2)
	}
	bin, err := filepath.Abs(os.Args[1])
	if err != nil {
		fatal(err)
	}
	dir, err := os.MkdirTemp("", "bashy-signal-probe-")
	if err != nil {
		fatal(err)
	}
	defer os.RemoveAll(dir)
	sh := filepath.Join(dir, "sh")
	if err := os.Symlink(bin, sh); err != nil {
		fatal(err)
	}
	for _, tc := range []struct {
		name string
		sig  syscall.Signal
	}{
		{"ABRT", syscall.SIGABRT}, {"ALRM", syscall.SIGALRM}, {"PIPE", syscall.SIGPIPE},
		{"QUIT", syscall.SIGQUIT}, {"TERM", syscall.SIGTERM}, {"USR1", syscall.SIGUSR1}, {"USR2", syscall.SIGUSR2},
	} {
		cmd := exec.Command("/bin/sh", "-c", `ulimit -c 0; trap '' "$SIGNAL"; exec "$BASHY_SH"`)
		cmd.Env = append(os.Environ(), "BASHY_SH="+sh, "SIGNAL="+tc.name, "PATH=/usr/bin:/bin")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			fatal(err)
		}
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Start(); err != nil {
			fatal(err)
		}
		time.Sleep(200 * time.Millisecond)
		signalErr := cmd.Process.Signal(tc.sig)
		_, writeErr := io.WriteString(stdin, "printf test_string\n")
		_ = stdin.Close()
		waitErr := cmd.Wait()
		if signalErr != nil || writeErr != nil || waitErr != nil || stdout.String() != "test_string" {
			fatal(fmt.Errorf("%s: signal=%v write=%v wait=%v stdout=%q stderr=%q", tc.name, signalErr, writeErr, waitErr, stdout.String(), stderr.String()))
		}
	}
	fmt.Println("bashysignalprobe: PASS seven inherited ignored signals")
}
func fatal(err error) { fmt.Fprintln(os.Stderr, "bashysignalprobe:", err); os.Exit(1) }
