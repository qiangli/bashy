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

	"github.com/qiangli/coreutils/pkg/weavecli"
)

const nudgeSchemaVersion = "bashy-hint-v1"

// nudgeRules maps a legacy builtin name to the suggestion shown when an agent
// uses it. Only state-mutating builtins with a safer agentic counterpart belong
// here — their behavior is never changed, only annotated.
var nudgeRules = map[string]string{
	"cd":    "to run a single command elsewhere, prefer `awd DIR -- CMD` — it won't leave the shell in a new directory (cd persists and strands the next command).",
	"pushd": "for a one-off command in another directory, prefer `awd DIR -- CMD` over pushd/popd — no directory-stack state to unwind.",
	"popd":  "for a one-off command in another directory, prefer `awd DIR -- CMD` over pushd/popd — no directory-stack state to unwind.",
}

// nudger emits proactive tool-hints, rate-limited via the shared session memory.
type nudger struct {
	agent bool
	mem   *memory
	w     io.Writer
}

// newNudger shares the advisor's memory so hint rate-limiting is per session.
func newNudger(mem *memory) *nudger {
	return &nudger{agent: weavecli.IsAgent(), mem: mem, w: os.Stderr}
}

// onAudit is the [interp.WithAuditHandler] callback. It fires once per simple
// command (post-expansion); we act only on watched builtins, once per session.
func (n *nudger) onAudit(ev interp.AuditEvent) {
	if !ev.IsBuiltin || len(ev.Args) == 0 {
		return
	}
	suggest, ok := nudgeRules[ev.Args[0]]
	if !ok {
		return
	}
	if n.mem != nil && !n.mem.firstHint("builtin:"+ev.Args[0]) {
		return // already nudged for this tool this session
	}
	n.emit(ev.Args[0], suggest)
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
	return weavecli.IsAgent()
}
