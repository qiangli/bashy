// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"os/exec"
	"syscall"
)

// mcpHostedContain gives a hosted call its own process group, so its deadline
// reaches every process the command started and not only the first.
func mcpHostedContain(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
