//go:build !darwin && !linux

package agentos

// Targets without a supported process-tree probe FAIL OPEN: the watcher keeps
// running and the roster keeps trusting the card. An unproved relationship is
// reported as unknown, never as proof that the owning session is gone.
func inboxAncestrySupported() bool { return false }

func inboxParentProcessID(int) (int, bool) { return 0, false }
