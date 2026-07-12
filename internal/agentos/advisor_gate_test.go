// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"testing"

	"github.com/qiangli/coreutils/pkg/fleet"
)

// The advisor exists to stop an agent from re-running a command that CANNOT succeed
// (wrong cwd, host gone remote, disk full). It was DARK in every human-launched agent
// session -- the only place it was ever meant to fire.
//
// The cause: it gated on weavecli.IsAgent(), which reads only BASHY_AGENTIC, and
// BASHY_AGENTIC is set in exactly one place -- when bashy spawns a worker itself. A
// human who types `claude` (with bashy as its shell, via `bashy install-agent`) sets
// nothing. So the feature shipped, was documented, and never ran.
func TestAdvisorFiresForAHumanLaunchedAgent(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("CLAUDECODE", "1") // a human typed `claude`; bashy did not spawn this

	if !advisorEnabled() {
		t.Fatal(`the advisor is OFF in a human-launched Claude session.

This is the bug: BASHY_AGENTIC is set only when bashy spawns a worker, so the advisor --
whose entire job is to stop an AGENT from looping on a doomed command -- never fired for
the agents that actually run this shell.`)
	}
}

// A human at a terminal still gets a quiet shell. Over-firing is how a helpful feature
// becomes a thing people disable.
func TestAdvisorIsQuietForAHuman(t *testing.T) {
	clearAgentEnv(t)
	if advisorEnabled() {
		t.Fatal("the advisor fires for a plain human shell — unsolicited advice on every failed command")
	}
}

// The explicit controls still win, in both directions.
func TestAdvisorExplicitOverrides(t *testing.T) {
	clearAgentEnv(t)
	t.Setenv("BASHY_ADVISOR", "1")
	if !advisorEnabled() {
		t.Error("BASHY_ADVISOR=1 did not force the advisor on for a human")
	}
	t.Setenv("CLAUDECODE", "1")
	t.Setenv("BASHY_ADVISOR", "0")
	if advisorEnabled() {
		t.Error("BASHY_ADVISOR=0 did not force the advisor off for an agent")
	}
}

// clearAgentEnv scrubs every agent signal. The marker list is queried from the registry,
// never hardcoded: it is data (`bashy tools add` extends it), and a stale literal would
// leave the very marker that matters set.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	t.Setenv("BASHY_AGENTIC", "")
	t.Setenv("BASHY_ADVISOR", "")
	for _, env := range fleet.MarkerEnvs() {
		t.Setenv(env, "")
	}
}
