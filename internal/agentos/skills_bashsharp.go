package agentos

import (
	"context"
	"io"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// runSkillBashSharp uses the same decorator-aware execution seam as dag bodies.
func runSkillBashSharp(ctx context.Context, dir string, env []string, body string, stdout, stderr io.Writer) (int, error) {
	prog, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(body), "SKILL.md")
	if err != nil {
		return 2, err
	}
	opts := []interp.RunnerOption{
		interp.Lang(syntax.LangBashPP),
		interp.Dir(dir),
		interp.Env(expand.ListEnviron(env...)),
	}
	opts = wireExec(opts, false, env, nil, stdout, stderr, false)
	runner, err := interp.New(opts...)
	if err != nil {
		return 1, err
	}
	return dagExitCodeFromErr(runner.Run(ctx, prog))
}
