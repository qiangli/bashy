// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// One proactive hint rule, riding the same bashy-hint-v1 engine as the nudger
// (nudge.go): when an agent reads the board the long way — `mb --history`, a
// `bus` read, a bare `ping`, or the chat `timeline` — point it at `bashy inbox`,
// the cursor-safe view over every actionable unread source. Stderr-only (the
// command's stdout, including --json, is never touched), rate-limited once per
// (command, session), and gated exactly like every other hint.
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

// inboxReaderHint is the one line every reader surface names, kept byte-identical
// with yoke pkg/bus's InboxReaderLine (copied, not imported, so the pinned-module
// build stays green before that sibling lands).
const inboxReaderHint = "Messages to you: bashy inbox (peek by default; --from/--search/--since; reply with bashy mb send)"

// inboxReadSuggest returns the reader hint when args is one of the board-reading
// commands `bashy inbox` supersedes, or "" otherwise. Pure so it is unit-testable
// without wiring an interpreter, and deliberately conservative: a send, publish,
// or ICMP ping is never flagged.
func inboxReadSuggest(args []string) string {
	if len(args) == 0 {
		return ""
	}
	switch baseName(args[0]) {
	case "mb", "messages":
		// The whole board, read-or-not — the view `bashy inbox` narrows to the
		// unread that is actually for you.
		if hasLongFlag(args[1:], "history") {
			return inboxReaderHint
		}
	case "bus":
		// Read paths only; the publish/subscribe/sidecar write paths are left alone.
		switch firstOperand(args[1:]) {
		case "", "watch", "pending", "subscriptions":
			return inboxReaderHint
		}
	case "ping":
		// A bare ping reads the board; a target makes it a send or an ICMP probe.
		if firstOperand(args[1:]) == "" {
			return inboxReaderHint
		}
	case "chat":
		if firstOperand(args[1:]) == "timeline" {
			return inboxReaderHint
		}
	}
	return ""
}

// hasLongFlag reports whether args carries --name or --name=value.
func hasLongFlag(args []string, name string) bool {
	flag := "--" + name
	for _, a := range args {
		if a == flag || strings.HasPrefix(a, flag+"=") {
			return true
		}
	}
	return false
}

// firstOperand returns the first non-flag token in args, or "".
func firstOperand(args []string) string {
	for _, a := range args {
		if a == "" || strings.HasPrefix(a, "-") {
			continue
		}
		return a
	}
	return ""
}

// inboxHinter emits the inbox reader hint, rate-limited through the shared
// session memory so it teaches once rather than every turn.
type inboxHinter struct {
	agent bool
	mem   *memory
	w     io.Writer
}

func newInboxHinter(mem *memory) *inboxHinter {
	return &inboxHinter{agent: weavecli.IsAgentDriven(), mem: mem, w: os.Stderr}
}

// onAudit is an [interp.WithAuditHandler] callback. It fires once per simple
// command and acts only on the watched board-reading shapes, once per session.
func (h *inboxHinter) onAudit(ev interp.AuditEvent) {
	if len(ev.Args) == 0 {
		return
	}
	suggest := inboxReadSuggest(ev.Args)
	if suggest == "" {
		return
	}
	name := baseName(ev.Args[0])
	if h.mem != nil && !h.mem.firstHint("inbox-reader:"+name) {
		return
	}
	h.emit(name, suggest)
}

func (h *inboxHinter) emit(tool, suggest string) {
	if h.w == nil {
		return
	}
	if h.agent {
		b, _ := json.Marshal(nudgeLine{
			Schema:  nudgeSchemaVersion,
			Kind:    "hint",
			Tool:    tool,
			Suggest: suggest,
			Off:     "BASHY_HINTS=off",
		})
		fmt.Fprintf(h.w, "%s\n", b)
		return
	}
	fmt.Fprintf(h.w, "─── bashy hint ─── %s (silence: BASHY_HINTS=off)\n", suggest)
}
