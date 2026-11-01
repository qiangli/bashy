// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package agentos

import "os/exec"

// startRepairTree starts a suite as the leader of its own process group, so
// the watchdog kill and the final reap reach every descendant (the bashy
// shell's go/make/test children), not just the shell. The returned reap kills
// whatever the group still holds once the suite is done; a setsid'd daemon
// deliberately left the group and is not ours.
func startRepairTree(cmd *exec.Cmd) (reap func() error, err error) {
	cmd.SysProcAttr = ownProcessGroupAttr()
	cmd.Cancel = func() error { return killProcessGroup(cmd.Process) }
	if err := cmd.Start(); err != nil {
		return func() error { return nil }, err
	}
	return func() error { _ = killProcessGroup(cmd.Process); return nil }, nil
}
