// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !unix

package agentos

import (
	"os"
	"os/exec"
)

// The non-unix (windows) half of the steward supervisor's process lifecycle.
//
// There are no process groups to detach into and no SIGTERM to ask politely
// with, so the supervisor is left in its parent's group and a stop is a KILL.
// That is worth stating plainly rather than hiding: on Windows a
// `bashy steward stop` cannot run the graceful wrap-up through a signal, so the
// stop path signals the supervisor through its session record instead and only
// falls back to this. See stopStewardSession.

func stewardDetach(*exec.Cmd) {}

// stewardTermSignals is empty here: Windows has no SIGTERM to ask politely
// with, so a stop reaches the supervisor through its session record and only
// falls back to a kill. Returning os.Kill would be wrong — it cannot be caught,
// so the wrap-up would never run.
func stewardTermSignals() []os.Signal { return nil }

func stewardProcAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	// Unlike unix, FindProcess actually looks: an error means no such process.
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// A successful open is the only evidence available; Release it so the probe
	// leaves no handle behind.
	_ = proc.Release()
	return true
}

func stewardSignalStop(pid int) error { return stewardForceStop(pid) }

func stewardForceStop(pid int) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	return proc.Kill()
}
