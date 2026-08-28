//go:build unix

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestFixtureParentExitKillsProcessGroup exercises the watcher through
// runFixture, not as an isolated helper. If the harness itself is SIGKILLed,
// the fixture and its ordinary background children must not remain on the
// certification host.
func TestFixtureParentExitKillsProcessGroup(t *testing.T) {
	if os.Getenv("BASH53_PARENT_EXIT_HELPER") == "1" {
		testsDir := os.Getenv("BASH53_PARENT_EXIT_DIR")
		sh, err := exec.LookPath("sh")
		if err != nil {
			return
		}
		_, _ = runFixture(testsDir, testsDir, sh, fixture{
			Name:  "parent-exit",
			Test:  "parent-exit.sh",
			Right: "parent-exit.right",
		}, 2*time.Minute)
		return
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	_ = sh

	dir := t.TempDir()
	script := "echo $$ > leader.pid\nsleep 120 &\necho $! > child.pid\nwait\n"
	if err := os.WriteFile(filepath.Join(dir, "parent-exit.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "parent-exit.right"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	helper := exec.Command(os.Args[0], "-test.run=^TestFixtureParentExitKillsProcessGroup$", "-test.v=false")
	helper.Env = append(os.Environ(),
		"BASH53_PARENT_EXIT_HELPER=1",
		"BASH53_PARENT_EXIT_DIR="+dir,
	)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	leader := readFixturePID(t, filepath.Join(dir, "leader.pid"), 5*time.Second)
	child := readFixturePID(t, filepath.Join(dir, "child.pid"), 5*time.Second)
	defer func() {
		_ = syscall.Kill(leader, syscall.SIGKILL)
		_ = syscall.Kill(child, syscall.SIGKILL)
	}()

	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := helper.Wait(); err == nil {
		t.Fatal("helper should be killed")
	}
	if !waitFixtureGone(leader, 5*time.Second) {
		t.Errorf("fixture leader %d survived harness death", leader)
	}
	if !waitFixtureGone(child, 5*time.Second) {
		t.Errorf("fixture child %d survived harness death", child)
	}
}

func TestGuardedFixtureNormalRun(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no sh on this host")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "normal.sh"), []byte("echo guarded-normal\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "normal.right"), []byte("guarded-normal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	status, err := runFixture(dir, dir, sh, fixture{Name: "normal", Test: "normal.sh", Right: "normal.right"}, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if status != "PASS" {
		t.Fatalf("status = %s", status)
	}
}

func TestFixtureGuardExistsBeforeCommandStart(t *testing.T) {
	if os.Getenv("BASH53_GUARD_EARLY_HELPER") == "1" {
		dir := os.Getenv("BASH53_GUARD_EARLY_DIR")
		cmd := exec.Command("sh", "-c", "sleep 0.05; echo $$ > child.pid; sleep 120")
		cmd.Dir = dir
		configureProcess(cmd)
		guard, err := armParentDeathWatch(cmd)
		if err != nil {
			return
		}
		startErr := cmd.Start()
		parentDeathWatchStarted(guard, startErr)
		if startErr != nil {
			return
		}
		_ = os.WriteFile(filepath.Join(dir, "ready"), []byte("ready"), 0o600)
		_ = cmd.Wait()
		stopParentDeathWatch(guard)
		return
	}

	dir := t.TempDir()
	helper := exec.Command(os.Args[0], "-test.run=^TestFixtureGuardExistsBeforeCommandStart$", "-test.v=false")
	helper.Env = append(os.Environ(), "BASH53_GUARD_EARLY_HELPER=1", "BASH53_GUARD_EARLY_DIR="+dir)
	if err := helper.Start(); err != nil {
		t.Fatal(err)
	}
	waitFixtureFile(t, filepath.Join(dir, "ready"), 5*time.Second)
	if err := helper.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = helper.Wait()
	time.Sleep(150 * time.Millisecond)
	raw, err := os.ReadFile(filepath.Join(dir, "child.pid"))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = syscall.Kill(pid, syscall.SIGKILL) }()
	if !waitFixtureGone(pid, 5*time.Second) {
		t.Fatalf("fixture %d survived harness death at the Start boundary", pid)
	}
}

func waitFixtureFile(t *testing.T, path string, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("file did not appear: %s", path)
}

func readFixturePID(t *testing.T, path string, within time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && pid > 1 {
				return pid
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fixture did not publish pid at %s", path)
	return 0
}

func waitFixtureGone(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if syscall.Kill(pid, 0) == syscall.ESRCH {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return syscall.Kill(pid, 0) == syscall.ESRCH
}
