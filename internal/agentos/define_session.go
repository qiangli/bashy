// Copyright (c) 2026, the bashy authors.
// See LICENSE for licensing information.

// Sprint 221, Story #580 (Story-ID c19cb824e6bd), B27b integration: the
// bashy half of `bashy define <name>` answering from a LIVE Bash# session.
//
// `define` is one of alwaysShimVerbs, so a bare `define NAME` inside any
// bashy shell is really the installed function `define() { command SELF
// define "$@"; }` (see Preamble) — and `command` forces that through the
// normal external-command dispatch, i.e. the ExecHandler ring, with
// args = [SELF, "define", NAME]. Before that reaches the ring's actual exec
// and forks a whole second bashy process to ask the projected-registry
// lexicon, this rung answers a plain variable name straight from the
// session it is already running in, via interp.HandlerContext.DescribeValue
// (../sh, cad5a8e3) — the same engine seat @timed's parity work landed
// alongside (see ../sh/plan-b27-timed-introspection.md).
//
// This is deliberately NOT an evaluator or a debugger: only a bare
// identifier is considered (defineSessionCandidate refuses flags, extra
// operands, and anything DescribeValue itself would refuse — a selector, an
// index, an expression), nothing is invoked or converted, and no cell is
// mutated. A miss — the name is not live in this session, or the call does
// not look like this exact shape at all — falls straight through to `next`,
// which is byte-identical to the fork-based front door `bashy define` has
// always used: a direct `bashy define NAME` (no running session at all,
// Dispatch's own case in agentos.go) never reaches this rung, and neither
// does any invocation this rung does not recognise.
package agentos

import (
	"context"
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// defineSessionHandler is the ExecHandler rung wiring HandlerContext.
// DescribeValue into `bashy define`. It sits ahead of the coreutils applet
// and registered-command rungs in wireExec: those never match a self
// re-exec of the bashy binary, so ordering only matters in that it must run
// before the real exec at the bottom of the chain forks a process.
func defineSessionHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			name, ok := defineSessionCandidate(args)
			if !ok {
				return next(ctx, args)
			}
			hc := interp.HandlerCtx(ctx)
			desc, found := hc.DescribeValue(name)
			if !found {
				// Not a live session value (unset, or a name only the projected
				// registries know) — the ordinary lexicon fallback answers,
				// unchanged, exactly as a direct front-door `bashy define` would.
				return next(ctx, args)
			}
			fmt.Fprint(hc.Stdout, renderSessionValue(name, desc))
			return nil
		}
	}
}

// defineSessionCandidate reports the plain variable name a self re-exec of
// `bashy define NAME` names, or false for anything this rung must leave
// alone: not a self re-exec of `define`, a flag (--json, --kind, --list-kinds),
// more than one operand, or a token that is not a plain shell identifier — a
// selector, an index, or an expression is refused here exactly like
// DescribeValue refuses it, never evaluated.
func defineSessionCandidate(args []string) (string, bool) {
	if len(args) != 3 || args[1] != "define" {
		return "", false
	}
	if args[0] != bashySelfPath() {
		switch args[0] {
		case "bashy", "bashy.real":
		default:
			return "", false
		}
	}
	name := args[2]
	if strings.HasPrefix(name, "-") || !syntax.ValidName(name) {
		return "", false
	}
	return name, true
}

// renderSessionValue formats one ValueDescription in the shape `define`
// prints a concept in: a header line naming what was asked about, then
// indented "key: value" lines. Redaction and nil-ness are always said
// explicitly, never inferred from an absent line.
func renderSessionValue(name string, d interp.ValueDescription) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s  (session %s)\n", name, d.Kind)
	fmt.Fprintf(&b, "  type: %s\n", d.Type)
	switch {
	case d.Nil:
		fmt.Fprintln(&b, "  value: nil")
	case d.Redacted:
		fmt.Fprintln(&b, "  value: ‹redacted› (opaque handle)")
	case d.Kind == "list", d.Kind == "map":
		fmt.Fprintf(&b, "  len:  %d\n", d.Len)
	case d.Kind == "record":
		fmt.Fprintln(&b, "  fields:")
		for _, f := range d.Fields {
			switch {
			case f.Redacted:
				fmt.Fprintf(&b, "    %s  %s  ‹redacted›\n", f.Name, f.Type)
			case f.Value != "":
				fmt.Fprintf(&b, "    %s  %s = %s\n", f.Name, f.Type, f.Value)
			default:
				fmt.Fprintf(&b, "    %s  %s\n", f.Name, f.Type)
			}
		}
	default:
		fmt.Fprintf(&b, "  value: %s\n", d.Value)
	}
	return b.String()
}
