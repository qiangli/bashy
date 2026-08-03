package agentos

import (
	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/fleet"
)

// wireMessageBoard connects `bashy mb` to the fleet catalog.
//
// pkg/bus is the transport and the roster is policy, so every question the
// board asks about the fleet is a hook the host fills in. All four are wired
// HERE, in one function, for a reason that is the whole point of the file:
//
// A seam nobody connects is indistinguishable from a feature nobody built —
// except that it looks finished. This codebase has produced that shape five
// times in a single day (an ExecHandler never registered, a struct field with
// no producer, a bus with no subscriptions, a routing hint on a path nothing
// reached, and DetectHarness's own predecessor). Every one of them passed its
// unit tests, because a test that calls the mechanism directly proves the
// mechanism works and says nothing about whether anything calls it.
//
// So the wiring is a named function with a test that asserts each hook is
// non-nil after it runs. That test fails if someone adds a fifth hook and
// forgets it here — which is the only failure mode this class of bug has.
func wireMessageBoard() {
	bus.FleetNames = fleetAgentNames
	bus.FleetSelect = fleetSelectAudience
	bus.FleetResolveName = fleetResolveAgentName

	// WHO you are, not merely who you can address.
	//
	// Without this, an agent in a third-party TUI — no BASHY_PRINCIPAL, and it
	// inherits the operator's environment — falls through to $USER, so its
	// posts are SIGNED with the operator's name, its cursor advances under that
	// name, and any claim it takes is recorded against it. Measured on this
	// host 2026-08-03: six of eight posts on a live board read `from: qiangli`,
	// spanning the operator and two different agents.
	//
	// fleet.DetectTool is the registry-driven marker table (`bashy tools add`
	// extends it), so the board and the rest of bashy agree on what counts as
	// an agent rather than keeping a second copy that can drift.
	bus.DetectHarness = fleet.DetectTool
}
