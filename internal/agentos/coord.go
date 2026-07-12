// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"fmt"
	"os"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/handoff"
	"github.com/qiangli/coreutils/pkg/policy/coord"
	coreskills "github.com/qiangli/coreutils/pkg/skills"
)

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

// isGitWrite decides whether git's own arguments mutate shared state.
func isGitWrite(args []string) bool {
	for i, a := range args {
		if strings.HasPrefix(a, "-") {
			continue // a global flag: git -C <dir> commit
		}
		if a == "reset" {
			// `git reset --hard` destroys work; a plain `git reset` only unstages.
			// Guard the destructive form only -- a guard that fires on harmless
			// commands is a guard that gets switched off.
			return containsString(args[i:], "--hard")
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

// coordEnabled gates the whole mechanism.
//
// It keys on coreskills.DetectAgent(), NOT weavecli.IsAgent(). That distinction is
// load-bearing and was a live bug: IsAgent() checks only BASHY_AGENTIC, which is set
// in exactly one place — so a plain `claude` session with bashy as its shell is NOT
// "an agent" by that test. Gating on it would have made this middleware silently
// no-op in EXACTLY the sessions that collided. (The same wrong gate is why the
// advisor and the nudges are off in a normal Claude session today.)
func coordEnabled() bool {
	if v := os.Getenv("BASHY_CLAIM"); v == "0" || strings.EqualFold(v, "off") {
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
	if err := coord.Enforce(roots, "git "+strings.Join(args, " ")); err != nil {
		fmt.Fprintf(os.Stderr, "\nbashy: refusing `git %s`\n\n%v\n", strings.Join(args, " "), err)
		return coordExitRefused
	}
	return 0
}
