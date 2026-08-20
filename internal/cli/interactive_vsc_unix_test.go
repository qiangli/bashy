// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package cli

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty/v2"
)

type ptyCapture struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	notify chan struct{}
}

func startPTYCapture(ptmx *os.File) *ptyCapture {
	c := &ptyCapture{notify: make(chan struct{}, 1)}
	go func() {
		buf := make([]byte, 1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				c.mu.Lock()
				c.buf.Write(buf[:n])
				c.mu.Unlock()
				select {
				case c.notify <- struct{}{}:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return c
}

func (c *ptyCapture) waitFor(t *testing.T, want []byte, timeout time.Duration) []byte {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		c.mu.Lock()
		got := append([]byte(nil), c.buf.Bytes()...)
		c.mu.Unlock()
		if bytes.Contains(got, want) {
			return got
		}
		select {
		case <-c.notify:
		case <-timer.C:
			t.Fatalf("PTY output never contained %q; got %q", want, got)
		}
	}
}

func (c *ptyCapture) waitForCount(t *testing.T, want []byte, count int, timeout time.Duration) []byte {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		c.mu.Lock()
		got := append([]byte(nil), c.buf.Bytes()...)
		c.mu.Unlock()
		if bytes.Count(got, want) >= count {
			return got
		}
		select {
		case <-c.notify:
		case <-timer.C:
			t.Fatalf("PTY output contained %d copies of %q, want %d; got %q", bytes.Count(got, want), want, count, got)
		}
	}
}

func TestInteractiveTerminalUIUsesStderrWhenStdoutRedirected(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "sh")
	if err := os.Symlink(builtBashBin(t), sh); err != nil {
		t.Fatal(err)
	}
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()

	var stdout bytes.Buffer
	cmd := exec.Command(sh, "--noprofile", "--norc", "-s", "one", "two")
	cmd.Env = []string{
		"HOME=" + dir,
		"PATH=" + dir + ":/bin:/usr/bin",
		"PS1=$ ",
		"TERM=xterm",
	}
	cmd.Stdin = tty
	cmd.Stdout = &stdout
	cmd.Stderr = tty
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	_ = tty.Close()
	capture := startPTYCapture(ptmx)
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	capture.waitFor(t, []byte("$ "), 3*time.Second)
	if _, err := io.WriteString(ptmx, `printf 'PAYLOAD:%s:%s:%s\n' "$0" "$1" "$2"`+"\r"); err != nil {
		t.Fatal(err)
	}
	capture.waitForCount(t, []byte("$ "), 2, 3*time.Second)
	if _, err := io.WriteString(ptmx, "exit\r"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("interactive shell exit: %v", err)
	}
	want := "PAYLOAD:" + sh + ":one:two\n"
	if got := stdout.String(); got != want {
		t.Fatalf("redirected stdout = %q, want %q", got, want)
	}
}

// TestVSCInteractiveSpawnHandshakeZeroSizePTY reproduces libexpect.exp's
// SpawnSh handshake on the zero-sized PTY Expect creates by default. The
// command echo and CRLF must remain contiguous so its prompt matcher can
// advance to TP718's actual unprefixed-signal assertion.
func TestVSCInteractiveSpawnHandshakeZeroSizePTY(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "sh")
	if err := os.Symlink(builtBashBin(t), sh); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(sh, "--noprofile", "--norc")
	cmd.Env = []string{
		"HOME=" + dir,
		"PATH=" + dir + ":/bin:/usr/bin",
		"PS1=$ ",
		"TERM=xterm",
	}
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{})
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	capture := startPTYCapture(ptmx)
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	// The first prompt follows the bounded, unanswered DSR query.
	capture.waitFor(t, []byte("\x1b[0K"), 3*time.Second)
	const setPrompt = "export PS1='PS1 '"
	if _, err := io.WriteString(ptmx, setPrompt+"\r"); err != nil {
		t.Fatal(err)
	}
	capture.waitFor(t, []byte(setPrompt+"\r\nPS1 "), 3*time.Second)

	if _, err := io.WriteString(ptmx, "exit\r"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("interactive shell exit: %v", err)
	}
}
