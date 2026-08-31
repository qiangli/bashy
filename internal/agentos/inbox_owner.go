package agentos

import (
	"fmt"

	"github.com/qiangli/coreutils/pkg/room"
)

// A registered inbox watcher spends one fleet identity's cursors. Its right to
// do that comes entirely from the agent session that started it: what it drains
// is rendered into that session's turn stream and nowhere else. When the owning
// process exits, the watcher is reparented and keeps draining — every record it
// acknowledges from that moment on is delivered to nobody, while the identity
// stays claimed against the live agent that should have replaced it.
//
// Liveness alone cannot answer this. A pid is reused, so "the owner pid is
// alive" silently becomes true again under an unrelated process. The real
// question is a RELATIONSHIP: is the recorded owner still this watcher's parent
// or one of its ancestors.
type inboxOwnerState int

const (
	// inboxOwnerUnknown means this platform cannot read the process tree. It is
	// deliberately not a refusal: failing closed here would stop every watcher
	// on an unsupported target, which is a worse failure than the leak it
	// prevents.
	inboxOwnerUnknown inboxOwnerState = iota
	inboxOwnerLive
	inboxOwnerGone
)

// inboxAncestryDepth bounds the walk so a corrupt or cyclic process table can
// never spin. A chain longer than this is unproved, which is not live.
const inboxAncestryDepth = 128

// The platform probes, indirected so a test can build a deterministic process
// tree instead of racing real pids.
var (
	inboxParentPID     = inboxParentProcessID
	inboxAncestryKnown = inboxAncestrySupported
)

// inboxOwnerRelation reports whether anchor is pid itself or one of its
// ancestors. A pid whose parent cannot be read is treated as gone: the walk
// proves the relationship or it proves nothing.
func inboxOwnerRelation(pid, anchor int) inboxOwnerState {
	if !inboxAncestryKnown() {
		return inboxOwnerUnknown
	}
	if pid <= 0 || anchor <= 0 {
		return inboxOwnerGone
	}
	seen := make(map[int]bool, 8)
	for depth := 0; pid > 0 && depth < inboxAncestryDepth; depth++ {
		if pid == anchor {
			return inboxOwnerLive
		}
		if seen[pid] {
			return inboxOwnerGone
		}
		seen[pid] = true
		parent, ok := inboxParentPID(pid)
		if !ok || parent == pid {
			return inboxOwnerGone
		}
		pid = parent
	}
	return inboxOwnerGone
}

// inboxWatcherAnchor is the process whose continued relationship to the watcher
// justifies the card. OwnerPID is the owning agent harness; a watcher that
// could not prove one (its parent was already PID 1) anchors on itself, which
// keeps the check honest rather than inventing a session it never had.
func inboxWatcherAnchor(card room.Card) int {
	if card.OwnerPID > 0 {
		return card.OwnerPID
	}
	return card.PID
}

// inboxWatcherOrphaned reports whether a published watcher card has outlived
// the agent session it was registered for. Unknown ancestry is not orphaned.
func inboxWatcherOrphaned(card room.Card) bool {
	return inboxOwnerRelation(card.PID, inboxWatcherAnchor(card)) == inboxOwnerGone
}

// inboxOwnerGoneError is the watch loop's exit. It states that monitoring
// ENDED, why, that nothing was consumed, and who must resume coverage — the
// same contract `bashy inbox --help` requires of a sentinel that stops.
type inboxOwnerGoneError struct {
	agent string
	owner int
}

func (e *inboxOwnerGoneError) Error() string {
	return fmt.Sprintf("inbox: monitoring ENDED for %s: the owning agent session (pid %d) is no longer this watcher's parent or ancestor; "+
		"no record was rendered or acknowledged and no source cursor advanced, so %s's backlog is intact and still OUTSTANDING; "+
		"a live session must re-enter `bashy inbox --as %s --watch` to resume coverage",
		e.agent, e.owner, e.agent, e.agent)
}
