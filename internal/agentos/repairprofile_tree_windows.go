// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// startRepairTree starts a suite inside a kill-on-close Job Object, so the
// watchdog kill and the final reap reach every descendant. Windows has no
// process groups: killing only the bashy shell leaves its children running,
// holding the suite's working directory and pipes open (a TempDir that cannot
// be removed, a Wait that blocks for WaitDelay). The shell is assigned right
// after Start, before it has spawned anything in practice; a job that cannot
// be created or joined degrades to killing the direct child, as before.
func startRepairTree(cmd *exec.Cmd) (reap func(), err error) {
	job, jobErr := newRepairKillOnCloseJob()
	cmd.Cancel = func() error {
		if job != 0 {
			if err := windows.TerminateJobObject(job, 1); err == nil {
				return nil
			}
		}
		return cmd.Process.Kill()
	}
	if err := cmd.Start(); err != nil {
		if job != 0 {
			_ = windows.CloseHandle(job)
		}
		return func() {}, err
	}
	if jobErr == nil {
		if err := assignRepairPIDToJob(job, cmd.Process.Pid); err != nil {
			_ = windows.CloseHandle(job)
			job = 0
		}
	}
	return func() {
		if job == 0 {
			return
		}
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
		job = 0
	}, nil
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
