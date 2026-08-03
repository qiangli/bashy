// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// The weave isolation guard: warn BEFORE the edit lands, not after.
//
// weave detects that the live checkout moved while a run held its workspace,
// stamps `IsolationViolated` on the item, and makes `weave pull` refuse the
// merge. All correct, and all on the READ path — so whoever caused it finds out
// at the next `weave list`, often days later, when the only options left are
// --force or salvage.
//
// This exists because it happened. An agent in the steward seat committed three
// files to the live bashy checkout while run #35 was in flight; nothing said a
// word at commit time, and the violation surfaced later in an unrelated `weave
// list`. Detection after the fact is a diagnosis; what was wanted is a warning.
//
// bashy is the shell, so it is the ONE process that sees a mutating git command
// before it runs. That is the whole justification for the feature living here:
// no hook to install, nothing to remember, and it covers every agent CLI that
// routes its shell through bashy.
//
// # Sibling of coordHandler, and it borrows that file's hard-won lessons
//
// coord.go refuses a write when ANOTHER AGENT holds the project. This warns when
// A WEAVE RUN holds the checkout. Different source, different severity, same
// chokepoint — so it reuses coord's `isWrite` rather than growing a second
// argv matcher, and that reuse is load-bearing rather than tidy:
//
//   - `git` is a shell FUNCTION in every bashy session (the Preamble), so
//     `git commit` arrives as `bashy git commit`. A matcher keyed on
//     argv[0] == "git" catches /usr/bin/git and MISSES the only path an agent
//     actually uses — the same silent no-op this file exists to eliminate.
//   - `git reset --hard` destroys work; a plain `git reset` only unstages.
//     isWrite already draws that line.
//
// # It warns; coord refuses
//
// A weave violation is recoverable — `weave pull --force`, or salvage — and the
// operator often has good reason to edit the live checkout, including to fix
// what the run broke. Refusing would be wrong, and a guard that refuses gets
// switched off, at which point it protects nothing. One line to stderr, the
// command runs untouched, the exit code is never changed.
package agentos

import (
	"context"
	"fmt"
	"os"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/weave"
)

// weaveGuardEnabled gates the guard.
//
// It follows coordEnabled's gate, NOT the advisor's, and that choice is the one
// recorded in coord.go: `weavecli.IsAgent()` keys on BASHY_AGENTIC, which is set
// in exactly one place, so a plain `claude` session with bashy as its shell is
// not "an agent" by that test — and gating on it would silently no-op the
// middleware in exactly the sessions that collide. The session that caused the
// #35 violation was one of those.
func weaveGuardEnabled() bool {
	if v := strings.TrimSpace(os.Getenv("BASHY_WEAVE_GUARD")); v == "0" || strings.EqualFold(v, "off") {
		return false
	}
	return coordEnabled()
}

// weaveGuardStrict widens the query to runs awaiting `weave pull` as well as
// running ones. Off by default: a submitted run can sit unmerged for days and
// there are usually several, so warning on them would fire on nearly every
// commit — and a hint that arrives when nothing is wrong is how people learn to
// ignore hints.
func weaveGuardStrict() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("BASHY_WEAVE_GUARD")), "strict")
}

// weaveGuardHandler warns when a mutating git command is about to run in a repo
// a live weave run holds.
//
// The PRE-exec counterpart to the advisor's post-exec hint: the advisor explains
// a failure that already happened; this heads off a success that will cost
// someone their merge.
func weaveGuardHandler(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		// isWrite is the cheap argv-only pre-filter and rejects essentially
		// every command, so the store is not touched on the common path.
		if len(args) > 0 && isWrite(args) && weaveGuardEnabled() {
			cwd := interp.HandlerCtx(ctx).Dir
			if cwd == "" {
				cwd, _ = os.Getwd()
			}
			if line := weaveGuardWarning(cwd); line != "" {
				fmt.Fprintln(os.Stderr, line)
			}
		}
		return next(ctx, args)
	}
}

// weaveGuardWarning returns the advisory line, or "" when nothing holds the repo.
//
// Keyed on the REPO root, not the project root coord uses: weave isolation is a
// property of one checkout, and a queue records the repo it serves. Widening to
// the project would warn about a sibling repo's runs, which is exactly the kind
// of false positive that gets a guard disabled.
func weaveGuardWarning(cwd string) string {
	root := weave.RepoRootOf(cwd)
	if root == "" {
		return ""
	}
	holders := weave.HoldersOf(root, weave.HoldersQuery{Strict: weaveGuardStrict()})
	if len(holders) == 0 {
		return ""
	}

	ids := make([]string, 0, len(holders))
	for _, h := range holders {
		ids = append(ids, fmt.Sprintf("#%d (%s)", h.ID, h.State))
	}
	noun, verb := "run", "holds"
	if len(holders) > 1 {
		noun, verb = "runs", "hold"
	}
	return fmt.Sprintf(
		"bashy: weave %s %s %s this checkout — editing here marks them ISOLATION VIOLATED, "+
			"and `weave pull` then refuses without --force. Work inside the run's workspace "+
			"(`weave shell <id>`), or accept it knowingly. Silence: BASHY_WEAVE_GUARD=off",
		noun, strings.Join(ids, ", "), verb)
}
