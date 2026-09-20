//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows containment is a JOB OBJECT, the platform's own process tree.
// Membership is inherited by every descendant at creation, and a job created
// with JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE terminates all of its processes the
// moment its last handle closes — which happens when the owning process exits
// for ANY reason, an abrupt kill included. That is the kill-on-parent-exit
// contract procguard gives the Unix harness, provided by the kernel instead of
// a helper process.
//
// Two jobs, nested (a process may sit in more than one job since Windows 8):
//
//   - the HARNESS job, created once and holding the harness itself, so every
//     fixture is born inside it and there is no child-before-watcher window;
//   - a per-FIXTURE job, assigned right after Start, carrying the memory cap
//     (JOB_OBJECT_LIMIT_JOB_MEMORY stands in for the Unix RSS poller: a commit
//     past the cap fails instead of wedging the host) and terminated when the
//     fixture is reaped, so background descendants of a finished fixture never
//     outlive the iteration.
//
// The per-fixture assignment happens after CreateProcess returns, so a child
// the fixture spawns in that window is in the harness job but not the fixture
// job. Parent-death containment is therefore complete; per-iteration teardown
// of such a child falls to killProcessTree, which runs on every reap as well.
//
// Until Sprint 216 this file failed closed ("unsupported on windows"), which
// is why the suite had never been measured there.

type parentDeathWatch struct {
	cmd *exec.Cmd
	job windows.Handle
}

func configureProcess(cmd *exec.Cmd) {}

// killProcessTree is the pid-addressed kill the shared harness code calls on
// timeout and on reap. The job object is the containment guarantee; this is
// the prompt path for the fixture's own tree.
func killProcessTree(pid int) {
	_ = exec.Command("taskkill.exe", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
}

// newKillOnCloseJob creates an anonymous job whose processes die with its last
// handle. memLimitKB > 0 additionally caps the job's committed memory.
func newKillOnCloseJob(memLimitKB int) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if memLimitKB > 0 {
		info.BasicLimitInformation.LimitFlags |= windows.JOB_OBJECT_LIMIT_JOB_MEMORY
		info.JobMemoryLimit = jobMemoryLimitBytes(memLimitKB)
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	return job, nil
}

// jobMemoryLimitBytes converts the KB cap to the job's byte limit, saturating
// rather than wrapping on a 32-bit build (4 GB in KB times 1024 is 2^32).
func jobMemoryLimitBytes(memLimitKB int) uintptr {
	limit := uint64(memLimitKB) * 1024
	if limit > uint64(^uintptr(0)) {
		return ^uintptr(0)
	}
	return uintptr(limit)
}

// harnessJob puts the harness itself into a kill-on-close job exactly once.
// The handle is deliberately never closed: it lives as long as the process,
// and its implicit close at process death is the parent-death guard.
var harnessJob = sync.OnceValue(func() error {
	job, err := newKillOnCloseJob(0)
	if err != nil {
		return err
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("AssignProcessToJobObject(self): %w (nested jobs need Windows 8 or later)", err)
	}
	return nil
})

// armParentDeathWatch fails closed: if the harness cannot be contained, no
// fixture is launched, exactly as the Unix guard refuses an unguarded start.
func armParentDeathWatch(cmd *exec.Cmd) (*parentDeathWatch, error) {
	if err := harnessJob(); err != nil {
		return nil, fmt.Errorf("parent-death guard: %w", err)
	}
	job, err := newKillOnCloseJob(memCapKB)
	if err != nil {
		return nil, fmt.Errorf("parent-death guard: %w", err)
	}
	return &parentDeathWatch{cmd: cmd, job: job}, nil
}

var fixtureJobWarnOnce sync.Once

// parentDeathWatchStarted assigns the started fixture to its own job. The
// fixture already sits in the harness job by inheritance, so the assignment
// only adds the per-fixture teardown and memory cap; a failure here is not a
// containment hole and is not fatal — but it is said once, because a cap that
// silently stopped binding is the absence-of-evidence shape this repo hunts.
func parentDeathWatchStarted(watch *parentDeathWatch, startErr error) {
	if watch == nil {
		return
	}
	if startErr != nil || watch.cmd == nil || watch.cmd.Process == nil {
		stopParentDeathWatch(watch)
		return
	}
	if err := assignPIDToJob(watch.job, watch.cmd.Process.Pid); err != nil {
		fixtureJobWarnOnce.Do(func() {
			fmt.Fprintf(os.Stderr, "bash53-suite: per-fixture job assignment failed: %v (memory cap not enforced; parent-death containment still holds through the harness job)\n", err)
		})
	}
}

func assignPIDToJob(job windows.Handle, pid int) error {
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("OpenProcess(%d): %w", pid, err)
	}
	defer windows.CloseHandle(proc)
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		return fmt.Errorf("AssignProcessToJobObject(%d): %w", pid, err)
	}
	return nil
}

// stopParentDeathWatch terminates whatever is left in the fixture's job and
// releases it. Called on reap, on timeout and on a failed start.
func stopParentDeathWatch(watch *parentDeathWatch) {
	if watch == nil || watch.job == 0 {
		return
	}
	_ = windows.TerminateJobObject(watch.job, 1)
	_ = windows.CloseHandle(watch.job)
	watch.job = 0
}

// processInJob reports whether pid is in ANY job (the kernel's own answer,
// via IsProcessInJob with a NULL job handle). The tests use it to prove the
// harness contained itself rather than trusting that AssignProcessToJobObject
// returned nil.
func processInJob(pid int) (bool, error) {
	proc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(proc)
	var result int32
	r1, _, e1 := procIsProcessInJob.Call(uintptr(proc), 0, uintptr(unsafe.Pointer(&result)))
	if r1 == 0 {
		return false, fmt.Errorf("IsProcessInJob: %v", e1)
	}
	return result != 0, nil
}

var (
	modkernel32        = windows.NewLazySystemDLL("kernel32.dll")
	procIsProcessInJob = modkernel32.NewProc("IsProcessInJob")
)

// jobLimits reads back the extended limits a job carries.
func jobLimits(job windows.Handle) (windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION, error) {
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	var n uint32
	err := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &n)
	return info, err
}
