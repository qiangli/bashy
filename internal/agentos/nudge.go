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
// command (post-expansion); we act only on watched tools, once per session.
// Builtins (cd/pushd/popd) get the awd nudge; external search tools (grep/find)
// get an argv-conditioned routing hint toward --agentic / the yc code-intel verbs.
func (n *nudger) onAudit(ev interp.AuditEvent) {
	if len(ev.Args) == 0 {
		return
	}
	name := ev.Args[0]
	var suggest string
	if ev.IsBuiltin {
		suggest = nudgeRules[name]
	} else {
		suggest = routingHint(name, ev.Args)
	}
	if suggest == "" {
		return
	}
	if n.mem != nil && !n.mem.firstHint("nudge:"+name) {
		return // already nudged for this tool this session
	}
	n.emit(name, suggest)
}

// routingHint suggests a faster/structural path for legacy search tools, based
// only on the argv (no behavior change). Empty when there's nothing to suggest.
func routingHint(name string, args []string) string {
	switch name {
	case "grep":
		if hasArg(args, "--agentic") || !hasRecursiveFlag(args) {
			return ""
		}
		return "repo-wide grep also walks ignored noise (node_modules/.git/vendor). Add `--agentic` to skip it, or use `yc refs <symbol>` / `yc repomap` for structural, token-budgeted code search."
	case "find":
		if hasArg(args, "--agentic") {
			return ""
		}
		return "find walks ignored directories too. Add `--agentic` to skip .gitignore/node_modules, or use `yc symbols` / `yc repomap` to map the codebase."
	}
	return ""
}

func hasArg(args []string, want string) bool {
	for _, a := range args[1:] {
		if a == want {
			return true
		}
	}
	return false
}

// hasRecursiveFlag reports whether grep was asked to recurse (-r/-R, long forms,
// or a combined short cluster like -rn).
func hasRecursiveFlag(args []string) bool {
	for _, a := range args[1:] {
		switch a {
		case "-r", "-R", "--recursive", "--dereference-recursive":
			return true
		}
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsAny(a, "rR") {
			return true
		}
	}
	return false
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
