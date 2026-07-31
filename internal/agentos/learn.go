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
	"errors"
	"fmt"
	"os"
	"os/exec"
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
			args = commandArgv(args)
			if len(args) == 0 {
				return err
			}
			status, ok := observedExitOf(err)
			if !ok {
				return err // a non-exit error (an interrupt): nothing was observed
			}
			verdict := craft.Classify(args[0], status)

			// A PAYLOAD failure is not a failure of anything this package knows
			// about. The session was established; the thing it carried returned
			// non-zero on its own merits. So it is treated exactly like success:
			// the connection arguments are recorded as PROVEN, and nothing is
			// said.
			//
			// Both halves of that matter. Recording widens the evidence base to
			// the common case — most invocations run something that can fail —
			// and staying quiet keeps the hint channel credible, since a
			// suggestion about a host that was never the problem is precisely
			// how people learn to ignore suggestions.
			speak, record := routeVerdict(verdict, status)
			if speak {
				// A transport failure is the moment to SPEAK, not to write.
				suggestKnown(args)

				// FAILURE TEACHES NOTHING, and this is a deliberate reversal of
				// the obvious design.
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

			if !record {
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

// routeVerdict turns a classified exit into the two decisions this hook makes.
//
// It is a named function rather than two inline conditions because the pairing
// is the whole design and it is easy to get subtly wrong: the cases are not
// complements. A payload failure neither speaks NOR stays silent about
// recording — it records, exactly like success, because the session provably
// worked. A transport failure is the mirror image. Written inline, a later edit
// that "simplifies" one branch can silently decouple them.
//
//	verdict      speak   record
//	success      no      YES     nothing went wrong
//	payload      no      YES     the session worked; its payload did not
//	transport    YES     no      the arguments are suspect
//	unknown      YES     no      cannot interpret; hint, but never learn
func routeVerdict(verdict craft.Verdict, status int) (speak, record bool) {
	if status == 0 {
		return false, true
	}
	return verdict != craft.VerdictPayload, verdict.Confirms()
}

// commandArgv strips bashy's own front-door invocation so the argv names the
// command a person actually typed.
//
// bashy shims its front-door verbs as shell functions:
//
//	kubectl() { command /path/to/bashy kubectl "$@"; }
//	docker()  { command /path/to/bashy podman  "$@"; }
//
// So by the time the ExecHandler sees it, `kubectl --context prod get pods` has
// become `/path/to/bashy kubectl --context prod get pods` — argv[0] is the
// bashy binary, and no lookup on it can ever match. Every managed CLI was
// invisible to this hook for exactly that reason, and invisibly so: they simply
// never taught or offered anything.
//
// Note what the docker shim does — it does not just re-spell the command, it
// REPLACES it. `docker` on a bashy shell IS `bashy podman`, so the name that
// reaches here is podman and a spec keyed only on "docker" would never fire.
func commandArgv(args []string) []string {
	if len(args) < 2 {
		return args
	}
	if args[0] != bashySelfPath() {
		return args
	}
	// `bashy -c …` and friends are bashy operating on itself, not a front-door
	// verb. A flag in the first slot is the tell.
	if strings.HasPrefix(args[1], "-") {
		return args
	}
	return args[1:]
}

// observedExitOf reports the exit status of a command that actually RAN.
//
// It is deliberately broader than exitStatusOf, which only recognises the
// shell's own interp.ExitStatus. Commands bashy dispatches itself — the
// managed CLIs like kubectl and docker, and the cobra front-door verbs — do not
// return that type; they return an *exec.ExitError or a wrapped error carrying
// the status.
//
// The narrow check made the whole learning layer silently skip exactly those
// commands. Nothing reported it: they simply never taught anything and never
// offered anything, which is indistinguishable from having no facts about them.
// A spec for a command the hook cannot see is inert, and an inert spec reads as
// coverage while providing none.
//
// A truly unclassifiable error still returns false. "The command ran and
// failed" and "the command never ran" are different claims, and only the first
// is evidence.
func observedExitOf(err error) (int, bool) {
	if err == nil {
		return 0, true
	}
	var st interp.ExitStatus
	if errors.As(err, &st) {
		return int(st), true
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode(), true
	}
	return 0, false
}

// suggestKnown offers what this host already knows about the target, when a
// command fails without using it.
//
// The moment is chosen, not incidental. A failure against a host bashy has
// facts for is the highest-precision instant to speak: the failure is itself
// evidence the caller got something wrong, so a suggestion is wanted rather
// than interrupting. Proactive hinting at the prompt has a far worse precision
// profile, because nothing has signalled that help is welcome.
//
// Four conditions must ALL hold, which is what keeps this quiet:
//
//   - the command declared a target entity this host knows facts about;
//   - the fact was learned from a command in the SAME auth realm — an ssh login
//     is not a psql role, and suggesting one for the other is a confident wrong
//     answer;
//   - THIS command can express the role as a flag (scp takes a login as
//     user@host, not -l, so a login is simply not offered for scp);
//   - the invocation did not already supply it.
//
// That last one matters most for trust. Telling someone to pass a flag they
// just passed is the fastest way to teach them to ignore every future hint —
// and once hints are ignored the system is worse than one that never spoke.
func suggestKnown(args []string) {
	if len(args) == 0 {
		return
	}
	// Extract's bool result requires roles to be present; here the whole point
	// is that they may be MISSING, which is usually why the command failed. The
	// entity is populated regardless, so it is read directly.
	x, _ := craft.Extract(args)
	if !x.Entity.Valid() {
		return
	}

	facts := craft.OpenFacts(craftStoreDir()).For(x.Entity)
	if len(facts) == 0 {
		return
	}

	var offers []string
	for _, f := range facts {
		role := craft.Role(f.Key)
		// Already supplied, with this value: saying it again is noise.
		if have, used := x.Roles[role]; used && have == f.Value {
			continue
		}
		// Provenance decides whether the fact travels. It is either the command
		// that taught it or a declared source (an ssh config), and both resolve
		// to a realm — a fact whose realm cannot be resolved is skipped rather
		// than assumed into one.
		if !craft.TransfersTo(f.Source, args[0]) {
			continue
		}
		rendered, ok := craft.RenderRole(args[0], role, f.Value)
		if !ok {
			continue
		}
		offers = append(offers, rendered)
	}
	if len(offers) == 0 {
		return
	}

	// One line, and a SUGGESTION — never applied. The caller may have a reason
	// to use a different account for file transfer than for a shell, and an
	// agent that silently rewrote the command would be wrong in a way nothing
	// reported.
	fmt.Fprintf(os.Stderr, "bashy: %s is known here as: %s\n",
		x.Entity.Name, strings.Join(offers, " "))
}
