// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"os/exec"
	"syscall"
)

// repairContain keeps a suite's whole process tree reachable: the suite leads
// its own process group, so a watchdog kill and the post-run sweep reach
// grandchildren the shell spawned, not just the shell.
type repairContain struct{ pgid int }

func (c *repairContain) prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}

func (c *repairContain) attach(cmd *exec.Cmd) error {
	c.pgid = cmd.Process.Pid
	return nil
}

func (c *repairContain) finish() {
	if c.pgid > 0 {
		_ = syscall.Kill(-c.pgid, syscall.SIGKILL)
	}
}
