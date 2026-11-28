// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"errors"
	"fmt"
	"io"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
)

// THE FRONT-DOOR IDENTITY STEP.
//
// An agent started with its vendor CLI, not through `bashy <tool>`, reaches
// bashy with no name. Left alone, each verb settled that for itself: the
// board refused it, and everything else recorded the bare tool, so two
// sessions of one tool were one indistinguishable `claude` in a claim, a todo
// or a handoff.
//
// This step runs once, before any front-door verb, and does one of two
// things:
//
//   - CORRECT: when the session can be identified (its vendor session id, or
//     the harness process above this command), it is given its ad hoc name
//     and the process is stamped as a launcher would stamp it. The verb, and
//     every child it starts, then runs as that agent. The session is told its
//     name once.
//   - REFUSE: when it cannot be identified, a verb whose record names an actor
//     is refused, with the way to name the session. Recording the bare tool
//     there is the silent failure this replaces.
//
// It is in the front-door dispatch, so the bash and Bash# interpreter,
// scripts and the utilities never reach it; a human, a script and an agent
// that already has a name are not touched (bus.StampAdHocIdentity).

// agentIDRequiredVerbs are the coordination verbs: what they write names who
// did it, so an agent session nobody can name may not run them. Everything
// else runs unnamed as before — including `agent`, `instance`, `skill` and
// `commands`, which are how such a session learns to name itself.
var agentIDRequiredVerbs = map[string]bool{
	"mb": true, "messages": true, "inbox": true, "notify": true, "meet": true,
	"claim": true, "sprint": true, "todo": true, "issue": true, "weave": true,
	"handoff": true, "resume": true, "delegate": true, "coach": true,
	"foreman": true, "supervise": true, "ask": true, "bus": true, "feed": true,
	"herald": true, "activity": true,
}

// agentIdentityGate is the step. It returns the exit code to stop with and
// whether to stop.
func agentIdentityGate(argv []string, stderr io.Writer) (code int, refused bool) {
	if len(argv) == 0 || !isFrontDoorInvocation(argv[0]) {
		return 0, false
	}
	// The same two seams wireMessageBoard connects; a verb that never wires
	// the board still has to answer "which harness, which session".
	if bus.DetectHarness == nil {
		bus.DetectHarness = fleet.DetectTool
	}
	if bus.CurrentSessionClaim == nil {
		bus.CurrentSessionClaim = currentAgentSession
	}
	_, _, err := bus.StampAdHocIdentity()
	if err == nil || !errors.Is(err, bus.ErrAgentIDRequired) {
		return 0, false
	}
	if !agentIDRequiredVerbs[argv[0]] || asksForHelp(argv[1:]) {
		return 0, false
	}
	fmt.Fprintf(stderr, "bashy %s: %v\n%s\n", argv[0], err, bus.AgentIDGuidance())
	return 2, true
}

// asksForHelp reports a request for a verb's help, which is never refused: it
// is how a caller learns what the verb needs.
func asksForHelp(args []string) bool {
	for _, a := range args {
		switch a {
		case "--help", "-h", "help":
			return true
		case "--":
			return false
		}
	}
	return false
}
