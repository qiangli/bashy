package main

import (
	"os/exec"
	"syscall"
	"testing"
	"time"
)

// TestReapCollectsExitedChild: a child that exits is reaped by reap(), so no
// zombie is left behind.
func TestReapCollectsExitedChild(t *testing.T) {
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Skip("no `true` binary:", err)
	}
	pid := cmd.Process.Pid
	deadline := time.Now().Add(5 * time.Second)
	for {
		reap()
		// After reaping, the pid no longer exists for this parent.
		if _, err := syscall.Wait4(pid, nil, syscall.WNOHANG, nil); err == syscall.ECHILD {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("child %d was not reaped", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
