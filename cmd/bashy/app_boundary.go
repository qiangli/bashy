//go:build !bashy_core && !bashy_cert_base

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/qiangli/coreutils/shell"
	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/yoke/pkg/shape/boundary"
	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// appBoundaryMain enforces a shaped app's data boundary (S525.A5d). The
// console runs each unit as `bashy -c BODY NAME` with its policy in
// $BASHY_APP_BOUNDARY; when that variable is set this entry point builds the
// guard with yoke's boundary.FromEnv and interprets BODY under the same
// handlers yoke's own runner installs (as boundary.Run: open/stat/cd path
// checks, the exec gate, /dev/tcp refusal) — identical on every OS.
//
// handled is false when the variable is unset or the invocation is not a
// -c command, leaving behaviour unchanged. A set but malformed policy, or a
// -c invocation carrying options the guarded runner does not honour, refuses
// to run (status 126): never run a bounded unit unconfined.
func appBoundaryMain(args []string, stdin io.Reader, stdout, stderr io.Writer) (code int, handled bool) {
	if _, set := os.LookupEnv(boundary.EnvVar); !set {
		return 0, false
	}
	body, name, isC, err := appBoundaryCommand(args)
	if !isC {
		return 0, false
	}
	if err != nil {
		fmt.Fprintf(stderr, "bashy: %s: %v; refusing to run\n", boundary.EnvVar, err)
		return boundary.DeniedStatus, true
	}
	g, _, err := boundary.FromEnv(nil)
	if err != nil {
		fmt.Fprintf(stderr, "bashy: %v; refusing to run\n", err)
		return boundary.DeniedStatus, true
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "bashy: %s: working directory: %v; refusing to run\n", boundary.EnvVar, err)
		return boundary.DeniedStatus, true
	}
	code, err = runBounded(context.Background(), g, body, name, dir, os.Environ(), stdin, stdout, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "bashy: %v\n", err)
	}
	return code, true
}

// runBounded is boundary.Run plus interp.ServedInProcess, which every bashy
// runner sets: without it the second call of an in-process tool (cat, ls,
// ...) is hash-rewritten to its PATH file, and the guard refuses that as an
// undeclared external program. The guard, its handlers and decorators are
// yoke's, unchanged.
func runBounded(ctx context.Context, g *boundary.Guard, script, name, dir string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	f, err := syntax.NewParser(syntax.Variant(syntax.LangBashPP)).Parse(strings.NewReader(script), name)
	if err != nil {
		return 2, err
	}
	opts := append(g.RunnerOptions(shell.Handler()),
		interp.ServedInProcess(func(name string) bool { return tool.Lookup(name) != nil }),
		interp.Decorators(boundary.Decorators()),
		interp.Dir(dir), interp.Env(expand.ListEnviron(env...)), interp.StdIO(stdin, stdout, stderr))
	r, err := interp.New(opts...)
	if err != nil {
		return 2, err
	}
	if err := r.Run(ctx, f); err != nil {
		if st, ok := interp.IsExitStatus(err); ok {
			return int(st), nil
		}
		return 1, err
	}
	return 0, nil
}

// appBoundaryCommand recognises `-c BODY [NAME]` as args[1:]. isC reports
// a -c invocation; anything else (a script file, a subcommand, stdin) is not
// this entry point's business. err is set for a -c shape the guarded runner
// cannot run faithfully — options around -c (-e -c, -ec, -o x -c), or
// positional arguments it would drop — so it refuses rather than guesses.
func appBoundaryCommand(args []string) (body, name string, isC bool, err error) {
	if len(args) > 1 && args[1] == "-c" {
		switch {
		case len(args) < 3:
			return "", "", true, fmt.Errorf("-c needs a command")
		case len(args) > 4:
			return "", "", true, fmt.Errorf("positional arguments after -c BODY NAME are not supported under an app boundary")
		}
		name = "bashy"
		if len(args) == 4 {
			name = args[3]
		}
		return args[2], name, true, nil
	}
	for _, a := range args[1:] {
		if len(a) > 1 && a[0] == '-' && a[1] != '-' && strings.ContainsRune(a[1:], 'c') {
			return "", "", true, fmt.Errorf("option %s with -c is not supported under an app boundary", a)
		}
	}
	return "", "", false, nil
}
