// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package main

import (
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/interp"

	_ "github.com/qiangli/coreutils/cmds/all"
	coreutilsshell "github.com/qiangli/coreutils/shell"
	"github.com/qiangli/coreutils/pkg/weave"
)

// This file is what makes one build behave as two binaries:
//
//   - bash  — a faithful GNU Bash 5.3 drop-in. Nothing here fires; external
//     commands resolve through PATH exactly as bash does.
//   - bashy — the AgentOS shell. The coreutils ExecHandler is injected so the
//     pure-Go userland (cat, ls, grep, …) and the `yc` code-intel verbs run
//     in-process, uniformly across Linux/macOS/Windows.
//
// The two are the same compiled binary, distinguished by argv[0] (busybox
// style — the compliance harness already runs a copy named `bin/bash`). Only
// `bashy` opts into the AgentOS userland, so the bash drop-in and its
// conformance suite stay untouched.

// isAgentOSShell reports whether this invocation is the AgentOS `bashy`
// shell rather than the pure `bash` drop-in. A non-empty BASHY_AGENTOS env
// var forces it on regardless of the invoked name (BASHY_AGENTOS=0/false
// forces it off).
func isAgentOSShell() bool {
	if v, ok := os.LookupEnv("BASHY_AGENTOS"); ok {
		return v != "" && v != "0" && v != "false"
	}
	base := filepath.Base(strings.TrimPrefix(os.Args[0], "-"))
	base = strings.TrimSuffix(base, ".exe")
	return base == "bashy"
}

// maybeRunAgentOSSubcommand dispatches AgentOS front-door subcommands that
// are not shell scripts — currently `bashy weave …`, the re-homed
// multi-agent workspace orchestrator. Only the AgentOS `bashy` binary offers
// them; the pure `bash` drop-in falls through and treats the argument as a
// script path, exactly like bash. Call it at the very top of main(), before
// flag parsing, since the subcommand has its own flags. It os.Exit()s when
// it handles the invocation and returns otherwise.
func maybeRunAgentOSSubcommand() {
	if !isAgentOSShell() || len(os.Args) < 2 {
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
	}
}

// wireAgentOS appends the coreutils ExecHandler so any registered tool
// resolves in-process (pure-Go-first) before PATH lookup. It is a no-op for
// the pure bash binary. Shell builtins (echo, pwd, test, …) are handled by
// the interpreter before the ExecHandler runs, so they are never shadowed —
// only external-command names (ls, cat, grep, yc, …) are intercepted.
func wireAgentOS(opts []interp.RunnerOption) []interp.RunnerOption {
	if !isAgentOSShell() {
		return opts
	}
	return append(opts, interp.ExecHandlers(coreutilsshell.Handler()))
}
