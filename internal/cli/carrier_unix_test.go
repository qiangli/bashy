// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// These tests drive the real bin/bash binary end to end: the OS-backed job
// carrier re-execs the shell executable in helper mode, so only a subprocess
// run proves the whole chain — helper interception, real `$!` PIDs, external
// signaling, and carrier reaping.

var (
	bashBinOnce sync.Once
	bashBinPath string
	bashBinErr  error
)

// builtBashBin builds cmd/bash once for the whole package and returns its path.
func builtBashBin(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("carrier subprocess tests build cmd/bash; skipped with -short")
	}
	bashBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "bashy-carrier-bin-")
		if err != nil {
			bashBinErr = err
			return
		}
		testCleanups = append(testCleanups, func() { os.RemoveAll(dir) })
		bashBinPath = filepath.Join(dir, "bash")
		out, err := exec.Command("go", "build", "-o", bashBinPath, "github.com/qiangli/bashy/cmd/bash").CombinedOutput()
		if err != nil {
			bashBinErr = fmt.Errorf("building cmd/bash: %v\n%s", err, out)
		}
	})
	if bashBinErr != nil {
		t.Fatal(bashBinErr)
	}
	return bashBinPath
}

// killBinPrelude resolves the external kill binary into $K, bypassing the
// builtin, so scripts exercise the "an external process signals the job" path.
const killBinPrelude = `K=/bin/kill; [ -x "$K" ] || K=/usr/bin/kill` + "\n"

// runBuiltBash runs `bin/bash -c script` in dir with a minimal environment and
// returns its combined output and exit code. A wedged run is killed at 30s.
func runBuiltBash(t *testing.T, dir, script string) (string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, builtBashBin(t), "-c", script)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/bin:/usr/bin", "HOME=" + dir, "TMPDIR=" + dir}
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("script wedged (30s):\n%s", out)
	}
	exit := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("running script: %v\n%s", err, out)
		}
		exit = ee.ExitCode()
	}
	return string(out), exit
}

var numericPidRe = regexp.MustCompile(`^[1-9][0-9]*$`)

func pidLive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}

// waitPidsGone polls until none of the PIDs exist anymore, failing the test if
// any survives the deadline — the carrier-leak check.
func waitPidsGone(t *testing.T, pids []int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for _, pid := range pids {
		for pidLive(pid) {
			if time.Now().After(deadline) {
				t.Fatalf("carrier pid %d still alive; leaked?", pid)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// outPid extracts the `pid=N` line and asserts it is a real numeric PID.
func outPid(t *testing.T, out string) int {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(line, "pid="); ok {
			if !numericPidRe.MatchString(p) {
				t.Fatalf("$! = %q, want a real numeric PID; output:\n%s", p, out)
			}
			var pid int
			fmt.Sscanf(p, "%d", &pid)
			return pid
		}
	}
	t.Fatalf("no pid= line in output:\n%s", out)
	return 0
}

// TestCarrierCertificationShape runs the exact shape that invalidated the VSC
// certification run (tetapi.sh stores $! in tet_context and does integer
// comparisons on it): a bounded pure-builtin job whose $! must be numeric,
// probeable by the external /bin/kill, and resolvable by wait.
func TestCarrierCertificationShape(t *testing.T) {
	out, exit := runBuiltBash(t, t.TempDir(), killBinPrelude+
		`(:)& p=$!; case $p in *[!0-9]*|"") exit 90;; esac; "$K" -0 "$p"; wait "$p"`)
	if exit == 90 {
		t.Fatalf("$! was not numeric (synthetic g<N>?); output:\n%s", out)
	}
	if exit != 0 {
		t.Fatalf("exit = %d, want 0; output:\n%s", exit, out)
	}
}

// TestCarrierLivePidProbeAndCleanup makes the liveness probe deterministic: a
// file-gated pure-builtin job is held alive until the external kill -0 has
// seen its carrier, then released; afterwards the carrier must be gone.
func TestCarrierLivePidProbeAndCleanup(t *testing.T) {
	out, exit := runBuiltBash(t, t.TempDir(), killBinPrelude+`
{ : >ready; until [ -e go ]; do :; done; } &
p=$!
echo "pid=$p"
while [ ! -e ready ]; do :; done
"$K" -0 "$p" && echo alive
: >go
wait "$p"
echo "st=$?"
`)
	if exit != 0 || !strings.Contains(out, "alive") || !strings.Contains(out, "st=0") {
		t.Fatalf("exit=%d, unexpected output:\n%s", exit, out)
	}
	waitPidsGone(t, []int{outPid(t, out)})
}

// TestCarrierExternalTermAndKill pins the 128+signal mapping for an external
// signal aimed at a pure-builtin async compound: TERM -> 143, and the
// uncatchable KILL -> 137. The carrier must be reaped afterwards.
func TestCarrierExternalTermAndKill(t *testing.T) {
	for _, tc := range []struct{ sig, want string }{
		{"TERM", "143"},
		{"KILL", "137"},
	} {
		t.Run(tc.sig, func(t *testing.T) {
			out, exit := runBuiltBash(t, t.TempDir(), killBinPrelude+`
{ while :; do :; done; } &
p=$!
echo "pid=$p"
"$K" -`+tc.sig+` "$p"
wait "$p"
echo "st=$?"
`)
			if exit != 0 || !strings.Contains(out, "st="+tc.want) {
				t.Fatalf("exit=%d, want st=%s in output:\n%s", exit, tc.want, out)
			}
			waitPidsGone(t, []int{outPid(t, out)})
		})
	}
}

// TestCarrierExitsOnNaturalCompletion checks that a job finishing on its own
// reaps its carrier: after wait resolves, the carrier PID no longer exists —
// across several back-to-back jobs, each with a distinct real PID.
func TestCarrierExitsOnNaturalCompletion(t *testing.T) {
	out, exit := runBuiltBash(t, t.TempDir(), `
for i in 1 2 3 4; do
	{ :; } &
	echo "pid=$!"
	wait "$!" || exit 91
done
`)
	if exit != 0 {
		t.Fatalf("exit=%d, output:\n%s", exit, out)
	}
	var pids []int
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		p, ok := strings.CutPrefix(line, "pid=")
		if !ok {
			continue
		}
		if !numericPidRe.MatchString(p) {
			t.Fatalf("$! = %q, want a real numeric PID; output:\n%s", p, out)
		}
		if seen[p] {
			t.Fatalf("duplicate carrier pid %s; output:\n%s", p, out)
		}
		seen[p] = true
		var pid int
		fmt.Sscanf(p, "%d", &pid)
		pids = append(pids, pid)
	}
	if len(pids) != 4 {
		t.Fatalf("want 4 pids, got %d; output:\n%s", len(pids), out)
	}
	waitPidsGone(t, pids)
}

// TestCarrierHelperLifecycle drives helper mode directly: it must produce no
// output, survive while the parent holds its stdin pipe, exit 0 on EOF, and —
// dispositions preserved — die of an external SIGTERM with a signal status.
func TestCarrierHelperLifecycle(t *testing.T) {
	t.Run("EOF", func(t *testing.T) {
		cmd := exec.Command(builtBashBin(t), "--bashy-job-carrier")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		var out strings.Builder
		cmd.Stdout, cmd.Stderr = &out, &out
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Millisecond)
		if !pidLive(cmd.Process.Pid) {
			t.Fatal("helper exited while its stdin pipe was still open")
		}
		stdin.Close()
		if err := cmd.Wait(); err != nil {
			t.Fatalf("helper exit on EOF: %v; output: %s", err, out.String())
		}
		if out.String() != "" {
			t.Fatalf("helper produced output: %q", out.String())
		}
	})
	t.Run("Signal", func(t *testing.T) {
		cmd := exec.Command(builtBashBin(t), "--bashy-job-carrier")
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		defer stdin.Close()
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
		if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
			t.Fatal(err)
		}
		err = cmd.Wait()
		ws, ok := cmd.ProcessState.Sys().(syscall.WaitStatus)
		if !ok || !ws.Signaled() || ws.Signal() != syscall.SIGTERM {
			t.Fatalf("helper death: err=%v state=%v, want SIGTERM signal death", err, cmd.ProcessState)
		}
	})
}
