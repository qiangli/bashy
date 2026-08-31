//go:build darwin || linux

package agentos

import (
	"os"
	"os/exec"
	"testing"
)

// The stubbed tree proves the walk; this proves the PROBE. A relationship check
// that reads the process table wrongly would still satisfy every stub.
func TestInboxAncestryProbeReadsTheRealProcessTree(t *testing.T) {
	if !inboxAncestryKnown() {
		t.Fatal("darwin/linux reported no process-tree support")
	}
	if parent, ok := inboxParentPID(os.Getpid()); !ok || parent != os.Getppid() {
		t.Fatalf("inboxParentPID(self) = %d, %v; want %d", parent, ok, os.Getppid())
	}
	if got := inboxOwnerRelation(os.Getpid(), os.Getppid()); got != inboxOwnerLive {
		t.Fatalf("own parent relation = %v, want live", got)
	}

	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		t.Skipf("cannot start a child process: %v", err)
	}
	defer func() {
		_ = child.Process.Kill()
		_, _ = child.Process.Wait()
	}()
	pid := child.Process.Pid

	// We are its parent, so it sees us as a live owner.
	if got := inboxOwnerRelation(pid, os.Getpid()); got != inboxOwnerLive {
		t.Fatalf("child -> parent relation = %v, want live", got)
	}
	// It is alive, and it is NOT our ancestor. A liveness probe would call this
	// owner present; the relationship is what says otherwise.
	if got := inboxOwnerRelation(os.Getpid(), pid); got != inboxOwnerGone {
		t.Fatalf("live-but-unrelated pid %d relation = %v, want gone", pid, got)
	}
}
