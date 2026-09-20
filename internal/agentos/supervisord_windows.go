// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"syscall"

	"golang.org/x/sys/windows"
)

// Windows has no POSIX process groups or signals. The honest equivalents:
// the child gets its own console process group (CREATE_NEW_PROCESS_GROUP),
// the graceful phase is a CTRL_BREAK event to that group (a Go child sees it
// as SIGINT; a console child must share our console for it to arrive), and
// the kill phase terminates the direct child only — descendants that outlive
// it are not reached. There is no orphan reaping (supervisord_other.go).
func ownProcessGroupAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP}
}

func signalProcessGroup(p *os.Process, _ os.Signal) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(p.Pid))
}

func killProcessGroup(p *os.Process) error {
	if err := p.Kill(); err != nil && err != os.ErrProcessDone {
		return err
	}
	return nil
}
