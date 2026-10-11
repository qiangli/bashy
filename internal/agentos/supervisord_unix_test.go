// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"os/signal"
	"syscall"
)

func signalIgnoreTerm() { signal.Ignore(syscall.SIGTERM, syscall.SIGINT) }

// processGone: no process (not even a zombie we failed to reap) has pid.
func processGone(pid int) bool { return syscall.Kill(pid, 0) == syscall.ESRCH }
