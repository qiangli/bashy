//go:build windows

package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// longRunner is a child that only a kill can end within the test budget:
// ping is on every Windows and blocks between echoes.
func longRunner() *exec.Cmd {
	return exec.Command("ping.exe", "-n", "60", "127.0.0.1")
}

// pidAlive asks the kernel, not the parent: an exited process reports an exit
// code other than STILL_ACTIVE, and a reaped one cannot be opened at all.
func pidAlive(pid int) bool {
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(proc)
	var code uint32
	if err := windows.GetExitCodeProcess(proc, &code); err != nil {
		return false
	}
	return code == 259 // STILL_ACTIVE
}

func waitPIDGone(pid int, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !pidAlive(pid) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return !pidAlive(pid)
}

// TestFixtureGuardContainsWindowsChild is the per-fixture half of the
// contract: a fixture launched through the guard is inside a job object, and
// releasing the guard terminates it.
func TestFixtureGuardContainsWindowsChild(t *testing.T) {
	cmd := longRunner()
	watch, err := armParentDeathWatch(cmd)
	if err != nil {
		t.Fatalf("armParentDeathWatch: %v", err)
	}
	if watch == nil || watch.job == 0 {
		t.Fatal("armed guard has no job object")
	}
	startErr := cmd.Start()
	parentDeathWatchStarted(watch, startErr)
	if startErr != nil {
		t.Fatalf("start: %v", startErr)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		t.Fatalf("child exited before the guard was released: %v", err)
	case <-time.After(500 * time.Millisecond):
	}
	inJob, err := processInJob(cmd.Process.Pid)
	if err != nil || !inJob {
		t.Fatalf("started fixture is not in a job (inJob=%v err=%v)", inJob, err)
	}
	stopParentDeathWatch(watch)
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("releasing the guard did not terminate the contained child")
	}
	if watch.job != 0 {
		t.Fatal("released guard still holds its job handle")
	}
}

// TestFixtureGuardHarnessContainsItself: arming the first guard puts the
// harness process into a job. Every later fixture inherits that membership
// at creation, which is what closes the child-before-watcher window.
func TestFixtureGuardHarnessContainsItself(t *testing.T) {
	watch, err := armParentDeathWatch(exec.Command("cmd.exe", "/c", "exit", "0"))
	if err != nil {
		t.Fatalf("armParentDeathWatch: %v", err)
	}
	defer stopParentDeathWatch(watch)
	inJob, err := processInJob(os.Getpid())
	if err != nil {
		t.Fatalf("IsProcessInJob(self): %v", err)
	}
	if !inJob {
		t.Fatal("harness process is not in a job after arming the guard")
	}
}

// TestFixtureGuardJobCarriesKillOnCloseAndMemoryCap reads the limits back from
// the kernel: the per-fixture job must kill on close and carry memCapKB.
func TestFixtureGuardJobCarriesKillOnCloseAndMemoryCap(t *testing.T) {
	old := memCapKB
	memCapKB = 512 * 1024
	t.Cleanup(func() { memCapKB = old })
	watch, err := armParentDeathWatch(exec.Command("cmd.exe", "/c", "exit", "0"))
	if err != nil {
		t.Fatalf("armParentDeathWatch: %v", err)
	}
	defer stopParentDeathWatch(watch)
	info, err := jobLimits(watch.job)
	if err != nil {
		t.Fatalf("QueryInformationJobObject: %v", err)
	}
	flags := info.BasicLimitInformation.LimitFlags
	if flags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		t.Fatalf("job lacks KILL_ON_JOB_CLOSE (flags=%#x)", flags)
	}
	if flags&windows.JOB_OBJECT_LIMIT_JOB_MEMORY == 0 {
		t.Fatalf("job lacks JOB_MEMORY (flags=%#x)", flags)
	}
	if want := uintptr(512 * 1024 * 1024); info.JobMemoryLimit != want {
		t.Fatalf("JobMemoryLimit = %d, want %d", info.JobMemoryLimit, want)
	}
}

// TestFixtureGuardNoMemoryCapWhenDisabled: BASH53_MEM_KB=0 means no cap, and
// the job must not carry a zero-byte memory limit (which would refuse every
// allocation).
func TestFixtureGuardNoMemoryCapWhenDisabled(t *testing.T) {
	old := memCapKB
	memCapKB = 0
	t.Cleanup(func() { memCapKB = old })
	watch, err := armParentDeathWatch(exec.Command("cmd.exe", "/c", "exit", "0"))
	if err != nil {
		t.Fatalf("armParentDeathWatch: %v", err)
	}
	defer stopParentDeathWatch(watch)
	info, err := jobLimits(watch.job)
	if err != nil {
		t.Fatalf("QueryInformationJobObject: %v", err)
	}
	if info.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_JOB_MEMORY != 0 {
		t.Fatalf("job carries a memory limit with the cap disabled (flags=%#x)", info.BasicLimitInformation.LimitFlags)
	}
}

// TestFixtureGuardFailedStartReleasesJob: a start failure must not leak the
// per-fixture job handle.
func TestFixtureGuardFailedStartReleasesJob(t *testing.T) {
	cmd := exec.Command(`C:\no\such\program.exe`)
	watch, err := armParentDeathWatch(cmd)
	if err != nil {
		t.Fatalf("armParentDeathWatch: %v", err)
	}
	startErr := cmd.Start()
	if startErr == nil {
		t.Fatal("expected the start of a missing program to fail")
	}
	parentDeathWatchStarted(watch, startErr)
	if watch.job != 0 {
		t.Fatal("failed start left the job handle open")
	}
}

// TestFixtureParentExitKillsJob is the parent-death half of the contract, the
// Windows twin of TestFixtureParentExitKillsProcessGroup: a helper process
// (this test binary re-executed) arms a guard around a long-running child and
// is then killed abruptly; the child must not survive it.
func TestFixtureParentExitKillsJob(t *testing.T) {
	if os.Getenv("BASH53_PARENT_EXIT_HELPER") == "1" {
		cmd := longRunner()
		watch, err := armParentDeathWatch(cmd)
		if err != nil {
			fmt.Println("ERR", err)
			return
		}
		startErr := cmd.Start()
		parentDeathWatchStarted(watch, startErr)
		if startErr != nil {
			fmt.Println("ERR", startErr)
			return
		}
		fmt.Println("CHILD", cmd.Process.Pid)
		select {} // hold the job handles until the parent kills us
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	helper := exec.Command(exe, "-test.run=^TestFixtureParentExitKillsJob$", "-test.v")
	helper.Env = append(os.Environ(), "BASH53_PARENT_EXIT_HELPER=1")
	out, err := helper.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	helper.Stderr = os.Stderr
	if err := helper.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer func() { _ = helper.Process.Kill(); _ = helper.Wait() }()

	childPID := 0
	lines := bufio.NewScanner(out)
	deadline := time.After(20 * time.Second)
	got := make(chan string, 1)
	go func() {
		for lines.Scan() {
			line := strings.TrimSpace(lines.Text())
			if strings.HasPrefix(line, "CHILD ") || strings.HasPrefix(line, "ERR ") {
				got <- line
				return
			}
		}
		got <- "EOF"
	}()
	select {
	case line := <-got:
		if !strings.HasPrefix(line, "CHILD ") {
			t.Fatalf("helper did not launch a contained child: %s", line)
		}
		childPID, err = strconv.Atoi(strings.TrimPrefix(line, "CHILD "))
		if err != nil {
			t.Fatalf("helper pid line %q: %v", line, err)
		}
	case <-deadline:
		t.Fatal("helper never reported its child")
	}
	if !pidAlive(childPID) {
		t.Fatalf("contained child %d is not running before the helper dies", childPID)
	}
	if err := helper.Process.Kill(); err != nil {
		t.Fatalf("kill helper: %v", err)
	}
	_ = helper.Wait()
	if !waitPIDGone(childPID, 10*time.Second) {
		_ = exec.Command("taskkill.exe", "/F", "/PID", strconv.Itoa(childPID)).Run()
		t.Fatalf("child %d survived its guarding parent's abrupt exit", childPID)
	}
}
