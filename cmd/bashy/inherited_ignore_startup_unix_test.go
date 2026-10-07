//go:build linux || darwin

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"
)

// Unlike the elapsed-time smoke tests, this test delivers signals while a
// dependency init is stopped, before route/main can restore dispositions.
// It covers both one-file entries and the native launcher over a pure-Go
// payload. No test-only signal handler or restoration is installed.
func TestInheritedIgnoreDuringStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("builds startup-instrumented executable profiles")
	}
	for _, cgo := range []string{"1", "0"} {
		if cgo == "0" && runtime.GOOS != "linux" {
			continue // one-file snapshot recovery is currently Linux-specific
		}
		t.Run("cgo="+cgo, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bashy")
			build := exec.Command("go", "build", "-tags=startup_signal_probe", "-o", bin, "./cmd/bashy")
			build.Dir = filepath.Join("..", "..")
			build.Env = append(os.Environ(), "CGO_ENABLED="+cgo)
			if out, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build startup probe: %v\n%s", err, out)
			}
			t.Run("single", func(t *testing.T) {
				checkStartupSignals(t, bin)
				checkStartupDefaultTERM(t, bin)
			})
			if cgo == "0" {
				launcher := filepath.Join(dir, "launcher")
				cc := exec.Command("cc", "-x", "c", "-std=c11", "-O2", "-Wall", "-Wextra", "-Werror", "-o", launcher, "native/siglaunch.c.in")
				cc.Dir = filepath.Join("..", "..")
				if out, err := cc.CombinedOutput(); err != nil {
					t.Fatalf("build launcher: %v\n%s", err, out)
				}
				if err := os.Symlink(bin, launcher+".real"); err != nil {
					t.Fatal(err)
				}
				t.Run("launcher", func(t *testing.T) {
					checkStartupSignals(t, launcher)
					checkStartupDefaultTERM(t, launcher)
				})
			}
		})
	}
}

func checkStartupSignals(t *testing.T, bin string) {
	t.Helper()
	for _, sig := range []struct {
		name string
		num  syscall.Signal
	}{{"ABRT", syscall.SIGABRT}, {"QUIT", syscall.SIGQUIT}, {"TERM", syscall.SIGTERM}} {
		t.Run(sig.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, "/bin/sh", "-c",
				`ulimit -c 0; trap '' "$TEST_SIGNAL"; exec "$TEST_BINARY" -c 'trap "echo BAD" "$TEST_SIGNAL"; trap - "$TEST_SIGNAL"; kill -s "$TEST_SIGNAL" $$; printf survived'`)
			cmd.Env = append(os.Environ(), "TEST_BINARY="+bin, "TEST_SIGNAL="+sig.name, "BASHY_TEST_STARTUP_STOP=1")
			// Select the shell-only route using its supported argv[0] alias.
			sh := filepath.Join(t.TempDir(), "sh")
			if err := os.Symlink(bin, sh); err != nil {
				t.Fatal(err)
			}
			cmd.Env = append(cmd.Env, "TEST_BINARY="+sh)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
			if err := waitStartupStop(ctx, cmd.Process.Pid); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Process.Signal(sig.num); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
				t.Fatal(err)
			}
			if err := cmd.Wait(); err != nil || stdout.String() != "survived" {
				t.Fatalf("early %s: wait=%v stdout=%q stderr=%q", sig.name, err, stdout.String(), stderr.String())
			}
		})
	}
}

// A repair must not swallow an ordinary non-shell startup termination signal.
func checkStartupDefaultTERM(t *testing.T, bin string) {
	t.Helper()
	t.Run("default-TERM", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", `ulimit -c 0; trap - TERM; exec "$TEST_BINARY" --version`)
		cmd.Env = append(os.Environ(), "TEST_BINARY="+bin, "BASHY_TEST_STARTUP_STOP=1")
		var output bytes.Buffer
		cmd.Stdout, cmd.Stderr = &output, &output
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		if err := waitStartupStop(ctx, cmd.Process.Pid); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Process.Signal(syscall.SIGCONT); err != nil {
			t.Fatal(err)
		}
		err := cmd.Wait()
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("default TERM: want signal termination, got %v output=%q", err, output.String())
		}
		status, ok := exit.Sys().(syscall.WaitStatus)
		if !ok || !status.Signaled() || status.Signal() != syscall.SIGTERM {
			t.Fatalf("default TERM: want SIGTERM wait status, got %v output=%q", exit.Sys(), output.String())
		}
	})
}

func waitStartupStop(ctx context.Context, pid int) error {
	for {
		var status syscall.WaitStatus
		got, err := syscall.Wait4(pid, &status, syscall.WUNTRACED|syscall.WNOHANG, nil)
		if err != nil {
			return err
		}
		if got == pid {
			// Darwin syscall.WaitStatus classifies the SIGSTOP encoding as
			// Continued. We did not request WCONTINUED; inspect the POSIX
			// stopped status directly on both platforms.
			if uint32(status)&0xff == 0x7f && syscall.Signal(uint32(status)>>8&0xff) == syscall.SIGSTOP {
				return nil
			}
			return fmt.Errorf("startup probe exited before barrier: %v", status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
}
