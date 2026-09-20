// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"os"
	"syscall"
)

// ownProcessGroupAttr makes the dag root the leader of a new process group
// (pgid == its pid), so one kill(-pgid) reaches every descendant that did not
// deliberately leave the group (a setsid'd daemon is, by design, not ours).
func ownProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// signalProcessGroup forwards sig to the whole group the child leads. An
// empty group (ESRCH) is not an error: there is nobody left to tell.
func signalProcessGroup(p *os.Process, sig os.Signal) error {
	s, ok := sig.(syscall.Signal)
	if !ok {
		s = syscall.SIGTERM
	}
	if err := syscall.Kill(-p.Pid, s); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}

func killProcessGroup(p *os.Process) error {
	return signalProcessGroup(p, syscall.SIGKILL)
}
