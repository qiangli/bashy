package agentos

import (
	"testing"

	"github.com/qiangli/coreutils/pkg/bus"
)

// THE HOP THAT KEEPS GETTING MISSED.
//
// Every hook here defaults to nil and every nil default is SILENT: the board
// keeps working, it just answers a fleet question wrongly — or, for
// DetectHarness, signs an agent's post with the operator's login name. There is
// no crash to notice and no log line to grep, which is precisely why the same
// mistake has been made five times in this tree.
//
// pkg/bus tests prove each mechanism works when wired. This test proves it IS
// wired, which is a different claim and the one that was false.
func TestWireMessageBoard_ConnectsEveryFleetSeam(t *testing.T) {
	for _, h := range []struct {
		name string
		set  func()
		got  func() bool
	}{
		{"FleetNames", func() { bus.FleetNames = nil }, func() bool { return bus.FleetNames != nil }},
		{"FleetSelect", func() { bus.FleetSelect = nil }, func() bool { return bus.FleetSelect != nil }},
		{"FleetResolveName", func() { bus.FleetResolveName = nil }, func() bool { return bus.FleetResolveName != nil }},
		// The identity seam. Unwired, `bashy mb` misattributes every post made
		// from a third-party TUI to whoever owns the login session.
		{"DetectHarness", func() { bus.DetectHarness = nil }, func() bool { return bus.DetectHarness != nil }},
	} {
		t.Run(h.name, func(t *testing.T) {
			h.set()
			wireMessageBoard()
			if !h.got() {
				t.Fatalf("bus.%s is nil after wireMessageBoard — the seam exists but nothing connects it", h.name)
			}
		})
	}
}

// DetectHarness must be the catalog's own detector, not a private copy. The
// marker table is registry DATA (`bashy tools add` extends it), so a second
// implementation would drift the moment a harness is added — and the drift
// would show up as an agent silently posting under the operator's name again.
func TestWireMessageBoard_HarnessDetectionAnswersForThisProcess(t *testing.T) {
	wireMessageBoard()
	// Under the agent harness running these tests this reports true; on a bare
	// CI runner it reports false. Either is correct — what must not happen is a
	// panic or a hook that cannot answer at all.
	tool, ok := bus.DetectHarness()
	if ok && tool == "" {
		t.Fatal("DetectHarness reported an agent with no tool name — an identity that resolves to nothing is the bug, not the fix")
	}
}
