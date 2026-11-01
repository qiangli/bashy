// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// startRepairTree creates the suite suspended, assigns it to a kill-on-close
// Job Object, then resumes its initial thread. No suite code can run (and spawn
// an escaping descendant) before job assignment. Failure to establish this
// containment aborts the launch instead of silently reverting to a direct kill.
func startRepairTree(cmd *exec.Cmd) (reap func() error, err error) {
	job, err := newRepairKillOnCloseJob()
	if err != nil {
		return func() error { return nil }, err
	}
	// CommandContext may call Cancel as soon as Start returns. Serialize it
	// with assignment/resume and handle closure, including the failed-start path.
	var mu sync.Mutex
	mu.Lock()
	cmd.Cancel = func() error {
		mu.Lock()
		defer mu.Unlock()
		if job == 0 {
			return os.ErrProcessDone
		}
		return windows.TerminateJobObject(job, 1)
	}
	closeJob := func() error {
		mu.Lock()
		defer mu.Unlock()
		if job != 0 {
			defer func() {
				_ = windows.CloseHandle(job)
				job = 0
			}()
			if err := windows.TerminateJobObject(job, 1); err != nil {
				return fmt.Errorf("terminate suite job: %w", err)
			}
			return waitRepairJobEmpty(job)
		}
		return nil
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	if err := cmd.Start(); err != nil {
		mu.Unlock()
		closeJob()
		return func() error { return nil }, err
	}
	if err = assignRepairPIDToJob(job, cmd.Process.Pid); err == nil {
		err = resumeRepairInitialThread(cmd.Process.Pid)
	}
	if err != nil {
		// The child is still suspended, so there are no descendants. Kill it
		// directly even if job assignment failed, and Wait to drain exec's
		// goroutines. Unlock first: Wait may wait for the cancellation callback.
		_ = cmd.Process.Kill()
		mu.Unlock()
		closeJob()
		_ = cmd.Wait()
		return func() error { return nil }, fmt.Errorf("repair suite containment: %w", err)
	}
	mu.Unlock()
	return closeJob, nil
}

// os/exec closes CreateProcess's thread handle. The suspended child has only
// its initial thread, which Toolhelp lets us reopen using documented APIs.
func resumeRepairInitialThread(pid int) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("snapshot initial thread: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}
		thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			return fmt.Errorf("open initial thread: %w", err)
		}
		defer windows.CloseHandle(thread)
		if _, err := windows.ResumeThread(thread); err != nil {
			return fmt.Errorf("resume initial thread: %w", err)
		}
		return nil
	}
	return fmt.Errorf("find initial thread for suspended suite: %w", err)
}

// newRepairKillOnCloseJob creates an anonymous job whose processes die with
// its last handle (mirrors tools/bash53suite's fixture containment).
func newRepairKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("CreateJobObject: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("SetInformationJobObject: %w", err)
	}
	return job, nil
}

func assignRepairPIDToJob(job windows.Handle, pid int) error {
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return os.NewSyscallError("OpenProcess", err)
	}
	defer windows.CloseHandle(proc)
	return windows.AssignProcessToJobObject(job, proc)
}

// TerminateJobObject is asynchronous. Wait for every member to finish so file
// and directory handles cannot outlive cleanup. A failed/slow drain is a failed
// suite, never a successful result with lingering children.
func waitRepairJobEmpty(job windows.Handle) error {
	// JOBOBJECT_BASIC_ACCOUNTING_INFORMATION (Windows SDK layout).
	var accounting struct {
		TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
		TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return fmt.Errorf("query suite job: %w", err)
		}
		if accounting.ActiveProcesses == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("suite job still has %d active processes after termination", accounting.ActiveProcesses)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
