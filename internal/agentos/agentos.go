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
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/bashy/skills"

	_ "github.com/qiangli/coreutils/cmds/all"
	"github.com/qiangli/coreutils/external/act"
	"github.com/qiangli/coreutils/external/clang"
	"github.com/qiangli/coreutils/external/cmake"
	"github.com/qiangli/coreutils/external/gh"
	"github.com/qiangli/coreutils/external/gotoolchain"
	"github.com/qiangli/coreutils/external/kopia"
	"github.com/qiangli/coreutils/external/loom"
	"github.com/qiangli/coreutils/external/rclone"
	"github.com/qiangli/coreutils/external/seaweedfs"
	"github.com/qiangli/coreutils/external/zot"
	"github.com/qiangli/coreutils/pkg/dag"
	"github.com/qiangli/coreutils/pkg/jobs"
	"github.com/qiangli/coreutils/pkg/mirror"
	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/pkg/secrets"
	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/qiangli/coreutils/pkg/weavecli"
	coreutilsshell "github.com/qiangli/coreutils/shell"
)

// Preamble returns shell source defining AgentOS default functions, registered
// before user startup files (so they can be overridden in an rc). It is the
// `docker` → `bashy podman` shim plus bare-name shims for the front-door verbs,
// so `weave …`, `schedule …`, `gh …` work without the `bashy ` prefix. `command`
// bypasses the function so the external bashy binary runs (no recursion).
//
// Shadowing policy (a function shadows a same-named PATH binary):
//   - Native verbs + identical drop-in passthroughs (which provision/exec the
//     real tool, +extras) shadow ALWAYS.
//   - Version-sensitive provisioners (go/cmake/clang) shadow ONLY in agent mode,
//     where auto-provisioning + loud errors help; a human's installed toolchain
//     wins in a regular shell. Reach bashy's explicitly with `bashy go …`.
//   - `time` (a bash keyword) and the job-control builtins (jobs/fg/bg/kill) are
//     never shimmed.
//
// Every shim is overridable: `unset -f <name>` (or redefine it) falls back to
// PATH, and a specific on-disk binary is always reachable by absolute path
// (e.g. /usr/local/bin/gh).
func Preamble() string {
	var b strings.Builder
	b.WriteString(`docker() { command bashy podman "$@"; }` + "\n")
	always := []string{
		"weave", "sprint", "dag", "schedule", "secrets", "skills", "run",
		"gh", "act", "rclone", "podman", "ollama",
		"loom", "zot", "seaweedfs", "kopia", "mirror",
	}
	for _, v := range always {
		fmt.Fprintf(&b, "%s() { command bashy %s \"$@\"; }\n", v, v)
	}
	if weavecli.IsAgent() {
		for _, v := range []string{"go", "cmake", "clang"} {
			fmt.Fprintf(&b, "%s() { command bashy %s \"$@\"; }\n", v, v)
		}
	}
	return b.String()
}

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
	// The container/LLM engines (`bashy podman`, `bashy ollama`) embed cgo +
	// platform-specific backends (podman's btrfs/devmapper drivers, ollama's
	// Apple MLX) and only build on unix hosts — they are split into a
	// platform-tagged dispatchEngine so the rest of AgentOS (shell, git, dag,
	// weave, the binmgr-managed externals) cross-compiles to Windows.
	dispatchEngine(os.Args[1])
	// The observability stack (`bashy otel`) compiles in the OpenTelemetry
	// Collector + VictoriaMetrics/Logs + Jaeger + Perses + k8s/aws SDKs (~193 MB,
	// 60% of the binary). It is a mesh-HOST service, not a worker need, so it is
	// excluded from the default lean build and gated behind dispatchObs: present
	// only under `-tags bashy_obs`; the default stub points the user at a host.
	dispatchObs(os.Args[1])
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
	case "skills":
		// Surface the tier-2 workspace skills embedded in the binary (self-
		// contained — no source tree needed). `bashy skills [list]` lists them;
		// `bashy skills show <name> [--reference]` prints the content.
		os.Exit(dispatchSkills(os.Args[2:]))
	case "schedule":
		// Modern cron: run commands on a cron/interval/at schedule from a
		// self-contained store + optional daemon, with an agentic prompt/context
		// delivered to the fired command. The host cron/crontab are untouched.
		cmd := schedule.NewScheduleCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "run":
		// Wrap a command and emit a structured result envelope (bashy-run-v1)
		// bundling exit/signal/duration/cwd + the advisor's hints. Streams live
		// by default (meta trails on stderr); --capture embeds the streams in one
		// stdout record. Returns the command's own exit status.
		os.Exit(dispatchRun(os.Args[2:]))
	case "go":
		// Self-provisioning Go toolchain (check → download from go.dev →
		// sha256-verify → cache → exec). No embedding, no system Go: this is
		// what lets a bare node `bashy go build/test`. Pure-Go + cross-platform,
		// so it stays in the shared switch (not engine-gated).
		cmd := gotoolchain.NewGoCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "cmake":
		// Self-provisioning CMake (binmgr download -> verify -> cache; no system
		// CMake needed). Pure-Go fetch + cross-platform, same shape as bashy go.
		cmd := cmake.NewCmakeCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "clang":
		// Self-provisioning Clang toolchain: the standalone llvm-mingw on Windows
		// (binmgr), the system clang on macOS/Linux. The compiler half of the
		// self-contained cross-platform build userland (cmake is the other half).
		cmd := clang.NewClangCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
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
	case "act":
		// Run GitHub Actions locally via a binmgr-managed nektos/act (MIT, not
		// compiled in) — test CI on a mesh node before pushing. Needs a container
		// engine (bashy podman, unix host). Transparent passthrough.
		cmd := act.NewActCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "gh":
		// The GitHub CLI (cli/cli, MIT) via binmgr — open PRs, trigger/watch the
		// real github runs, `gh api`. With act+go+git it closes the CI/CD loop.
		cmd := gh.NewGhCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "rclone":
		// Transparent passthrough to a binmgr-managed rclone (MIT) — the transfer
		// engine + a NAS-style file server (`rclone serve …`). Not compiled in.
		cmd := rclone.NewRcloneCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "mirror":
		// Continuous one-way directory mirror (Syncthing's architecture, all
		// permissive parts: rjeczalik/notify MIT recursive watch + rclone MIT
		// transfer; our own orchestration). Node B keeps a live replica of a dir
		// on node A — over the mesh, point --dest at the replica's exposed rclone.
		cmd := mirror.NewMirrorCmd()
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

// dispatchSkills implements `bashy skills`: list embedded skills, or print one.
//
//	bashy skills [list]
//	bashy skills show <name> [--reference]
func dispatchSkills(args []string) int {
	if len(args) == 0 || args[0] == "list" {
		for _, n := range skills.Names() {
			fmt.Println(n)
		}
		return 0
	}
	if args[0] == "show" && len(args) >= 2 {
		name := args[1]
		ref := len(args) > 2 && (args[2] == "--reference" || args[2] == "-r")
		var body string
		var ok bool
		if ref {
			body, ok = skills.Reference(name)
		} else {
			body, ok = skills.Body(name)
		}
		if !ok {
			fmt.Fprintf(os.Stderr, "bashy skills: %q not found\n", name)
			return 1
		}
		fmt.Print(body)
		return 0
	}
	fmt.Fprintln(os.Stderr, "usage: bashy skills [list | show <name> [--reference]]")
	return 2
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

	// The nudge subsystem (non-intrusive). Two halves sharing one session memory:
	//   - advisor (reactive): OUTERMOST ExecHandler middleware; on a command's
	//     non-zero exit, appends one advisory line explaining a space-determined
	//     failure (wrong cwd, host gone remote, OOM, full/ro disk) so an agent
	//     stops the doomed retry loop. Always returns the exit unchanged.
	//   - nudger (proactive): a WithAuditHandler callback; when an agent uses a
	//     legacy builtin (cd/pushd/popd) it emits one rate-limited hint toward the
	//     better counterpart (`awd`). Never alters the command.
	// Both are stderr-only, gated (agent mode / BASHY_ADVISOR / BASHY_HINTS, with
	// BASHY_AGENTIC as master kill), and never active in posix mode / cmd/bash.
	if advisorEnabled() || hintsEnabled() {
		a := newAdvisor()
		if hintsEnabled() {
			opts = append(opts, interp.WithAuditHandler(newNudger(a.mem).onAudit))
		}
		if advisorEnabled() {
			return append(opts, interp.ExecHandlers(
				advisorHandler(a), dryRunHandler(r), coreutilsshell.Handler()))
		}
	}
	return append(opts, interp.ExecHandlers(dryRunHandler(r), coreutilsshell.Handler()))
}
