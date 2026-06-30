// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"os"
	"syscall"
)

// procStatus returns the exit status and whether the process was terminated by
// a signal. A signal is encoded as 128+signum (shell convention), so e.g. an
// OOM SIGKILL reports 137 — which the advisor's compute case keys on.
func procStatus(ps *os.ProcessState) (int, bool) {
	if ws, ok := ps.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return 128 + int(ws.Signal()), true
	}
	return ps.ExitCode(), false
}
