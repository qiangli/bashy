// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// The learning hook: every successful invocation teaches the host something.
//
// bashy is the shell, so the ExecHandler already holds the fully-expanded argv
// — variables resolved, globs expanded, quotes handled, aliases applied — plus
// the exit code. There is nothing to parse and nothing to intercept; the only
// question is what to do with it.
//
// What it does: on exit 0, extract the ENTITY the command targeted and the
// ROLES its arguments filled, and record them as facts. `ssh -p 2222 -l xuser
// remote-host` teaches that remote-host answers on 2222 as xuser — bound to the
// HOST rather than to ssh, so a later scp against the same machine can use it.
//
// That entity-binding is the whole point. Knowledge recorded against a command
// only helps re-running that command; knowledge recorded against a host helps
// everything that targets it.
package agentos

import (
	"context"
	"os"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/craft"
)

// learnEnabled reports whether passive learning is on.
//
// It follows the advisor's gating rather than inventing a posture of its own:
// the advisor already records host reachability on success, so this is the same
// kind of local memory rather than a new category of collection. BASHY_LEARN=off
// is the explicit kill switch, and BASHY_AGENTIC remains the master.
func learnEnabled() bool {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("BASHY_LEARN")), "off") {
		return false
	}
	return advisorEnabled() || hintsEnabled()
}

// learnHandler is the post-exec middleware that records what an invocation
// taught.
//
// Three properties it must have, because it sits on the hot path of every
// command:
//
//   - It never changes the outcome. The underlying error is returned unchanged
//     on every path, and a store failure is swallowed: a shell that fails to
//     run a command because its note-taking broke would be indefensible.
//   - It does nothing for commands it has no schema for, which is most of them.
//     The lookup is one map hit.
//   - It writes only on success. See below.
func learnHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			err := next(ctx, args)
			if len(args) == 0 {
				return err
			}
			status, ok := exitStatusOf(err)
			if !ok || status != 0 {
				// FAILURE TEACHES NOTHING HERE, and this is a deliberate
				// reversal of the obvious design.
				//
				// "Invalidate on failure" sounds right until you ask WHY the
				// command failed. `ssh -p 2222 -l xuser host` can fail because
				// the port is wrong, or the login is wrong — or because the host
				// is rebooting, the VPN dropped, or DNS is down. In three of
				// those five the facts are perfectly good, and deleting them
				// would erode the store every time the network hiccuped.
				//
				// A failure is UNATTRIBUTED: it does not say which of the
				// arguments was at fault. Treating it as evidence against a
				// specific fact is exactly the absence-of-evidence error, run in
				// reverse — inferring a value is wrong from the absence of
				// success.
				//
				// Correction still happens, through supersession: when a
				// DIFFERENT value later succeeds, Record closes the old one. So
				// the store self-corrects on positive evidence, which is the
				// only kind that identifies the culprit.
				//
				// Attributing failures needs the error text classified (which
				// role does "Permission denied" implicate?). That is a real
				// capability and it belongs here later; it is not a reason to
				// guess in the meantime.
				return err
			}

			x, usable := craft.Extract(args)
			if !usable {
				return err
			}
			store := craft.OpenFacts(craftStoreDir())
			for _, f := range craft.FactsFrom(x, "exec:"+args[0]) {
				// Best effort by design: a fact that cannot be written is a
				// missed lesson, never a failed command.
				_ = store.Record(f)
			}
			return err
		}
	}
}
