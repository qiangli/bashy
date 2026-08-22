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
	"strings"
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

func TestInteractiveShUsesPosixStartupAndPrompt(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "sh")
	if err := os.Symlink(builtBashBin(t), sh); err != nil {
		t.Fatal(err)
	}
	bashrc := filepath.Join(dir, ".bashrc")
	if err := os.WriteFile(bashrc, []byte("echo BASHRC_MARKER\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(dir, "env")
	if err := os.WriteFile(envFile, []byte("echo ENV_MARKER\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(sh)
	cmd.Env = []string{
		"ENV=" + envFile,
		"HOME=" + dir,
		"PATH=" + dir + ":/bin:/usr/bin",
		"TERM=xterm",
	}
	ptmx, err := pty.Start(cmd)
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

	got := capture.waitFor(t, []byte("ENV_MARKER"), 3*time.Second)
	got = capture.waitFor(t, []byte("sh-"), 3*time.Second)
	if bytes.Contains(got, []byte("BASHRC_MARKER")) {
		t.Fatalf("interactive sh sourced .bashrc: %q", got)
	}
	if _, err := io.WriteString(ptmx, "exit\r"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("interactive shell exit: %v", err)
	}
}

func TestInteractivePosixStartupBoundaries(t *testing.T) {
	dir := t.TempDir()
	sh := filepath.Join(dir, "sh")
	bash := filepath.Join(dir, "bash")
	for _, name := range []string{sh, bash} {
		if err := os.Symlink(builtBashBin(t), name); err != nil {
			t.Fatal(err)
		}
	}
	bashrc := filepath.Join(dir, ".bashrc")
	if err := os.WriteFile(bashrc, []byte("echo BASHRC_MARKER\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	envFile := filepath.Join(dir, "env")
	if err := os.WriteFile(envFile, []byte("echo ENV_MARKER\nPS1='CUSTOM_PS1> '\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bashEnv := filepath.Join(dir, "bash-env")
	if err := os.WriteFile(bashEnv, []byte("echo BASH_ENV_MARKER\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("explicit PS1 from ENV", func(t *testing.T) {
		cmd := exec.Command(sh)
		cmd.Env = startupBoundaryEnv(dir, envFile, bashEnv)
		ptmx, err := pty.Start(cmd)
		if err != nil {
			t.Fatal(err)
		}
		defer ptmx.Close()
		capture := startPTYCapture(ptmx)
		got := capture.waitFor(t, []byte("CUSTOM_PS1> "), 3*time.Second)
		if bytes.Contains(got, []byte("BASHRC_MARKER")) {
			t.Fatalf("interactive sh sourced .bashrc: %q", got)
		}
		if _, err := io.WriteString(ptmx, "exit\r"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("interactive shell exit: %v", err)
		}
	})

	t.Run("explicit POSIX bash uses ENV", func(t *testing.T) {
		cmd := exec.Command(bash, "--posix")
		cmd.Env = startupBoundaryEnv(dir, envFile, bashEnv)
		ptmx, err := pty.Start(cmd)
		if err != nil {
			t.Fatal(err)
		}
		defer ptmx.Close()
		capture := startPTYCapture(ptmx)
		got := capture.waitFor(t, []byte("CUSTOM_PS1> "), 3*time.Second)
		if bytes.Contains(got, []byte("BASHRC_MARKER")) {
			t.Fatalf("interactive --posix bash sourced .bashrc: %q", got)
		}
		if _, err := io.WriteString(ptmx, "exit\r"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("interactive shell exit: %v", err)
		}
	})

	t.Run("noninteractive sh skips ENV and BASH_ENV", func(t *testing.T) {
		cmd := exec.Command(sh)
		cmd.Env = startupBoundaryEnv(dir, envFile, bashEnv)
		cmd.Stdin = strings.NewReader("printf BODY")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("noninteractive sh: %v: %s", err, out)
		}
		if string(out) != "BODY" {
			t.Fatalf("noninteractive sh startup output = %q, want BODY", out)
		}
	})

	t.Run("login sh uses profile not bash profiles", func(t *testing.T) {
		for name, marker := range map[string]string{
			".profile":      "PROFILE_MARKER",
			".bash_profile": "BASH_PROFILE_MARKER",
			".bash_login":   "BASH_LOGIN_MARKER",
		} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("echo "+marker+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command(sh, "--login")
		cmd.Env = startupBoundaryEnv(dir, envFile, bashEnv)
		cmd.Stdin = strings.NewReader("printf BODY")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("login sh: %v: %s", err, out)
		}
		text := string(out)
		if !strings.Contains(text, "PROFILE_MARKER") ||
			strings.Contains(text, "BASH_PROFILE_MARKER") ||
			strings.Contains(text, "BASH_LOGIN_MARKER") {
			t.Fatalf("wrong login sh startup files: %q", text)
		}
	})

	t.Run("interactive login sh adds ENV after profile", func(t *testing.T) {
		cmd := exec.Command(sh, "--login")
		cmd.Env = startupBoundaryEnv(dir, envFile, bashEnv)
		ptmx, err := pty.Start(cmd)
		if err != nil {
			t.Fatal(err)
		}
		defer ptmx.Close()
		capture := startPTYCapture(ptmx)
		got := capture.waitFor(t, []byte("CUSTOM_PS1> "), 3*time.Second)
		if !bytes.Contains(got, []byte("PROFILE_MARKER")) ||
			bytes.Contains(got, []byte("BASH_PROFILE_MARKER")) ||
			bytes.Contains(got, []byte("BASH_LOGIN_MARKER")) {
			t.Fatalf("wrong interactive login sh startup files: %q", got)
		}
		if _, err := io.WriteString(ptmx, "exit\r"); err != nil {
			t.Fatal(err)
		}
		if err := cmd.Wait(); err != nil {
			t.Fatalf("interactive login shell exit: %v", err)
		}
	})
}

func startupBoundaryEnv(home, envFile, bashEnv string) []string {
	var env []string
	for _, entry := range os.Environ() {
		name := strings.SplitN(entry, "=", 2)[0]
		switch name {
		case "HOME", "ENV", "BASH_ENV", "PS1", "POSIXLY_CORRECT", "POSIX_PEDANTIC", "SHELLOPTS":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "HOME="+home, "ENV=$HOME/"+filepath.Base(envFile), "BASH_ENV="+bashEnv, "TERM=xterm")
}
