// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package agentos holds the AgentOS wiring that turns the shell core into the
// `bashy` system shell: the coreutils ExecHandler (so the pure-Go userland and
// `yc` code-intel verbs run in-process) and the front-door subcommands
// (`bashy weave …`, `bashy otel …`, `bashy podman …`).
//
// It is imported ONLY by cmd/bashy — never by cmd/bash. That is what keeps the
// two binaries independent: the pure `bash` drop-in's import graph never pulls
// in coreutils or any external-tool surface, so it stays a lean GNU Bash 5.3
// drop-in, while `bashy` is the self-contained bootstrapper for a whole
// unix-like userland (bash + coreutils + pkg + external tools).
package agentos

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/interp"

	_ "github.com/qiangli/coreutils/cmds/all"
	"github.com/qiangli/coreutils/external/kopia"
	"github.com/qiangli/coreutils/external/loom"
	"github.com/qiangli/coreutils/external/ollama"
	"github.com/qiangli/coreutils/external/otel/otelcli"
	"github.com/qiangli/coreutils/external/podman"
	"github.com/qiangli/coreutils/external/seaweedfs"
	"github.com/qiangli/coreutils/external/zot"
	"github.com/qiangli/coreutils/pkg/dag"
	"github.com/qiangli/coreutils/pkg/jobs"
	"github.com/qiangli/coreutils/pkg/secrets"
	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/qiangli/coreutils/pkg/weavecli"
	coreutilsshell "github.com/qiangli/coreutils/shell"
)

// Dispatch handles AgentOS front-door subcommands that are not shell scripts —
// `bashy weave …` (the multi-agent workspace orchestrator), `bashy otel …`
// (the all-in-one observability stack), `bashy secrets …`
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
	case "sprint":
		// Plan/handoff layer (cross-repo), peer to `weave` (per-repo
		// execution). Shares the AgentOS state root; user-global board.
		cmd := weave.NewSprintCmd()
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
	case "otel":
		cmd := otelcli.NewCommand()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "podman":
		// Shell-out pass-through to an externally installed podman (Layer 2 of
		// the AgentOS substrate plan): no embedded engine, no fork — the
		// caller's env (CONTAINER_HOST etc.) is inherited verbatim.
		os.Exit(podman.Run(context.Background(), os.Args[2:], os.Stdin, os.Stdout, os.Stderr))
	case "loom":
		// The mesh git forge: run Gitea as a managed external binary (binmgr
		// downloads/verifies/caches it; not compiled in). bashy is the "OS of
		// binaries" host; outpost exposes it over the mesh.
		cmd := loom.NewLoomCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "zot":
		// The mesh OCI registry (images + Ollama models): run Zot as a managed
		// external binary (binmgr — not compiled in). Same wrap pattern as loom.
		cmd := zot.NewZotCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "seaweedfs":
		// The mesh object/blob store (S3 gateway): run SeaweedFS as a managed
		// external binary (binmgr — not compiled in). Same wrap pattern as loom.
		cmd := seaweedfs.NewSeaweedfsCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "kopia":
		// The mesh snapshot-backup repository server: run Kopia as a managed
		// external binary (binmgr — not compiled in). Same wrap pattern as loom.
		cmd := kopia.NewKopiaCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "ollama":
		cmd := ollama.NewOllamaCmd(ollama.CmdOptions{
			RunEmbeddedServe: func(ctx context.Context) error {
				home, err := os.UserHomeDir()
				if err != nil {
					return err
				}
				dataDir := filepath.Join(home, ".agents", "bashy", "observability", "inference")
				comp := ollama.NewOllamaComponent(nil, dataDir)
				if err := comp.Start(ctx); err != nil {
					return err
				}

				baseURL := comp.BaseURL()
				if baseURL == "" {
					baseURL = ollama.DefaultURL()
				}
				fmt.Printf("Ollama server listening on %s\n", baseURL)
				fmt.Println("Press Ctrl-C to stop.")

				sigCh := make(chan os.Signal, 1)
				signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
				defer signal.Stop(sigCh)

				select {
				case <-sigCh:
					fmt.Println("\nShutting down...")
				case <-ctx.Done():
				}

				stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer stopCancel()
				return comp.Stop(stopCtx)
			},
		})
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "jobs", "fg", "bg", "kill":
		// Real-PID job control over detached background jobs (`foo &`). The
		// in-shell fg/bg/jobs builtins can't own the controlling terminal
		// (subshells are goroutines), so the supported path is these
		// subcommands operating on the shared coreutils/pkg/jobs registry —
		// the same model outpost ships. WireExec records each `foo &` PID via
		// WithBgPidCallback below.
		var cmd *cobra.Command
		switch os.Args[1] {
		case "jobs":
			cmd = jobs.JobsCommand()
		case "fg":
			cmd = jobs.FgCommand()
		case "bg":
			cmd = jobs.BgCommand()
		case "kill":
			cmd = jobs.KillCommand()
		}
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
}

// WireExec appends the coreutils ExecHandler so any registered tool resolves
// in-process (pure-Go-first) before PATH lookup. It is wired into the shell
// core via cli.AgentOSWireExec. Shell builtins (echo, pwd, test, …) are handled
// by the interpreter before the ExecHandler runs, so they are never shadowed —
// only external-command names (ls, cat, grep, yc, …) are intercepted.
func WireExec(opts []interp.RunnerOption, posix bool) []interp.RunnerOption {
	// --dry-run (bashy-only, inert under --posix). The handlers are installed
	// whenever NOT in posix mode (they no-op when dry-run is off) so the runtime
	// `set -o dryrun` toggle works even without the flag. EnableDryRunOption
	// makes the engine recognize `set -o dryrun`; the pure bash drop-in never
	// passes it, so it rejects the option exactly like Bash.
	//
	// Record each detached `foo &` real OS PID in the shared coreutils/pkg/jobs
	// registry so `bashy jobs/fg/bg/kill` (Dispatch above) can manage it — the
	// same real-PID model outpost uses. Harmless in posix mode (recording only).
	opts = append(opts, interp.WithBgPidCallback(func(pid int) {
		_ = jobs.DefaultRegistry().Record(pid, "(detached)")
	}))
	if posix {
		return append(opts, interp.ExecHandlers(coreutilsshell.Handler()))
	}
	initial := dryRunRequested()
	if initial && weavecli.IsAgent() {
		// Agent mode emits a clean JSON manifest on stdout; suppress the
		// script's own stdout so only the manifest comes through.
		opts = append(opts, interp.StdIO(os.Stdin, io.Discard, os.Stderr))
	}
	opts = append(opts, interp.EnableDryRunOption(initial))
	r := newReporter(os.Stdout)
	// OpenHandler catches `>` truncations (records, never writes); the exec
	// handler prints+skips external commands and reports rm destructions. Both
	// no-op when HandlerContext.DryRun() is false.
	opts = append(opts, interp.OpenHandler(dryRunOpenHandler(r)))
	return append(opts, interp.ExecHandlers(dryRunHandler(r), coreutilsshell.Handler()))
}
