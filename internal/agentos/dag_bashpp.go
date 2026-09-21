// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// dag_bashpp.go — the ```bsh dag body runs through the SAME runner wiring
// as `bashy --bashsharp FILE` (Sprint 216, Story 541).
//
// yoke/pkg/dag ships its own Bash# interpreter for ```bsh bodies, and it
// has to: yoke may not import this package, so the runner it builds carries
// the coreutils userland and the target's effect cap and nothing else — no
// decorator registry, no policy advice, no attestation sink. That is exactly
// what run 36 hit: `@guard(effects: "read")` on an agentic function inside a
// dag body printed BASHPP-EDECO-UNDEF, the call failed with status 1, and
// the target still exited 0 because its body never looked at the status. A
// contract that is not registered is not a contract that fails — it is a
// contract that is silently not there.
//
// The repair is wiring, not a second registry: re-register the bashpp tag
// with an interpreter that builds its runner through wireExec — the ONE seam
// every agentic runner in this binary goes through (main.go's cold CLI, the
// warm session, and now a dag body). Decorators (trace · guard · retry ·
// require · ensure), registration-time advice, the attestation sink, the
// audit/advisor/learn middleware and the registered-command ring all come
// with it, in the order wireExec already fixes. dag's own CapExecHandler
// stays OUTERMOST, exactly where yoke places it, so a target's `Effects:`
// denial keeps its diagnostic and its exit 126 (B17) — the auditHandler
// inside enforces the same advice.Cap a second time, silently, which is what
// `@guard` relies on inside a function.
//
// Classic (```bash and untagged) bodies are untouched: they carry no
// decorator syntax, and re-wiring them is not what the failure asked for.
package agentos

import (
	"context"
	"errors"
	"strings"
	"time"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/yoke/pkg/dag"
)

// dagBashPPInterp is the bashy-wired replacement for yoke's bashppInterp.
type dagBashPPInterp struct{}

// Run parses the body under syntax.LangBashPP and executes it in a fresh
// runner assembled by wireExec (per-recipe isolation, as in yoke). The
// exit-code mapping mirrors yoke's: an interp.ExitStatus is the shell's own
// status (a yield's 6 included), anything else is a fatal interpreter error.
func (dagBashPPInterp) Run(ctx context.Context, t *dag.Task, tio dag.TaskIO) dag.TaskResult {
	start := time.Now()
	res := dag.TaskResult{Name: t.Name}

	prog, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(t.Body), t.Name)
	if err != nil {
		res.Status, res.ExitCode, res.Err = dag.StatusFailed, 2, err
		res.Duration = time.Since(start)
		return res
	}

	opts := []interp.RunnerOption{
		interp.Lang(syntax.LangBashPP),
		interp.Dir(tio.Dir),
		interp.Env(expand.ListEnviron(tio.Env...)),
		// First appended = outermost: the target's declared-effects cap is
		// judged before any bashy middleware sees the command.
		interp.ExecHandlers(dag.CapExecHandler()),
	}
	// posix=false: a ```bashpp body is agentic by construction. No initial
	// dry-run — `bashy dag -n` plans without running any body at all.
	opts = wireExec(opts, false, tio.Env, nil, tio.Stdout, tio.Stderr, false)

	runner, err := interp.New(opts...)
	if err != nil {
		res.Status, res.ExitCode, res.Err = dag.StatusFailed, 1, err
		res.Duration = time.Since(start)
		return res
	}

	runErr := runner.Run(ctx, prog)
	res.Duration = time.Since(start)
	res.ExitCode, res.Err = dagExitCodeFromErr(runErr)
	if res.ExitCode == 0 {
		res.Status = dag.StatusDone
	} else {
		res.Status = dag.StatusFailed
	}
	return res
}

func dagExitCodeFromErr(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var st interp.ExitStatus
	if errors.As(err, &st) {
		return int(st), nil
	}
	return 1, err
}

// Package init order guarantees yoke/pkg/dag has registered its interpreters
// before this runs, so the override wins for every in-process dag run in the
// bashy binary — `bashy dag`, and any verb that drives the engine directly.
// It is registered under every spelling yoke accepts (bsh/bashsharp official,
// bashpp/bash++ aliases): a tag missing here would not fail, it would fall
// through to yoke's plain interpreter and run without the contracts.
func init() {
	bi := dagBashPPInterp{}
	for _, tag := range dag.BashSharpTags {
		dag.RegisterInterpreter(tag, bi)
	}
}
