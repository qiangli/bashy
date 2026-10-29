package agentos

import (
	"context"
	"io"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/yoke/pkg/dag"
)

// runSkillBashSharp uses the same decorator-aware execution seam as dag
// bodies (dagBashPPInterp): dag's CapExecHandler outermost, so a @guard cap
// inside the body denies exactly as it does in a dag target, and the host's
// job carrier, so `cmd &` in a skill body is the same probeable job it is
// everywhere else. The body handed in is the one unit yoke's catalog
// extracted (pkg/unit.BashSharpBody) — this seam never re-reads fences.
func runSkillBashSharp(ctx context.Context, dir string, env []string, body string, stdout, stderr io.Writer) (int, error) {
	prog, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(body), "SKILL.md")
	if err != nil {
		return 2, err
	}
	opts := []interp.RunnerOption{
		interp.Lang(syntax.LangBashPP),
		interp.Dir(dir),
		interp.Env(expand.ListEnviron(env...)),
		interp.ExecHandlers(dag.CapExecHandler()),
	}
	if carrier := cli.JobCarrier(); carrier != nil {
		opts = append(opts, interp.WithJobCarrier(carrier))
	}
	opts = wireExec(opts, false, env, nil, stdout, stderr, false)
	runner, err := interp.New(opts...)
	if err != nil {
		return 1, err
	}
	return dagExitCodeFromErr(runner.Run(ctx, prog))
}
