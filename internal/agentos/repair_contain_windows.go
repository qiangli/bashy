// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build windows

package agentos

import (
	"fmt"
	"os/exec"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// repairContain puts the suite in a Job Object so a watchdog kill terminates
// the whole tree and finish waits until no member is left. Without it the
// kill reached only the shell, and an orphaned grandchild kept the suite's
// directory open ("being used by another process").
type repairContain struct {
	job windows.Handle
}

func (c *repairContain) prepare(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if c.job != 0 {
			return windows.TerminateJobObject(c.job, 1)
		}
		return cmd.Process.Kill()
	}
}

func (c *repairContain) attach(cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("CreateJobObject: %w", err)
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("SetInformationJobObject: %w", err)
	}
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(cmd.Process.Pid))
	if err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("OpenProcess(%d): %w", cmd.Process.Pid, err)
	}
	defer windows.CloseHandle(proc)
	if err := windows.AssignProcessToJobObject(job, proc); err != nil {
		_ = windows.CloseHandle(job)
		return fmt.Errorf("AssignProcessToJobObject(%d): %w", cmd.Process.Pid, err)
	}
	c.job = job
	return nil
}

func (c *repairContain) finish() {
	if c.job == 0 {
		return
	}
	_ = windows.TerminateJobObject(c.job, 1)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var acct jobBasicAccounting
		if err := windows.QueryInformationJobObject(c.job, windows.JobObjectBasicAccountingInformation,
			uintptr(unsafe.Pointer(&acct)), uint32(unsafe.Sizeof(acct)), nil); err != nil || acct.ActiveProcesses == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	_ = windows.CloseHandle(c.job)
	c.job = 0
}

// jobBasicAccounting mirrors JOBOBJECT_BASIC_ACCOUNTING_INFORMATION, which
// x/sys/windows does not export.
type jobBasicAccounting struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}
