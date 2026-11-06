//go:build unix

package main

import (
	"context"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty/v2"
	"golang.org/x/sys/unix"
)

func TestFixtureTerminalControl(t *testing.T) {
	if scenario := os.Getenv("BASH53_TERMINAL_SCENARIO"); scenario != "" {
		if scenario == "restore-ownership" {
			terminal, err := openFixtureTerminal()
			if err != nil || terminal == nil {
				t.Fatalf("open terminal: %v", err)
			}
			fixture := exec.Command("sleep", "120")
			fixture.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			other := exec.Command("sleep", "120")
			other.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := fixture.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = fixture.Process.Kill(); _ = fixture.Wait() }()
			if err := other.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = other.Process.Kill(); _ = other.Wait() }()
			f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			owner := terminal.owner
			defer func() {
				ignored := signal.Ignored(syscall.SIGTTOU)
				signal.Ignore(syscall.SIGTTOU)
				_ = unix.IoctlSetPointerInt(int(f.Fd()), unix.TIOCSPGRP, owner)
				if !ignored {
					signal.Reset(syscall.SIGTTOU)
				}
			}()
			if err := terminal.handoff(fixture.Process.Pid); err != nil {
				t.Fatal(err)
			}
			ignored := signal.Ignored(syscall.SIGTTOU)
			signal.Ignore(syscall.SIGTTOU)
			err = unix.IoctlSetPointerInt(int(f.Fd()), unix.TIOCSPGRP, other.Process.Pid)
			if !ignored {
				signal.Reset(syscall.SIGTTOU)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := terminal.restore(); err == nil || !strings.Contains(err.Error(), "unrelated owner") {
				t.Fatalf("expected unrelated owner rejection, got %v", err)
			}
			got, err := unix.IoctlGetInt(int(f.Fd()), unix.TIOCGPGRP)
			if err != nil || got != other.Process.Pid {
				t.Fatalf("unrelated owner changed to %d, want %d: %v", got, other.Process.Pid, err)
			}
			if err := other.Process.Signal(syscall.Signal(0)); err != nil {
				t.Fatalf("unrelated process signaled: %v", err)
			}
			return
		}
		if scenario == "ownership" {
			other := exec.Command("sleep", "120")
			other.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			if err := other.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = other.Process.Kill(); _ = other.Wait() }()
			f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			owner := syscall.Getpgrp()
			if err := unix.IoctlSetPointerInt(int(f.Fd()), unix.TIOCSPGRP, other.Process.Pid); err != nil {
				t.Fatal(err)
			}
			defer func() {
				ignored := signal.Ignored(syscall.SIGTTOU)
				signal.Ignore(syscall.SIGTTOU)
				_ = unix.IoctlSetPointerInt(int(f.Fd()), unix.TIOCSPGRP, owner)
				if !ignored {
					signal.Reset(syscall.SIGTTOU)
				}
			}()
			if _, err := openFixtureTerminal(); err == nil || !strings.Contains(err.Error(), "belongs to process group") {
				t.Fatalf("expected foreign terminal owner rejection, got %v", err)
			}
			return
		}
		if scenario == "disposition-default" || scenario == "disposition-ignored" {
			ignored := scenario == "disposition-ignored"
			if ignored {
				signal.Ignore(syscall.SIGTTOU)
			}
			bash, err := exec.LookPath("bash")
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			for i, script := range []string{"exit 0\n", "trap -p TTOU\n"} {
				want := ""
				if i == 1 && ignored {
					want = "trap -- '' SIGTTOU\n"
				}
				if err := os.WriteFile(filepath.Join(dir, "terminal.sh"), []byte(script), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "terminal.right"), []byte(want), 0600); err != nil {
					t.Fatal(err)
				}
				status, detail, err := runFixtureContext(context.Background(), dir, dir, bash, fixture{Name: "terminal", Test: "terminal.sh", Right: "terminal.right"}, 5*time.Second)
				if err != nil || status != "PASS" {
					t.Fatalf("fixture %d: status=%s detail=%s err=%v", i, status, detail, err)
				}
			}
			if got := signal.Ignored(syscall.SIGTTOU); got != ignored {
				t.Fatalf("runner SIGTTOU ignored=%t, want %t", got, ignored)
			}
			return
		}
		dir := t.TempDir()
		script := "echo $$ > leader.pid\nsleep 120 &\necho $! > child.pid\nwait\n"
		if scenario == "no-tty" {
			script = "exit 0\n"
		}
		if scenario == "read" {
			script = "printf 'READY\\n' > /dev/tty\nIFS= read -r line < /dev/tty\nprintf '%s\\n' \"$line\"\n"
		}
		if err := os.WriteFile(filepath.Join(dir, "terminal.sh"), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		want := []byte(nil)
		if scenario == "read" {
			want = []byte("hello\n")
		}
		if err := os.WriteFile(filepath.Join(dir, "terminal.right"), want, 0o644); err != nil {
			t.Fatal(err)
		}
		var owner int
		terminal, err := openFixtureTerminal()
		if err != nil {
			t.Fatal(err)
		}
		if scenario == "no-tty" {
			if terminal != nil {
				t.Fatal("headless process acquired a controlling terminal")
			}
		} else {
			if terminal == nil {
				t.Fatal("PTY process has no controlling terminal")
			}
			owner = terminal.owner
			_ = terminal.file.Close()
		}
		ctx := context.Background()
		if scenario == "cancel" {
			var cancel context.CancelFunc
			ctx, cancel = context.WithCancel(ctx)
			defer cancel()
			stop := time.AfterFunc(100*time.Millisecond, cancel)
			defer stop.Stop()
		}
		timeout := 5 * time.Second
		if scenario == "timeout" {
			timeout = 100 * time.Millisecond
		}
		sh, err := exec.LookPath("sh")
		if err != nil {
			t.Fatal(err)
		}
		status, _, err := runFixtureContext(ctx, dir, dir, sh, fixture{Name: "terminal", Test: "terminal.sh", Right: "terminal.right"}, timeout)
		switch scenario {
		case "read", "no-tty":
			if err != nil || status != "PASS" {
				t.Fatalf("status=%s err=%v", status, err)
			}
		case "timeout":
			if err != nil || status != "TIME" {
				t.Fatalf("status=%s err=%v", status, err)
			}
		case "cancel":
			if err != context.Canceled {
				t.Fatalf("status=%s err=%v", status, err)
			}
		}
		if scenario == "timeout" || scenario == "cancel" {
			for _, name := range []string{"leader.pid", "child.pid"} {
				pid := readFixturePID(t, filepath.Join(dir, name), time.Second)
				if !waitFixtureGone(pid, 2*time.Second) {
					t.Errorf("%s process %d survived %s", name, pid, scenario)
				}
			}
		}
		if scenario != "no-tty" {
			f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			got, err := unix.IoctlGetInt(int(f.Fd()), unix.TIOCGPGRP)
			if err != nil || got != owner {
				t.Fatalf("restored foreground group=%d, want %d, err=%v", got, owner, err)
			}
		}
		return
	}
	for _, scenario := range []string{"read", "timeout", "cancel", "ownership", "restore-ownership", "no-tty", "disposition-default", "disposition-ignored"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestFixtureTerminalControl$", "-test.v=false")
			cmd.Env = append(os.Environ(), "BASH53_TERMINAL_SCENARIO="+scenario)
			var master *os.File
			var terminalOutput <-chan []byte
			var err error
			if scenario == "no-tty" {
				cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
				cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
				err = cmd.Start()
			} else {
				master, err = pty.Start(cmd)
			}
			if err != nil {
				t.Fatal(err)
			}
			if master != nil {
				defer master.Close()
				if scenario == "read" {
					ready := make(chan string, 1)
					go func() {
						buf := make([]byte, 256)
						n, _ := master.Read(buf)
						ready <- string(buf[:n])
					}()
					select {
					case output := <-ready:
						if !strings.Contains(output, "READY") {
							t.Fatalf("PTY readiness=%q", output)
						}
					case <-time.After(5 * time.Second):
						_ = cmd.Process.Kill()
						t.Fatal("fixture never reached terminal read")
					}
					_, _ = master.Write([]byte("hello\n"))
				}
				drained := make(chan []byte, 1)
				terminalOutput = drained
				go func() {
					output, _ := io.ReadAll(master)
					drained <- output
				}()
			}
			waitErr := cmd.Wait()
			var output []byte
			if terminalOutput != nil {
				select {
				case output = <-terminalOutput:
				case <-time.After(2 * time.Second):
					_ = master.Close()
					output = <-terminalOutput
				}
			}
			if waitErr != nil {
				t.Fatalf("%v\n%s", waitErr, output)
			}
		})
	}
}
