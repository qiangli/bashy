package main

import (
	"context"
	"os"
	"strings"
	"sync"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/meet"
	"github.com/qiangli/yoke/pkg/weave"
)

// wireMessageBoard connects `bashy mb` to the fleet catalog.
//
// pkg/bus is the transport and the roster is policy, so every question the
// board asks about the fleet is a hook the host fills in. They are wired
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
	bus.CurrentSessionClaim = currentAgentSession
	bus.IsMainLineTool = isSteerToolName

	// WHO you are, not merely who you can address.
	//
	// Without this, an agent in a third-party TUI — no BASHY_PRINCIPAL, and it
	// inherits the operator's environment — falls through to $USER, so its
	// posts are SIGNED with the operator's name, its cursor advances under that
	// name, and any claim it takes is recorded against it. Measured on this
	// host 2026-08-03: six of eight posts on a live board read `from: qiangli`,
	// spanning the operator and two different agents.
	//
	// fleet.DetectTool is the registry-driven marker table (`bashy tool add`
	// extends it), so the board and the rest of bashy agree on what counts as
	// an agent rather than keeping a second copy that can drift.
	bus.DetectHarness = fleet.DetectTool
	// One receive surface, one turn-boundary hook. The callback is a read-through
	// view over MB/Meet/role stores; pkg/bus remains the owner of pending delivery
	// and no additional spool is introduced.
	bus.PrepareTurnInbox = unifiedTurnPreamble
	bus.PrepareTurnItems = unifiedTurnItems
	// Whoever adds an inbox source owns its change gate. The hook above makes
	// every managed session's relay read the board and meet stores too, so the
	// relay's poll gate must see THOSE move — coreutils' default fingerprint
	// stats only the bus stores. The same fsnotify generation counter that
	// keeps `inbox --watch` off the CPU (Sprint 138) does it; one notifier per
	// process, armed lazily on first use so a session that never relays pays
	// nothing (coreutils story #127).
	bus.InboxFingerprint = sharedInboxFingerprint

	// Cross-host mail (Sprint 217): a target the local ladder cannot place —
	// or one spelled <name>@<host> — is asked of the repo's shared session
	// roster, and the message rides that session as the relay. The repo the
	// caller stands in is the session key; nothing is configured. bus keeps
	// no cloudbox import: weave answers through these three seams, the same
	// shape as FleetNames above.
	bus.RemoteResolve = func(target string) (bus.RemoteRoute, error) {
		cwd, _ := os.Getwd()
		return weave.ResolveRemoteParticipant(context.Background(), cwd, target)
	}
	bus.RemoteSend = func(m bus.RemoteMessage) (bus.RemoteReceipt, error) {
		cwd, _ := os.Getwd()
		return weave.SendRemoteMessage(context.Background(), cwd, m)
	}
	bus.RemoteSender = weave.RemoteSenderSignature

	// Shared meet rooms (Sprint 217, story 8): a `--shared` board's posts ride
	// the same session; a host that drains the feed keeps a mirror of the
	// room. meet never imports the relay — these two seams are it.
	meet.RelayShared = func(st *meet.State, ev meet.Event) error {
		cwd, _ := os.Getwd()
		return weave.RelaySharedRoomPost(context.Background(), cwd, st, ev)
	}
	meet.SharedSessionID = func() (string, error) {
		cwd, _ := os.Getwd()
		sc, err := weave.EnsureRepoSession(context.Background(), cwd)
		if err != nil {
			return "", err
		}
		return sc.Pointer().TaskID, nil
	}
}

var (
	sharedInboxNotifierOnce sync.Once
	sharedInboxNotifier     *inboxChangeNotifier
)

func sharedInboxFingerprint(agent string) (uint64, bool) {
	sharedInboxNotifierOnce.Do(func() { sharedInboxNotifier = newInboxChangeNotifier() })
	return sharedInboxNotifier.fingerprint(agent)
}

// currentAgentSession returns the current tool session only when agentName is
// a registered fleet identity. The transport hashes this value before storing
// or comparing it; keeping registry policy here avoids making pkg/bus depend on
// the fleet catalog.
func currentAgentSession(agentName string) string {
	if agent, ok := fleet.New().Agent(agentName); ok {
		if tool, ok := fleet.New().Tool(agent.Tool); ok {
			return tool.CurrentSession()
		}
	}
	if id, isInst := bus.ExplicitInstanceID(agentName); isInst {
		if inst, err := fleet.NewInstanceStore("").Get(id); err == nil && inst.Active() {
			if raw := sessionFromInstanceCandidates(inst); raw != "" {
				return raw
			}
		}
	}
	if inst, found, err := bus.ResolveLiveInstanceByLabel(agentName); err == nil && found {
		if raw := sessionFromInstanceCandidates(inst); raw != "" {
			return raw
		}
	}
	if raw := sessionForBindingOrTool(agentName); raw != "" {
		return raw
	}
	return ""
}

func sessionFromInstanceCandidates(inst fleet.Instance) string {
	for _, candidate := range append([]string{inst.Label, inst.Family}, inst.Bindings...) {
		if raw := sessionForBindingOrTool(candidate); raw != "" {
			return raw
		}
	}
	return ""
}

func sessionForBindingOrTool(name string) string {
	toolName, _, _ := strings.Cut(strings.TrimSpace(name), ":")
	if tool, ok := fleet.New().Tool(toolName); ok {
		return tool.CurrentSession()
	}
	if tool, ok := fleet.New().Tool(bus.MainLineBase(name)); ok {
		return tool.CurrentSession()
	}
	return ""
}

func isSteerToolName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	t, ok := fleet.New().Tool(name)
	if !ok {
		return false
	}
	_, steer := t.SteerArgvPrefix("")
	return steer
}
