// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// contracts.go — the Bash++ contract decorators, @require and @ensure
// (Sprint 203, design by contract at the process boundary). They ride the
// same interp.Decorators slot as trace · guard · retry and add nothing to the
// engine: a clause is one or more SHELL CHECKS (exit 0 = pass), the same
// contract `bashy dag`'s Require:/Ensure: lines follow, never an expression
// language and never anything generative.
//
//   - @require("<check>", ...) runs each check BEFORE the body; the first
//     failing check ends the call with status 3 (weavecli.ExitPrecondFail)
//     and `<fn>: precondition failed: <check>` on stderr. The body never runs.
//   - @ensure("<check>", ...) runs the body, then each check; a failing one
//     sets status 3 and `<fn>: postcondition failed: <check>`. The
//     postcondition judges a COMPLETED body only: a body that already failed
//     keeps its own status, and in particular an agentic yield (exit 6,
//     weavecli.ExitInputRequired) propagates untouched — a harness must never
//     see "postcondition failed" masking "input required".
//
// A check runs in a fresh in-process shell at the process boundary, with the
// call's arguments bound as $1..$n, the process environment and cwd, and —
// for @ensure — STATUS (the body's exit status) and RESULT (a typed
// function's first result value, else empty). It does not see the caller's
// shell variables: the engine exposes only Call, and the boundary is the
// point. The effect cap riding the ctx (@guard, outer) is enforced on the
// check's commands exactly as on the body's.
//
// Both are source-only (D6): an advised @require/@ensure is refused here, as
// @retry is, until the tighten-only advice ratchet has its own gate.
package agentos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/coreutils/pkg/weavecli"
	coreutilsshell "github.com/qiangli/coreutils/shell"
)

// contractDecorators returns the two clauses bound to the shell's stderr,
// where a failed clause names itself (the compiled-host slot installed at
// init binds the process stderr).
func contractDecorators(stderr io.Writer) map[string]nativeDecoratorFunc {
	return map[string]nativeDecoratorFunc{
		"require": requireDecorator(stderr),
		"ensure":  ensureDecorator(stderr),
	}
}

func requireDecorator(stderr io.Writer) nativeDecoratorFunc {
	return func(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
		checks, err := contractChecks("require", c, args)
		if err != nil {
			return err
		}
		if failed := runContractChecks(ctx, c, checks, false); failed != "" {
			contractFail(stderr, c, "precondition", failed)
			return nil
		}
		c.Next(ctx)
		return nil
	}
}

func ensureDecorator(stderr io.Writer) nativeDecoratorFunc {
	return func(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
		checks, err := contractChecks("ensure", c, args)
		if err != nil {
			return err
		}
		c.Next(ctx)
		if c.Status != 0 {
			// The body did not complete (a failure, or a yield): its status
			// is the verdict, and there is no result to judge.
			return nil
		}
		if failed := runContractChecks(ctx, c, checks, true); failed != "" {
			contractFail(stderr, c, "postcondition", failed)
		}
		return nil
	}
}

// contractChecks validates a clause's arguments: one or more positional
// string checks, no named arguments, never advised.
func contractChecks(name string, c *nativeDecoratorCall, args []interp.DecoratorArg) ([]string, error) {
	if c.Advised != "" {
		return nil, fmt.Errorf("%s is source-only and never applied by advice", name)
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("%s takes at least one check, e.g. %s(%q)", name, name, "test -n \"$1\"")
	}
	checks := make([]string, 0, len(args))
	for _, a := range args {
		if a.Name != "" {
			return nil, fmt.Errorf("%s takes positional checks only, not %q", name, a.Name)
		}
		if strings.TrimSpace(a.Value) == "" {
			return nil, errors.New(name + ": empty check")
		}
		checks = append(checks, a.Value)
	}
	return checks, nil
}

// runContractChecks evaluates checks in order and returns the first that
// fails, or "" when all pass.
func runContractChecks(ctx context.Context, c *nativeDecoratorCall, checks []string, post bool) string {
	params := make([]string, len(c.Args))
	for i, a := range c.Args {
		params[i] = fmt.Sprint(a)
	}
	env := os.Environ()
	if post {
		result := ""
		if len(c.Results) > 0 {
			result = fmt.Sprint(c.Results[0])
		}
		env = append(env, fmt.Sprintf("STATUS=%d", c.Status), "RESULT="+result)
	}
	dir, _ := os.Getwd()
	for _, check := range checks {
		if !contractCheckPasses(ctx, dir, env, params, check) {
			return check
		}
	}
	return ""
}

// contractCheckPasses runs one check through the in-process shell + the
// coreutils userland, with the ctx's effect cap enforced; exit 0 = pass. A
// check that does not even parse fails — an unreadable contract is not a
// satisfied one.
func contractCheckPasses(ctx context.Context, dir string, env, params []string, check string) bool {
	prog, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(check), "contract")
	if err != nil {
		return false
	}
	runner, err := interp.New(
		interp.Dir(dir),
		interp.Params(params...),
		interp.Env(expand.ListEnviron(env...)),
		interp.StdIO(nil, io.Discard, io.Discard),
		interp.ExecHandlers(auditHandler(nil, auditActor(), auditHost()), coreutilsshell.Handler()),
	)
	if err != nil {
		return false
	}
	return runner.Run(ctx, prog) == nil
}

// contractFail seals the call: status 3 and one line naming the function, the
// clause and the check. No engine diagnostic (an EDECO line would be "the
// decorator broke"; this is "the contract held and the call did not").
func contractFail(stderr io.Writer, c *nativeDecoratorCall, clause, check string) {
	c.Status = weavecli.ExitPrecondFail
	fmt.Fprintf(stderr, "%s: %s failed: %s\n", c.Name, clause, check)
}
