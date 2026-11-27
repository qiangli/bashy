// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package discover

import (
	"os/exec"
	"syscall"
)

// fixtureOwnGroup starts cmd as the leader of a new process group, so the
// server it may launch as a child shares one group with it.
func fixtureOwnGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// fixtureKillGroup kills every process in cmd's group: bashy and any server
// it started.
func fixtureKillGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
