// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync/atomic"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/yoke/pkg/atlas"
	_ "github.com/qiangli/yoke/pkg/fleet/fleetkinds"
	"github.com/qiangli/yoke/pkg/handoff"
	"github.com/qiangli/yoke/pkg/policy/coord"
	coreskills "github.com/qiangli/yoke/pkg/skills"
)

func init() {
	if _, ok := coord.LookupKind("command"); !ok {
		coord.RegisterKind(coord.Kind{Name: "command", Match: coord.MatchName})
	}
	if _, ok := coord.LookupKind("sandbox"); !ok {
		coord.RegisterKind(coord.Kind{Name: "sandbox", Match: coord.MatchName})
	}
	if _, ok := coord.LookupKind("ollama"); !ok {
		coord.RegisterKind(coord.Kind{Name: "ollama", Match: coord.MatchName})
	}
	if _, ok := coord.LookupKind("inbox"); !ok {
		coord.RegisterKind(coord.Kind{Name: "inbox", Match: coord.MatchName})
	}
}

// coordHandler refuses a WRITE when another agent already holds this project.
//
// # Why it lives in the shell, and not in a document
//
// No document can be made mandatory. Different agent tools read different files;
// ycode truncates instruction files at 4 KB and reads AGENTS.md first; aider reads
// nothing at all. But `bashy install-agent` and the agent runner have already made
// bashy the SHELL under every one of them — so a rule enforced here reaches Claude,
// Codex, OpenCode, aider, Gemini and Copilot alike, WITHOUT any of them reading
// anything.
//
// And it sees the RESOLVED ARGV of every external command, so it survives
// `unset -f git` and `/usr/bin/git`, which the Preamble's shell-function shims do
// not.
//
// # The refusal IS the documentation
//
// An agent that read no documentation learns the rule the first time it tries to
// break it — and the message names who holds the project, what they are doing, and
// what to do instead. That is a better teacher than a paragraph nobody loads.
//
// # Refuse on CONFLICT, never on absence
//
// The claim is taken SILENTLY on the first write. You are stopped only when someone
// else already holds one. Friction that fires when you are alone on the machine is
// friction nobody accepts — and a rule nobody accepts is a rule nobody follows.
func coordHandler(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if len(args) == 0 || !isWrite(args) {
			return next(ctx, args)
		}
		cwd := interp.HandlerCtx(ctx).Dir
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		// The PROJECT, not the repo: a claim keyed on one .git root would not have
		// prevented the regression that prompted this — it spanned three repos.
		roots := handoff.ProjectRoots(projectRootOf(cwd))
		if err := coord.Enforce(roots, strings.Join(args, " ")); err != nil {
			// Return WITHOUT calling next: the command does not run. Modelled on
			// dryRunHandler, which is the working precedent for a middleware that
			// refuses.
			fmt.Fprintf(os.Stderr, "\nbashy: refusing `%s`\n\n%v\n", strings.Join(args, " "), err)
			return interp.ExitStatus(coordExitRefused)
		}
		return next(ctx, args)
	}
}

// coordExitRefused is distinct from 1, so a caller can tell "another agent holds
// this project" from "the command failed".
const coordExitRefused = 9

// writeVerbs are the commands that MUTATE shared state — the ones where two agents
// collide destructively and irreversibly.
//
// Deliberately NARROW. A read is never blocked; neither is a build, a test, or a
// grep. The cost of a false refusal is an agent that cannot work and a human who
// disables the guard; the cost of a missed write is a collision. So this list guards
// the operations that actually caused the failure — committing, pushing, merging,
// rebasing, resetting — and nothing else.
var writeVerbs = map[string]bool{
	"commit": true, "push": true, "merge": true, "rebase": true,
	"cherry-pick": true, "revert": true, "am": true,
}

// isWrite decides whether an EXTERNAL argv mutates shared state.
//
// It must see through the shim. `git` is a shell FUNCTION in every bashy session
// (the Preamble), so `git commit` reaches the ExecHandler as `bashy git commit` --
// argv[0] is "bashy", not "git". A naive check on argv[0] therefore misses the
// only path an agent actually uses, and catches only `/usr/bin/git`. Unwrap the
// wrapper first.
func isWrite(args []string) bool {
	// Unwrap: `bashy git commit` / `command bashy git commit` -> `git commit`.
	for len(args) > 1 && (baseName(args[0]) == "bashy" || baseName(args[0]) == "command") {
		args = args[1:]
	}
	if baseName(args[0]) != "git" || len(args) < 2 {
		return false
	}
	return isGitWrite(args[1:])
}

// gitGlobalFlagsWithValue are git's global options whose VALUE is a separate
// argument. Skipping only the flag leaves the value looking like a subcommand,
// so `git -C /path commit` resolves the verb as "/path" and matches nothing —
// which is how the -C form this function's comment claims to handle was in fact
// missed entirely.
var gitGlobalFlagsWithValue = map[string]bool{
	"-C": true, "-c": true, "--git-dir": true, "--work-tree": true,
	"--namespace": true, "--exec-path": true,
}

// isGitWrite decides whether git's own arguments mutate shared state.
func isGitWrite(args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if gitGlobalFlagsWithValue[a] {
			i++ // consume the value too
			continue
		}
		if strings.HasPrefix(a, "-") {
			continue // a valueless global flag, or --opt=value
		}
		if a == "reset" {
			// `git reset --hard` destroys work; a plain `git reset` only unstages.
			// Guard the destructive form only -- a guard that fires on harmless
			// commands is a guard that gets switched off.
			//
			// slices.Contains, NOT containsString: that helper is a
			// sort.SearchStrings BINARY SEARCH and requires its input sorted.
			// argv never is, so `git reset --hard` searched an unsorted slice
			// and reported not-found — the one destructive form this branch
			// exists to catch was the one it missed.
			return slices.Contains(args[i:], "--hard")
		}
		return writeVerbs[a]
	}
	return false
}

func projectRootOf(dir string) string {
	if r := detectProjectRoot(dir); r != "" {
		return r
	}
	return dir
}

// claimDisabled reports the explicit operator switch: BASHY_CLAIM=0|off.
func claimDisabled() bool {
	v := os.Getenv("BASHY_CLAIM")
	return v == "0" || strings.EqualFold(v, "off")
}

// coordEnabled gates the whole mechanism.
//
// It keys on coreskills.DetectAgent(), NOT weavecli.IsAgent(). That distinction is
// load-bearing and was a live bug: IsAgent() checks only BASHY_AGENTIC, which is set
// in exactly one place — so a plain `claude` session with bashy as its shell is NOT
// "an agent" by that test. Gating on it would have made this middleware silently
// no-op in EXACTLY the sessions that collided. (The same wrong gate is why the
// advisor and the nudges are off in a normal Claude session today.)
func coordEnabled() bool {
	if claimDisabled() {
		return false
	}
	_, isAgent := coreskills.DetectAgent()
	return isAgent
}

// coordGuard is the front-door choke point for `bashy git …`.
//
// The Preamble shims `git` to `command bashy git`, so an agent that types
// `git commit` in a bashy shell arrives at the front-door dispatch — IN-PROCESS,
// never touching an ExecHandler. Enforcing only in the middleware would therefore
// have guarded only `/usr/bin/git`, which is the one path an agent almost never
// takes.
//
// That is not hypothetical: it was live in the first build of this feature, and a
// second agent committed straight through it during the very test meant to prove it
// could not. Two choke points, because there are two paths.
//
// Returns 0 to proceed, or the exit code to die with.
func coordGuard(args []string) int {
	if !coordEnabled() || !isGitWrite(args) {
		return 0
	}
	cwd, _ := os.Getwd()
	roots := handoff.ProjectRoots(projectRootOf(cwd))
	if err := coordEnforceFn(roots, "git "+strings.Join(args, " ")); err != nil {
		var conf *coord.Conflict
		if errors.As(err, &conf) {
			fmt.Fprintf(os.Stderr, "\nbashy: refusing `git %s`\n\n%v\n", strings.Join(args, " "), err)
			return coordExitRefused
		}
		claimCheckUnavailable(err)
	}
	return 0
}

// coordEnforceFn and claimGuardFn are the ledger calls the guards make, held in
// vars so a test can stand in a failing ledger without corrupting a real one.
var (
	coordEnforceFn = coord.Enforce
	claimGuardFn   = coord.Guard
)

// claimCheckWarned records that this process has already said the ledger is down.
var claimCheckWarned atomic.Bool

// claimCheckUnavailable is the fail-OPEN half of every guard.
//
// Only a *coord.Conflict is a refusal. Anything else coord.Guard/Enforce returns —
// a lock it could not take, an unreadable ~/.bashy/coord, a short read — is a fault
// in the advisory ledger, not a verdict about the command, and an advisory ledger
// that bricks every command in an agent session the moment its own storage hiccups
// has stopped being advisory. So the guard says so once per process, on stderr,
// and lets the command run.
func claimCheckUnavailable(err error) {
	if claimCheckWarned.CompareAndSwap(false, true) {
		fmt.Fprintf(os.Stderr, "bashy: claim check unavailable: %v; proceeding\n", err)
	}
}

// claimRefused classifies a guard error: true (after printing the conflict) when
// the command must die with coordExitRefused, false when it may proceed — either
// because there was no error or because the ledger itself failed (see
// claimCheckUnavailable).
func claimRefused(err error) bool {
	if err == nil {
		return false
	}
	var conf *coord.Conflict
	if errors.As(err, &conf) {
		fmt.Fprint(os.Stderr, conf.Error())
		return true
	}
	claimCheckUnavailable(err)
	return false
}

// claimRegisteredLookup is the registered-ring lookup the claim guard uses,
// held in a var so a test can count how often a command pays for it.
var claimRegisteredLookup = registeredLookup

// isShippedVerb reports whether name is a command bashy itself ships — a shell
// builtin, a coreutils applet, or a front-door verb. A registered record can
// never shadow one (see reservedCommandName), so resolving it through the ring
// is wasted work: registeredLookup stats and lists the ring directories on
// every call. In-memory lookups only.
func isShippedVerb(name string) bool {
	if interp.IsBuiltin(name) || tool.Lookup(name) != nil {
		return true
	}
	if _, ok := atlas.Lookup(name); ok {
		return true
	}
	for _, list := range [][]string{alwaysShimVerbs, directFrontDoorVerbs, agentModeShimVerbs, hiddenFrontDoorVerbs, curatedHiddenVerbs, {"docker", "sandbox"}, dispatchOnlyNames} {
		if slices.Contains(list, name) {
			return true
		}
	}
	return false
}

// resolveCommandVerb unwraps `bashy X` / `command bashy X`, returning the verb
// and the arguments after it. A registered command alias resolves to its
// canonical name; shipped commands skip the ring lookup entirely.
func resolveCommandVerb(args []string) (verb string, rest []string) {
	for len(args) > 1 && (baseName(args[0]) == "bashy" || baseName(args[0]) == "command") {
		args = args[1:]
	}
	if len(args) == 0 {
		return "", nil
	}
	verb = baseName(args[0])
	if !isShippedVerb(verb) {
		if r, ok := claimRegisteredLookup(verb); ok && r.Name != "" {
			verb = r.Name
		}
	}
	return verb, args[1:]
}

// isClaimExemptCommand reports whether a command verb is exempt from claim guarding.
// The coordination and help tools must stay usable while blocked.
func isClaimExemptCommand(verb string) bool {
	if verb == "" || verb == "bashy" || strings.HasPrefix(verb, "-") {
		return true
	}
	switch verb {
	case "claim", "inbox", "ping", "mb", "meet", "help":
		return true
	default:
		return false
	}
}

// claimGuardMiddleware stops an agent from running a command claimed by another agent.
// It covers BOTH front-door verbs (Dispatch -> observing chain) and in-shell external commands.
//
// One invocation takes ONE coord.Guard call: the command use, plus the engine
// use when the verb is a container/LLM front door. Each Guard call takes
// claims.lock, reads the ledger and enumerates backends, so the front door
// reuses this call (engineClaimCovered) instead of making its own.
func claimGuardMiddleware(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(ctx context.Context, args []string) error {
		if !coordEnabled() || len(args) == 0 {
			return next(ctx, args)
		}
		verb, rest := resolveCommandVerb(args)
		if isClaimExemptCommand(verb) {
			return next(ctx, args)
		}
		uses := []coord.Use{{Kind: "command", Name: verb}}
		engineUse, isEngine := engineClaimUse(verb, rest)
		if isEngine {
			uses = append(uses, engineUse)
		}
		if claimRefused(claimGuardFn(ctx, coord.Self(), uses...)) {
			return interp.ExitStatus(coordExitRefused)
		}
		if isEngine {
			engineClaimCovered.Store(true)
		}
		return next(ctx, args)
	}
}
