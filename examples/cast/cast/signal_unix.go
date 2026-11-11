// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !windows

package cast

import (
	"os/exec"
	"syscall"
)

func signalNumber(ee *exec.ExitError) int {
	if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
		return int(ws.Signal())
	}
	return 0
}
