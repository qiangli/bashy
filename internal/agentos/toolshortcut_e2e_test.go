// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

// E2E for `bashy <tool>` (toolshortcut.go): the merged shortcut launches the
// vendor CLI through the `bashy chat` steerable path as a live session whose
// id is the tool name, reachable via `bashy chat steer`.
//
// ZERO TOKENS: no LLM, no network. The vendor CLIs are tiny POSIX sh fakes
// that log their argv, echo every input line, and exit on EOF or `quit`,
// bound via BASHY_TOOL_BINARY_<TOOL>. HOME and BASHY_HOME are hermetic temp
// dirs, so the host room/board is untouched.

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty/v2"
)

// writeToolShortcutFake writes a fake vendor CLI named tool into dir and
// returns its path. The fake appends its argv line to argvLog, echoes every
// stdin line back, and exits 0 on EOF or a `quit` line.
func writeToolShortcutFake(t *testing.T, dir, tool, argvLog string) string {
	t.Helper()
	path := filepath.Join(dir, tool)
	// tr -d '\015' strips a trailing CR: steer frames arrive as text + CR LF
	// over the PTY, and read keeps the CR without it.
	body := "#!/bin/sh\n" +
		"log=" + argvLog + "\n" +
		"printf '%s\\n' \"$*\" >> \"$log\"\n" +
		// Draw a prompt like a real TUI so bashy sees the session become ready
		// and delivers queued steers at once (yoke 56346984/97b765a2) instead
		// of after the bounded readiness timeout.
		"printf '> '\n" +
		"while IFS= read -r line; do\n" +
		"  line=$(printf '%s' \"$line\" | tr -d '\\015')\n" +
		"  printf '%s\\n' \"$line\"\n" +
		"  case \"$line\" in quit) exit 0;; esac\n" +
		"done\n" +
		"exit 0\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// runToolShortcutBin runs the built bashy binary with a bounded timeout,
// inheriting the test's (hermetic) environment.
func runToolShortcutBin(t *testing.T, bin, work string, stdinStr string, timeout time.Duration, args ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = work
	cmd.Env = os.Environ()
	if stdinStr != "" {
		cmd.Stdin = strings.NewReader(stdinStr)
	} else {
		cmd.Stdin = strings.NewReader("")
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("%s %s did not finish within %s\nstdout=%s\nstderr=%s",
			filepath.Base(bin), strings.Join(args, " "), timeout, out.String(), errb.String())
	}
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("%s %s: %v\nstdout=%s\nstderr=%s", bin, strings.Join(args, " "), err, out.String(), errb.String())
	}
	return out.String(), errb.String(), code
}

func readArgvLog(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(b))
}

// toolShortcutPs snapshots the session child's process tree for timeout
// diagnostics: it tells a stuck spawn (no vendor child) apart from a vendor
// child that never logged.
func toolShortcutPs(pid int) string {
	if pid <= 0 {
		return "no child pid"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	script := "ps -o pid,ppid,stat,command -p " + strconv.Itoa(pid) +
		"; pgrep -P " + strconv.Itoa(pid) + " | xargs ps -o pid,ppid,stat,command -p 2>/dev/null"
	out, _ := exec.CommandContext(ctx, "sh", "-c", script).CombinedOutput()
	return string(out)
}

// waitForToolShortcut polls fn until it reports true or the deadline passes.
func waitForToolShortcut(t *testing.T, what string, timeout time.Duration, fn func() (string, bool)) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		got, ok := fn()
		if ok {
			return got
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s; last output:\n%s", what, got)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// runToolShortcutPTY starts `bashy <tool> [toolArgs...]` under a real PTY,
// waits for the chat session, steers PING into it, observes the echo, then
// quits the fake. It returns the full PTY transcript and the fake's argv
// line. Callers assert yolo presence/absence on the argv.
func runToolShortcutPTY(t *testing.T, bin, work, tool string, toolArgs []string, argvLog string) (string, string) {
	t.Helper()
	if err := os.WriteFile(argvLog, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	args := append([]string{tool}, toolArgs...)
	cmd := exec.Command(bin, args...)
	cmd.Dir = work
	cmd.Env = os.Environ()
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Skipf("no PTY on this host: %v", err)
	}
	defer ptmx.Close()

	// Watch the child from the start: a launch that dies after claiming its
	// room card (failed bind, failed spawn) fails fast here instead of
	// masquerading as a slow spawn at the argv wait below. exitDone is a
	// broadcast close so every poll can check it without consuming the result.
	var exitErr error
	exitDone := make(chan struct{})
	go func() { exitErr = cmd.Wait(); close(exitDone) }()
	exited := func() (error, bool) {
		select {
		case <-exitDone:
			return exitErr, true
		default:
			return nil, false
		}
	}

	var mu sync.Mutex
	var transcript bytes.Buffer
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				mu.Lock()
				transcript.Write(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	snapshot := func() string {
		mu.Lock()
		defer mu.Unlock()
		return transcript.String()
	}
	childState := func() string {
		if err, done := exited(); done {
			return "child exited: " + err.Error()
		}
		return "child alive"
	}

	// (1) launched via the chat steerable path, (2) session id is the tool name.
	sessions := waitForToolShortcut(t, tool+" session in chat sessions", 45*time.Second, func() (string, bool) {
		if err, done := exited(); done {
			return snapshot() + "\n--- child exited while waiting for session: " + err.Error(), false
		}
		out, _, code := runToolShortcutBin(t, bin, work, "", 15*time.Second, "chat", "sessions")
		if code != 0 {
			return out, false
		}
		if strings.Contains(out, tool) {
			return out, true
		}
		return snapshot() + "\n--- sessions ---\n" + out + "\n--- " + childState(), false
	})
	_ = sessions
	got := snapshot()
	for _, want := range []string{"chat: ", "steer " + tool, "id " + tool} {
		if !strings.Contains(got, want) {
			t.Fatalf("PTY transcript lacks %q (steerable chat path):\n%s", want, got)
		}
	}

	// The fake starts with the session: its argv line lands promptly. Any
	// write counts — `agy --no-yolo` strips every flag, so its line is a bare
	// newline and TrimSpace is empty by design (absence asserted below).
	argv := waitForToolShortcut(t, tool+" argv log ("+childState()+")", 30*time.Second, func() (string, bool) {
		if err, done := exited(); done {
			return snapshot() + "\n--- child exited while waiting for argv: " + err.Error(), false
		}
		b, err := os.ReadFile(argvLog)
		if err != nil {
			return err.Error(), false
		}
		if len(b) == 0 {
			diag := snapshot()
			if cmd.Process != nil {
				diag += "\n--- ps ---\n" + toolShortcutPs(cmd.Process.Pid)
			}
			return diag, false
		}
		return strings.TrimSpace(string(b)), true
	})

	// (3) `bashy chat steer <tool> PING` reaches it: the fake echoes the line.
	pingSteerAt := time.Now()
	_, steerErr, steerCode := runToolShortcutBin(t, bin, work, "", 15*time.Second, "chat", "steer", tool, "PING")
	if steerCode != 0 {
		t.Fatalf("chat steer %s PING exit=%d\nstderr=%s\ntranscript:\n%s", tool, steerCode, steerErr, snapshot())
	}
	// A steer to a session that is still starting is queued and delivered once
	// it is ready (yoke 56346984), so either confirmation is valid; the echo
	// check below proves the line actually arrived.
	if !strings.Contains(steerErr, "steered "+tool) && !strings.Contains(steerErr, "steering queued for "+tool) {
		t.Fatalf("chat steer did not confirm %q:\nstderr=%s", tool, steerErr)
	}
	waitForToolShortcut(t, "PING echo from "+tool, 20*time.Second, func() (string, bool) {
		out := snapshot()
		// PING appears twice: once as the control channel types it (input
		// echo) and once when the fake reads the submitted line and echoes
		// it. A single occurrence proves nothing about the tool — the
		// second proves the tool acted on the steer.
		if strings.Count(out, "PING") >= 2 {
			return out, true
		}
		return out, false
	})
	// The control loop resubmits Enter ~3s after every steer (steerResubmitDelay:
	// a second Enter for TUIs that drop the first while busy). A quit steered
	// into that window is typed concurrently with the resubmit, lands in one
	// canonical buffer as `PINGquit`, and never matches — so the quit waits
	// until the PING cycle (including its resubmits) has fully flushed.
	if wait := 5*time.Second - time.Since(pingSteerAt); wait > 0 {
		time.Sleep(wait)
	}

	// Quit through the same steer channel (not a raw master write, whose line
	// ending the slave may not treat as a submit): the fake exits on `quit`.
	_, quitErr, quitCode := runToolShortcutBin(t, bin, work, "", 15*time.Second, "chat", "steer", tool, "quit")
	if quitCode != 0 {
		t.Fatalf("chat steer %s quit exit=%d\nstderr=%s\ntranscript:\n%s", tool, quitCode, quitErr, snapshot())
	}
	// exitDone already watches cmd.Wait from session start; reuse it here.
	select {
	case <-exitDone:
		if exitErr != nil {
			// The fake exits 0; the launcher may still report the session end
			// non-zero only on failure — surface the transcript on failure.
			if ee, ok := exitErr.(*exec.ExitError); !ok || ee.ExitCode() != 0 {
				t.Fatalf("bashy %s exited: %v\ntranscript:\n%s", tool, exitErr, snapshot())
			}
		}
	case <-time.After(25 * time.Second):
		_ = cmd.Process.Kill()
		<-exitDone // reap before the deferred Close; avoids a zombie
		t.Fatalf("bashy %s did not exit after quit\ntranscript:\n%s", tool, snapshot())
	}
	<-done
	final := snapshot()
	// Let the room card clear before a same-id session starts.
	waitForToolShortcut(t, tool+" session to leave", 15*time.Second, func() (string, bool) {
		out, _, _ := runToolShortcutBin(t, bin, work, "", 15*time.Second, "chat", "sessions")
		if strings.Contains(out, tool+" ") || strings.Contains(out, " "+tool+"\n") || strings.Contains(out, tool) && strings.Contains(out, "interactive") {
			return out, false
		}
		return out, true
	})
	return final, argv
}

func TestToolShortcutE2E(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake vendor CLIs are POSIX sh scripts")
	}
	if master, slave, err := pty.Open(); err != nil {
		t.Skipf("no PTY on this host: %v", err)
	} else {
		_ = master.Close()
		_ = slave.Close()
	}
	bin, _ := synergyBashyBinary(t)

	// A short HOME: the chat control socket lives at $HOME/.bashy/ctl/<hash>.sock
	// and unix socket paths cap near 104 bytes — a t.TempDir HOME under
	// /var/folders/... blows past it, the bind fails, and steering degrades.
	// /tmp keeps the whole socket path under ~60 bytes.
	home, err := os.MkdirTemp("/tmp", "bshye2e")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	work := filepath.Join(home, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	fakeDir := filepath.Join(home, "fakes")
	if err := os.MkdirAll(fakeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	argvAgy := filepath.Join(home, "argv-agy.log")
	argvCodex := filepath.Join(home, "argv-codex.log")
	fakeAgy := writeToolShortcutFake(t, fakeDir, "agy", argvAgy)
	fakeCodex := writeToolShortcutFake(t, fakeDir, "codex", argvCodex)

	// Hermetic operator: temp HOME and BASHY_HOME, fakes bound by tool.
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("BASHY_HOME", filepath.Join(home, "bashy-home"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".local", "share"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	t.Setenv("BASHY_HINTS", "off")
	t.Setenv("BASHY_TOOL_BINARY_AGY", fakeAgy)
	t.Setenv("BASHY_TOOL_BINARY_CODEX", fakeCodex)
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if strings.TrimSpace(os.Getenv("TERM")) == "" {
		t.Setenv("TERM", "xterm-256color")
	}

	// agy with the default yolo: the bypass flag reaches the vendor CLI.
	_, argv := runToolShortcutPTY(t, bin, work, "agy", nil, argvAgy)
	if !strings.Contains(argv, "--dangerously-skip-permissions") {
		t.Fatalf("agy default argv lacks the yolo flag, want --dangerously-skip-permissions in %q", argv)
	}

	// agy with --no-yolo: the bypass flag is gone.
	_, argv = runToolShortcutPTY(t, bin, work, "agy", []string{"--no-yolo"}, argvAgy)
	if strings.Contains(argv, "--dangerously-skip-permissions") {
		t.Fatalf("agy --no-yolo argv still carries the yolo flag: %q", argv)
	}

	// codex through the same steerable path. codex forces yolo in yoke
	// (agentlaunch.FinalizeArgs codexDefaultYolo), so the presence of its
	// bypass flag is yoke policy, not the shortcut's --yolo default — noted
	// here and deliberately not asserted.
	_, _ = runToolShortcutPTY(t, bin, work, "codex", nil, argvCodex)

	// (5) non-TTY launch refuses with guidance instead of starting a TUI that
	// exits at once. Stdin here is a pipe, never a terminal.
	for _, tool := range []string{"agy", "codex"} {
		_, stderr, code := runToolShortcutBin(t, bin, work, "", 30*time.Second, tool)
		if code == 0 {
			t.Fatalf("bashy %s without a TTY exited 0, want refusal", tool)
		}
		for _, want := range []string{"not a TTY", "bashy chat --agent " + tool} {
			if !strings.Contains(stderr, want) {
				t.Fatalf("bashy %s non-TTY refusal lacks %q:\nstderr=%s", tool, want, stderr)
			}
		}
	}

	// (6) `bashy <tool> -- ARGS` is a raw passthrough: the fake sees --version
	// unchanged, with no session and no governance flags added.
	for _, tc := range []struct{ tool, log string }{{"agy", argvAgy}, {"codex", argvCodex}} {
		if err := os.WriteFile(tc.log, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, code := runToolShortcutBin(t, bin, work, "", 30*time.Second, tc.tool, "--", "--version")
		if code != 0 {
			t.Fatalf("bashy %s -- --version exit=%d, want 0", tc.tool, code)
		}
		if got := readArgvLog(t, tc.log); got != "--version" {
			t.Fatalf("bashy %s -- --version reached the fake as %q, want exactly --version", tc.tool, got)
		}
	}
}
