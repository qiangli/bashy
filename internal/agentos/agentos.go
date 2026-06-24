// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package agentos holds the AgentOS wiring that turns the shell core into the
// `bashy` system shell: the coreutils ExecHandler (so the pure-Go userland and
// `yc` code-intel verbs run in-process) and the front-door subcommands
// (`bashy weave …`, `bashy podman …`).
//
// It is imported ONLY by cmd/bashy — never by cmd/bash. That is what keeps the
// two binaries independent: the pure `bash` drop-in's import graph never pulls
// in coreutils or any external-tool surface, so it stays a lean GNU Bash 5.3
// drop-in, while `bashy` is the self-contained bootstrapper for a whole
// unix-like userland (bash + coreutils + pkg + external tools).
package agentos

import (
	"context"
	"os"

	"mvdan.cc/sh/v3/interp"

	_ "github.com/qiangli/coreutils/cmds/all"
	"github.com/qiangli/coreutils/external/podman"
	"github.com/qiangli/coreutils/pkg/dag"
	"github.com/qiangli/coreutils/pkg/secrets"
	"github.com/qiangli/coreutils/pkg/weave"
	coreutilsshell "github.com/qiangli/coreutils/shell"
)

// Dispatch handles AgentOS front-door subcommands that are not shell scripts —
// `bashy weave …` (the multi-agent workspace orchestrator), `bashy secrets …`
// (cloudbox-managed API keys/tokens for the shell), `bashy dag …` (the
// agent-first markdown DAG task runner), and `bashy podman …` (a transparent
// shell-out to an installed podman). It is wired into the shell
// core via cli.AgentOSDispatch and runs before any bash flag parsing, since the
// subcommands carry their own flags. It os.Exit()s when it handles the
// invocation and returns otherwise.
func Dispatch() {
	if len(os.Args) < 2 {
		return
	}
	switch os.Args[1] {
	case "weave":
		cmd := weave.NewWeaveCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "secrets":
		cmd := secrets.NewSecretsCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "dag":
		// The agent-first DAG task runner: markdown-defined targets run as a
		// dependency graph. dag.ExitCodeOf recovers the stable weavecli exit
		// code from the cobra error so agents get a meaningful status.
		cmd := dag.NewDagCmd()
		cmd.SetArgs(os.Args[2:])
		os.Exit(dag.ExitCodeOf(cmd.Execute()))
	case "podman":
		// Shell-out pass-through to an externally installed podman (Layer 2 of
		// the AgentOS substrate plan): no embedded engine, no fork — the
		// caller's env (CONTAINER_HOST etc.) is inherited verbatim.
		os.Exit(podman.Run(context.Background(), os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	}
}

// WireExec appends the coreutils ExecHandler so any registered tool resolves
// in-process (pure-Go-first) before PATH lookup. It is wired into the shell
// core via cli.AgentOSWireExec. Shell builtins (echo, pwd, test, …) are handled
// by the interpreter before the ExecHandler runs, so they are never shadowed —
// only external-command names (ls, cat, grep, yc, …) are intercepted.
func WireExec(opts []interp.RunnerOption) []interp.RunnerOption {
	return append(opts, interp.ExecHandlers(coreutilsshell.Handler()))
}
