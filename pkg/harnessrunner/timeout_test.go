//go:build unix

package harnessrunner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A command whose external child leaves a long-lived grandchild behind (a
// reproduction script that starts a real server) must still end at the
// admitted wall time: the whole process tree is killed, Execute returns the
// partial output with a timed-out outcome, and nothing is leaked. Sprint 322:
// genie's execute waited 2h13m on Django runserver because the deadline only
// signalled the direct child and Wait then blocked on the pipe the grandchild
// still held.
func TestExecuteWallTimeKillsGrandchildrenAndReturnsPartialOutput(t *testing.T) {
	t.Setenv("BASHY_SELF", filepath.Join(t.TempDir(), "no-bashy-self"))
	manager, _ := newTestManager(t)
	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	script := `@effects("read,write,exec")
function serve() {
  /bin/sh -c 'sleep 300 & echo $! > "$1"; echo started; wait' serve ` + pidFile + `
}
serve`
	req := compileRequest(t, script)
	req.Limits.WallTimeMs = 1000
	req = authorizeRequest(t, manager, req)

	done := make(chan Result, 1)
	begin := time.Now()
	go func() { done <- manager.Execute(context.Background(), req) }()
	var result Result
	select {
	case result = <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("execute did not return after its wall time: the timeout was not enforced")
	}
	elapsed := time.Since(begin)
	if elapsed > 8*time.Second {
		t.Fatalf("execute returned after %v, want wall time + a short grace", elapsed)
	}
	if result.Outcome != OutcomeTimedOut {
		t.Fatalf("outcome = %q, want %q (%#v)", result.Outcome, OutcomeTimedOut, result)
	}
	if got := string(outputBytes(t, result.Output.Stdout)); !strings.Contains(got, "started") {
		t.Fatalf("partial stdout = %q, want the output produced before the deadline", got)
	}
	raw, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatalf("grandchild pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		t.Fatalf("grandchild pid = %q", raw)
	}
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("grandchild %d survived the timeout (leaked process)", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// A descendant that leaves the command's process group (a daemonizing
// server) escapes the group kill and keeps the output pipe open. The job
// must still settle shortly after its wall time: abandoned, timed out, with
// the output of the commands that did finish and a stderr line saying so.
// (The unfinished command's own output is withheld: bashy's output reducer
// redacts an external command's stream only once it is complete.)
func TestExecuteAbandonsCommandThatOutlivesItsWallTime(t *testing.T) {
	if _, err := exec.LookPath("perl"); err != nil {
		t.Skip("perl not available to start a process-group escapee")
	}
	t.Setenv("BASHY_SELF", filepath.Join(t.TempDir(), "no-bashy-self"))
	previous := abandonGrace
	abandonGrace = 500 * time.Millisecond
	t.Cleanup(func() { abandonGrace = previous })
	manager, _ := newTestManager(t)
	pidFile := filepath.Join(t.TempDir(), "escapee.pid")
	script := `@effects("read,write,exec")
function serve() {
  echo started
  /bin/sh -c 'perl -e "setpgrp(0,0); open(F, \">\", \$ARGV[0]); print F \$\$; close F; sleep 30" "$1" & wait' serve ` + pidFile + `
}
serve`
	req := compileRequest(t, script)
	req.Limits.WallTimeMs = 1000
	req = authorizeRequest(t, manager, req)
	// Runs before the TempDir removals (cleanups are LIFO): end the escapee
	// and let the abandoned command finish with the directories it uses.
	t.Cleanup(func() {
		if raw, err := os.ReadFile(pidFile); err == nil {
			if pid, _ := strconv.Atoi(strings.TrimSpace(string(raw))); pid > 0 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
		abandonedSessions.Wait()
	})

	done := make(chan Result, 1)
	begin := time.Now()
	go func() { done <- manager.Execute(context.Background(), req) }()
	var result Result
	select {
	case result = <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("execute did not settle: the abandon backstop did not fire")
	}
	if elapsed := time.Since(begin); elapsed > 8*time.Second {
		t.Fatalf("execute settled after %v", elapsed)
	}
	if result.Outcome != OutcomeTimedOut || result.Process == nil || result.Process.ExitCode == nil || *result.Process.ExitCode != abandonedExitCode {
		t.Fatalf("result = %#v", result)
	}
	if got := string(outputBytes(t, result.Output.Stdout)); !strings.Contains(got, "started") {
		t.Fatalf("partial stdout = %q, want the output of the commands that finished", got)
	}
	if got := string(outputBytes(t, result.Output.Stderr)); !strings.Contains(got, "abandoned") {
		t.Fatalf("stderr = %q, want the abandon notice", got)
	}
}
