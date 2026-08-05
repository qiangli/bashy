// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// stewardTermSignals is what `steward stop` uses to ask for a graceful wrap-up.
// SIGTERM is the ask; the supervisor turns it into "record a note, close the
// room, release the seat" rather than dying where it stands.
func stewardTermSignals() []os.Signal { return []os.Signal{syscall.SIGTERM} }

// The unix half of the steward supervisor's process lifecycle. It mirrors
// pkg/meet's service_unix.go rather than importing it — those helpers are
// unexported there, and one shared copy of four one-line syscalls is not worth a
// new exported surface on a package that has nothing else to do with stewards.

// stewardDetach puts the supervisor in its own process group, so closing the
// terminal that ran `steward start` does not take the steward down with it, and
// so a stop can reach the agent process it spawned.
func stewardDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// stewardProcAlive probes with signal 0. EPERM still means the process exists —
// reading it as "gone" would let `start` conclude the seat is free while a
// steward runs under another account.
func stewardProcAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func stewardSignalStop(pid int) error { return stewardKill(pid, syscall.SIGTERM) }
func stewardForceStop(pid int) error  { return stewardKill(pid, syscall.SIGKILL) }
func stewardLeadsGroup(pid int) bool  { g, err := syscall.Getpgid(pid); return err == nil && g == pid }

func stewardKill(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return nil
	}
	// The GROUP first — the supervisor was started with Setpgid, so it leads
	// one, and the agent CLI it spawned is inside it. But only once leadership
	// is CONFIRMED: kill(-pid) against a recycled pid that merely happens to sit
	// in somebody else's group would signal that whole unrelated group.
	if stewardLeadsGroup(pid) {
		_ = syscall.Kill(-pid, sig)
	}
	return syscall.Kill(pid, sig)
}
