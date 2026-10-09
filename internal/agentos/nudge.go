// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// The proactive half of the nudge subsystem (the advisor is the reactive half).
// When an agent uses a legacy tool that has a better agentic counterpart, bashy
// emits ONE rate-limited hint pointing at it — never changing the tool's
// behavior. First nudge: cd/pushd/popd → suggest `awd` (run one command
// elsewhere without leaking the shell's cwd).
//
// Prime invariant: help, don't obstruct. Nudges are stderr-only (stdout stays
// pure data), rate-limited to once per (tool, session) so they teach without
// becoming noise, and fully silenceable. They are observers — they never block
// or alter the command.
package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/nudge"
	"github.com/qiangli/coreutils/pkg/weavecli"
)

const nudgeSchemaVersion = nudge.SchemaVersion

// nudger emits proactive tool-hints, rate-limited via the shared session memory.
type nudger struct {
	agent bool
	mem   *memory
	w     io.Writer
}

// newNudger shares the advisor's memory so hint rate-limiting is per session.
func newNudger(mem *memory) *nudger {
	return &nudger{agent: weavecli.IsAgentDriven(), mem: mem, w: os.Stderr}
}

// onAudit is the [interp.WithAuditHandler] callback. It fires once per simple
// command (post-expansion); we act only on watched tools, once per session.
// Builtins (cd/pushd/popd) get the awd nudge; external search tools (grep/find)
// get an argv-conditioned routing hint toward --agentic / the code-intel verbs.
func (n *nudger) onAudit(ev interp.AuditEvent) {
	if len(ev.Args) == 0 {
		return
	}
	name := ev.Args[0]
	// Rules live in coreutils/pkg/nudge — the single source of truth shared with
	// ycode and any other consumer of the in-process userland. bashy keeps its
	// own session-memory rate-limiting + emit below.
	suggest := nudge.Suggest(ev.Args, ev.IsBuiltin)
	if suggest == "" {
		return
	}
	if n.mem != nil && !n.mem.firstHint("nudge:"+name) {
		return // already nudged for this tool this session
	}
	n.emit(name, suggest)
}

// nudgeLine is the agent-mode JSON shape (one line on stderr).
type nudgeLine struct {
	Schema  string `json:"schema_version"`
	Kind    string `json:"kind"` // "hint"
	Tool    string `json:"tool"`
	Suggest string `json:"suggest"`
	Off     string `json:"off"`
}

func (n *nudger) emit(tool, suggest string) {
	if n.w == nil {
		return
	}
	if n.agent {
		b, _ := json.Marshal(nudgeLine{
			Schema:  nudgeSchemaVersion,
			Kind:    "hint",
			Tool:    tool,
			Suggest: suggest,
			Off:     "BASHY_HINTS=off",
		})
		fmt.Fprintf(n.w, "%s\n", b)
		return
	}
	fmt.Fprintf(n.w, "─── bashy hint ─── %s (silence: BASHY_HINTS=off)\n", suggest)
}

// agenticDisabled reports the master off-switch: BASHY_AGENTIC set to an off-ish
// value turns off agentic defaults AND all nudges/advice.
func agenticDisabled() bool {
	switch strings.ToLower(os.Getenv("BASHY_AGENTIC")) {
	case "0", "false", "off", "no":
		return true
	}
	return false
}

// hintsEnabled reports whether proactive tool-hints should fire. BASHY_AGENTIC
// off is the master kill; otherwise BASHY_HINTS is the explicit control
// (off-ish silences, on-ish forces on); unset defaults to agent mode only.
func hintsEnabled() bool {
	if agenticDisabled() {
		return false
	}
	switch strings.ToLower(os.Getenv("BASHY_HINTS")) {
	case "0", "false", "off", "no":
		return false
	case "1", "true", "on", "yes":
		return true
	}
	return weavecli.IsAgentDriven()
}
