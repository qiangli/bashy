// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package agentos holds the AgentOS wiring that turns the shell core into the
// `bashy` system shell: the coreutils ExecHandler (so the pure-Go userland and
// the code-intel verbs run in-process) and the front-door subcommands
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
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/bashy/internal/agentos/session"
	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/bashy/skills"

	_ "github.com/qiangli/coreutils/cmds/all"
	// Code-knowledge-graph verb (graph build/impact/query/… as subcommands of one
	// `graph` tool). Registered here — NOT via cmds/all — so gfy's document-parsing
	// deps land in `bashy` only, never the bare cmd/coreutils multicall binary or
	// the cmd/bash drop-in. It reaches the front door + in-shell ExecHandler through
	// the tool registry (agentos.go dispatch fallthrough), like ast/graph.
	_ "github.com/qiangli/coreutils/cmds/graph"
	// Foreman — the steerable agent session (start/tell/status/pause/…, the
	// `chat` parent elevated to a persistent session). Registered here — NOT via
	// cmds/all — because it imports pkg/foreman → pkg/dag, which would form an
	// import cycle with pkg/dag's tests if listed in cmds/all. It is an AgentOS
	// front-door verb like weave/dag/chat; reachable as `bashy foreman` through
	// the tool-registry dispatch fallthrough.
	_ "github.com/qiangli/coreutils/cmds/foreman"
	"github.com/qiangli/coreutils/external/act"
	"github.com/qiangli/coreutils/external/actrunner"
	"github.com/qiangli/coreutils/external/clang"
	"github.com/qiangli/coreutils/external/cmake"
	"github.com/qiangli/coreutils/external/curlbin"
	"github.com/qiangli/coreutils/external/gh"
	"github.com/qiangli/coreutils/external/gitscm"
	"github.com/qiangli/coreutils/external/gotoolchain"
	"github.com/qiangli/coreutils/external/helm"
	"github.com/qiangli/coreutils/external/kopia"
	"github.com/qiangli/coreutils/external/kubectl"
	"github.com/qiangli/coreutils/external/loom"
	"github.com/qiangli/coreutils/external/mise"
	"github.com/qiangli/coreutils/external/node"
	"github.com/qiangli/coreutils/external/python"
	"github.com/qiangli/coreutils/external/rclone"
	"github.com/qiangli/coreutils/external/registry"
	"github.com/qiangli/coreutils/external/rust"
	"github.com/qiangli/coreutils/external/seaweedfs"
	"github.com/qiangli/coreutils/external/sphere"
	"github.com/qiangli/coreutils/external/tessaro"
	"github.com/qiangli/coreutils/external/zot"
	"github.com/qiangli/coreutils/pkg/agentcmd"
	"github.com/qiangli/coreutils/pkg/capability"
	"github.com/qiangli/coreutils/pkg/chat"
	"github.com/qiangli/coreutils/pkg/dag"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/gate"
	"github.com/qiangli/coreutils/pkg/handoff"
	"github.com/qiangli/coreutils/pkg/issue"
	"github.com/qiangli/coreutils/pkg/jobs"
	"github.com/qiangli/coreutils/pkg/telemetry"
	"github.com/qiangli/coreutils/pkg/judge"
	"github.com/qiangli/coreutils/pkg/kb"
	"github.com/qiangli/coreutils/pkg/lexicon"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/mirror"
	"github.com/qiangli/coreutils/pkg/policy/coord"
	"github.com/qiangli/coreutils/pkg/principal"
	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/pkg/sdlc"
	"github.com/qiangli/coreutils/pkg/secrets"
	coreskills "github.com/qiangli/coreutils/pkg/skills"
	"github.com/qiangli/coreutils/pkg/supervise"
	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/coreutils/pkg/webinspect"
	coreutilsshell "github.com/qiangli/coreutils/shell"
	"github.com/qiangli/coreutils/tool"
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
// alwaysShimVerbs are the front-door verbs exposed as bare-name shell functions
// unconditionally: bashy-native verbs + identical drop-in passthroughs.
// agentModeShimVerbs are version-sensitive provisioners, shimmed only in agent
// mode (a human's own go/cmake/clang on PATH wins otherwise). `commands` (the
// surface lister) is itself shimmed so it is reachable bare.
var (
	alwaysShimVerbs = []string{
		"weave", "sprint", "issue", "handoff", "resume", "claim", "invoke", "meet", "capability", "foreman", "agent", "sdlc", "web", "dag", "schedule", "secrets", "skills", "kb", "lexicon", "tools", "models", "agents", "people", "whois", "run", "commands", "context", "doctor", "otel", "audit", "self", "check", "gate", "judge", "conform",
		"git", "gh", "act", "act-runner", "rclone", "podman", "ollama",
		"loom", "zot", "seaweedfs", "kopia", "mirror",
		"kubectl", "helm", "sphere", "tessaro", "login",
	}
	agentModeShimVerbs   = []string{"go", "cmake", "clang", "node", "npm", "npx", "pnpm", "yarn", "python", "pip", "uv", "mise", "cargo", "rustc", "rustup", "rust", "git-scm", "curl"}
	hiddenFrontDoorVerbs = []string{"bootstrap", "upgrade", "chat", "verify"}
)

func Preamble() string {
	var b strings.Builder
	self := bashySelfPath()
	fmt.Fprintf(&b, "docker() { command %s podman \"$@\"; }\n", shellQuote(self))
	fmt.Fprintf(&b, "sh() { command %s --posix \"$@\"; }\n", shellQuote(self))
	for _, v := range alwaysShimVerbs {
		fmt.Fprintf(&b, "%s() { command %s %s \"$@\"; }\n", v, shellQuote(self), v)
	}
	// IsAgent (== BASHY_AGENTIC), deliberately NOT IsAgentDriven. `bashy go` does not
	// wrap the host toolchain — it DOWNLOADS AND PINS ITS OWN. Shimming `go` is right
	// when bashy orchestrated the run and owns the environment (a weave worker must
	// build against a pinned toolchain). It is WRONG in a human's Claude session, where
	// it would shadow the Go the developer actually installed and quietly fetch a
	// different version to build their project with.
	//
	// A machine at the wheel earns better HINTS. Only bashy orchestrating the run earns
	// a different WORLD.
	if weavecli.IsAgent() {
		for _, v := range agentModeShimVerbs {
			fmt.Fprintf(&b, "%s() { command %s %s \"$@\"; }\n", v, shellQuote(self), v)
		}
	}
	// Declarative managed-external registry (tier-5/6 client CLIs: doctl, …) —
	// bare-name shims too, so `doctl …` resolves to `bashy doctl …`.
	for _, v := range registry.Names() {
		fmt.Fprintf(&b, "%s() { command %s %s \"$@\"; }\n", v, shellQuote(self), v)
	}
	return b.String()
}

// The shell→agent capability manifest (a structured INSIDE_EMACS; an
// unoccupied niche per the 2026-07 survey): every child of any bashy
// process — a shell session's commands, an agent CLI the user launches
// from a bashy login shell, a weave/foreman-launched worker — inherits
// one env var saying what this shell can do and what to call first.
// Set in init (agentos links only into cmd/bashy; the lean cmd/bash
// drop-in never carries it). Static string: zero startup cost.
func init() {
	os.Setenv("BASHY_AGENT_MANIFEST",
		`v1 shell=agentic first-hop="bashy context --json" skills="bashy skills list" guide="bashy skills show bashy|bashy bashy"`)
}

// maybeAdvertiseSkillHint is L1 of the advertisement ladder: when an
// agentic tool is driving (env markers), the repo has no agent config
// at all, and we have not hinted here before — one stderr line pointing
// at the bashy skill. Zero writes to the repo; the once-per-repo marker
// lives in the skills store.
func maybeAdvertiseSkillHint() {
	agent, ok := coreskills.DetectAgent()
	if !ok {
		return
	}
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	root := cwd
	for d := cwd; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			root = d
			break
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	store := os.Getenv("BASHY_SKILLS_DIR")
	if store == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return
		}
		store = filepath.Join(home, ".config", "bashy", "skills")
	}
	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(root)))[:16]
	mark := filepath.Join(store, "hints", sum)
	if _, err := os.Stat(mark); err == nil {
		return // this repo was evaluated before (hinted, or already configured)
	}
	configured := false
	for _, marker := range []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md", ".claude", ".agents", ".cursor", ".goosehints", filepath.Join(".github", "copilot-instructions.md")} {
		if _, err := os.Stat(filepath.Join(root, marker)); err == nil {
			configured = true // the repo already speaks agent — no hint
			break
		}
	}
	if !configured {
		fmt.Fprintf(os.Stderr, "bashy: %s detected, and this repo has no agent config — bashy is an agentic shell with a built-in guide: `bashy skills show bashy` (install for your agent: `bashy skills export bashy --user`; this hint shows once per repo)\n", agent)
	}
	if err := os.MkdirAll(filepath.Dir(mark), 0o755); err == nil {
		_ = os.WriteFile(mark, []byte(root+"\n"), 0o644)
	}
}

func bashySelfPath() string {
	if exe, err := os.Executable(); err == nil && exe != "" {
		return exe
	}
	if len(os.Args) > 0 && os.Args[0] != "" {
		return os.Args[0]
	}
	return "bashy"
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// Dispatch handles AgentOS front-door subcommands that are not shell scripts —
// `bashy weave …` (the multi-agent workspace orchestrator), `bashy otel …`
// (the all-in-one observability stack), `bashy secrets …`
// (managed API keys/tokens for the shell), `bashy dag …` (the
// agent-first markdown DAG task runner), and `bashy podman …` (a transparent
// shell-out to an installed podman). It is wired into the shell
// core via cli.AgentOSDispatch and runs before any bash flag parsing, since the
// subcommands carry their own flags. It os.Exit()s when it handles the
// invocation and returns otherwise.
func Dispatch() {
	if len(os.Args) < 2 {
		return
	}
	// L1 of the skills advertisement ladder: agent driving + agent-naive
	// repo + not hinted here before → one stderr pointer. Zero repo writes.
	maybeAdvertiseSkillHint()
	// L2.5 (orchestrator channel): freshly created weave workspaces are
	// bashy-owned space-time — stock each with the agent skill surface
	// before any agent brand launches. $BASHY_WEAVE_SKILLS extends the
	// set (comma-separated) or disables it (0/none/off).
	weave.ProvisionWorkspace = func(workspace string, stderr io.Writer) {
		names := []string{"bashy"}
		switch v := os.Getenv("BASHY_WEAVE_SKILLS"); v {
		case "0", "none", "off":
			return
		case "":
		default:
			for _, n := range strings.Split(v, ",") {
				if n = strings.TrimSpace(n); n != "" && n != "bashy" {
					names = append(names, n)
				}
			}
		}
		coreskills.Provision(workspace, names, stderr, skillsOptions()...)
	}
	// Warm-session hot path: when $BASHY_SESSION points at a live `bashy serve`
	// listener and this is a simple `bashy -c "…"` invocation, forward it to the
	// warm process (skips the per-call process/package init). A dead or absent
	// session falls through to normal in-process execution — never stranded.
	if exit, handled := session.Route(); handled {
		os.Exit(exit)
	}
	// The container/LLM engines (`bashy podman`, `bashy ollama`) embed cgo +
	// platform-specific backends (podman's btrfs/devmapper drivers, ollama's
	// Apple MLX) and only build on unix hosts — they are split into a
	// platform-tagged dispatchEngine so the rest of AgentOS (shell, git, dag,
	// weave, the binmgr-managed externals) cross-compiles to Windows.
	dispatchEngine(os.Args[1])
	// The observability stack (`bashy otel`) compiles in the OpenTelemetry
	// Collector + VictoriaMetrics/Logs + Jaeger + Perses + k8s/aws SDKs (~193 MB,
	// 60% of the binary). It is a host-only service, not a worker need, so it is
	// excluded from the default lean build and gated behind dispatchObs: present
	// only under `-tags bashy_obs`; the default stub points the user at a host.
	dispatchObs(os.Args[1])
	switch os.Args[1] {
	case "help":
		os.Exit(dispatchHelp(os.Args[2:]))
	case "serve":
		// Warm session: one already-initialized process serves many
		// `bashy -c "…"` calls. Optional socket path arg overrides the default.
		socket := ""
		if len(os.Args) > 2 {
			socket = os.Args[2]
		}
		if err := session.Serve(socket); err != nil {
			fmt.Fprintln(os.Stderr, "bashy serve:", err)
			os.Exit(1)
		}
		os.Exit(0)
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
	case "handoff", "resume":
		// Portable session handoff. Every agentic tool has a /resume, and every one
		// of them is a prison: it reads that tool's private transcript store, on
		// that machine, in that tool. bashy is the SHELL underneath all of them, so
		// it is the one layer that can write a session record which OUTLIVES the
		// tool that made it — and the record is an ARTIFACT (a self-contained diff +
		// untracked files carried by content + the brief), never a pointer, so it
		// travels: scp, mesh, an issue comment.
		//
		// The piece nothing else had is the IN-FLIGHT WORKING TREE. sprint handoff,
		// weave baton and the cloudbox session lease all record PROSE, so a
		// successor inherited a narrative, not a diff.
		var hcmd *cobra.Command
		if os.Args[1] == "handoff" {
			hcmd = handoff.NewHandoffCmd()
		} else {
			hcmd = handoff.NewResumeCmd()
		}
		hcmd.SetArgs(os.Args[2:])
		if err := hcmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy "+os.Args[1]+":", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "claim":
		cmd := coord.NewClaimCmd(func() []string {
			cwd, err := os.Getwd()
			if err != nil {
				return nil
			}
			return handoff.ProjectRoots(projectRootOf(cwd))
		})
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy claim:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "lexicon":
		// The project's jargon, PROJECTED from the registries that already define it.
		// A user says "handoff this to codex": neither word means what the dictionary
		// says -- handoff is a bashy verb, and codex is an agent binding ON THIS HOST
		// (a CLI tool plus a bound model), which denotes something else on another
		// machine.
		//
		// It stores NOTHING. Verbs come from the Command Atlas, bindings from the
		// fleet registry. Only two things are hand-written, because a machine cannot
		// infer them: what the team actually SAYS, and the precedence rule. The
		// moment this starts storing vocabulary rather than projecting it, it has
		// become the hand-written glossary that always goes stale.
		lexicon.Synopses = verbSynopsis
		lcmd := lexicon.NewLexiconCmd()
		lcmd.SetArgs(os.Args[2:])
		if err := lcmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy lexicon:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "issue":
		// THE Plan-stage intake verb: the project's issue register -- GitHub Issues
		// without GitHub.
		//
		// bashy could track work an agent was ACTIVELY DOING (weave: a per-machine
		// queue with a workspace, a branch and a verify command) and what a conductor
		// was planning RIGHT NOW (sprint). It could not record a bug nobody has
		// triaged, a requirement nobody has scheduled, or a feature somebody merely
		// asked for -- so those lived as bullets in docs/TODO.md: invisible to every
		// verb, unqueryable, impossible to close.
		//
		// `sdlc issue` LOOKED like this and wasn't: SaveLocalIssue writes {timestamp,
		// title, body} into .bashy/GENERATED/ and has no counterpart anywhere in the
		// tree -- no List, no Load, no Close. A drop box, not a register.
		//
		// The store is COMMITTED (.bashy/issues/, source not scratch), because a
		// requirement must travel with the clone, show up in a diff, survive the
		// machine it was typed on, and need no forge to exist.
		icmd := issue.NewIssueCmd(func() (string, error) {
			cwd, err := os.Getwd()
			if err != nil {
				return "", err
			}
			if r := detectProjectRoot(cwd); r != "" {
				return r, nil
			}
			return cwd, nil
		})
		icmd.SetArgs(os.Args[2:])
		if err := icmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy issue:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "judge":
		// gate's SEMANTIC twin. gate asks "does it PASS" -- mechanical, reproducible,
		// safe to block a merge on. judge asks "is it GOOD" -- an LLM opinion, and so
		// advisory unless the caller says --gate. Together they are what the conductor
		// playbook keeps saying in prose: SANDBOX-GREEN IS NOT MERGEABLE.
		//
		// bashy could VERIFY but not JUDGE. `weave review` sounds like this and isn't
		// (it re-runs the verify command in a clean-room clone, never launching an
		// agent). The role existed as ad-hoc prompting -- docs/JUDGE-REPORT-R6.md and
		// friends are its artifacts. This is the verb behind them.
		jcmd := judge.NewJudgeCmd()
		jcmd.SetArgs(os.Args[2:])
		if err := jcmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy judge:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "gate":
		// THE Test verb. Before it, the Test stage was EMPTY -- not because nobody
		// tested, but because the gate (the command that decides pass/fail) was
		// spelled four incompatible ways across four packages: weave's suite-gate
		// file, sdlc's healthcheck: key, supervise's :: string, and a dag target
		// that happens to fail. All four mean the same thing -- run a command, let
		// its exit status be the verdict. They never disagreed about semantics, only
		// about where the command lives. This is the one place it lives.
		gcmd := gate.NewGateCmd()
		gcmd.SetArgs(os.Args[2:])
		if err := gcmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy gate:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "invoke", "chat":
		// Invoke ONE agent, ONCE, on one instruction — the primitive that unifies
		// the heterogeneous agent CLIs (resolve the tool, inject identity, force
		// bashy as its shell). Every orchestrator is built on it: sdlc, foreman and
		// meet all call it; only weave bypasses it (it drives a PTY).
		//
		// Renamed from `chat` 2026-07-12, because chat does not chat. Its own
		// synopsis always said "invoke an agent with a single unattended
		// instruction" — no conversation, no back-and-forth, no session. The name
		// misled an agent into thinking it was a session, which is what `foreman`
		// actually is. `chat` remains a hidden alias; nothing breaks.
		cmd := chat.NewChatCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "meet":
		// Multi-participant deliberation session: agentic CLIs + a human take
		// turns; a dedicated notes-only secretary keeps and files the minutes.
		cmd := meet.NewMeetCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy meet:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "supervise":
		// Conductor-as-a-verb: one supervisor agent drives a fleet of workers
		// against a goal decomposed into GATED tasks, in the current working
		// tree (the in-place counterpart to `bashy weave`'s isolated
		// workspaces). Each task's gate is a shell command the orchestrator runs
		// itself — the verdict is objective, not the agent's claim. See
		// dhnt/docs/agentic-design-pattern-gaps.md.
		cmd := supervise.NewSuperviseCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy supervise:", err)
			os.Exit(1)
		}
		os.Exit(0)
	// `fanout` was removed 2026-07-12 by the Command Atlas SDLC ratchet, which
	// asks every front-door verb: which stage do you serve that nothing else
	// already does? fanout had no answer. It shipped with zero callers, zero
	// skills, zero docs telling anyone to use it, and no atlas entry at all —
	// and its own design doc conceded the collapse: "at which point this *is*
	// weave with a shared seed, and the runner should delegate to weave."
	// N agents deliberating over one shared context is `bashy meet`; N agents
	// working in parallel is `bashy weave`. There was no third thing.
	case "agent":
		cmd := agentcmd.NewAgentCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "capability":
		// The living agent (tool:model) × capability matrix behind
		// capability-routed delegation — the routing table for `chat --capability`.
		cmd := capability.NewCapabilityCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy capability:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "sdlc":
		// Workflow control plane: intake/deployment/approval boundary that
		// delegates implementation planning and sprint execution to agents.
		cmd := sdlc.NewSDLCCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "web":
		cmd := webinspect.NewWebCmd()
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
		// The env-gated skills catalog (coreutils/pkg/skills): `list` shows
		// only skills applicable at this host's space-time coordinate,
		// `probe` prints the coordinate, `show` prints a skill (stdout
		// byte-identical; verdict on stderr), `add`/`verify`/`learn` run
		// the admission gates, `run` executes + attests, `promote`
		// renders the review bundle. Sources: see skillsOptions.
		cmd := coreskills.NewSkillsCmd(skillsOptions()...)
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy skills:", err)
			os.Exit(coreskills.ExitCode(err))
		}
		os.Exit(0)
	case "kb":
		// The host-shared knowledge base (coreutils/pkg/kb): the collective
		// memory of all agents on this host across all repos — OKF-style
		// wiki pages under ~/.bashy/kb. The loop: `search` before a task,
		// `add` when nothing relevant exists, `retro` (update/supersede/
		// validate) after the task completes.
		cmd := kb.NewKBCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy kb:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "tools", "models", "agents", "people", "whois":
		// The fleet registry (coreutils/pkg/fleet) and the principal
		// resolver over it (coreutils/pkg/principal). A `tool` is an
		// agentic CLI harness, a `model` an inference backend, an `agent`
		// a named tool:model binding, a `person` a human. Rings merge
		// embedded baseline → shared dirs → org overlay → local store, so
		// an operator's own entry always wins. `whois` resolves any name
		// across all of them — plus hosts — and says how to reach it.
		runFleet(os.Args[1], os.Args[2:])
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
	case "doctor":
		// Environment self-diagnostic: PATH/sh shadowing, a stale bashy on PATH,
		// toolchain + container engine, agent mode, bin cache. Advisory.
		os.Exit(dispatchDoctor(os.Args[2:]))
	case "audit":
		// The compliance audit trail: tail recent records, verify the hash chain
		// (tamper-evidence), or export an evidence bundle. Reads the log written
		// by the audit ExecHandler middleware (opt-in via BASHY_AUDIT).
		os.Exit(dispatchAudit(os.Args[2:]))
	case "install-agent":
		// Wire a coding agent (claude/opencode/aider/gemini/copilot) to use
		// bashy as its shell; --check verifies, --uninstall reverses. See
		// docs/agent-adoption/matrix.md for per-agent verification status.
		os.Exit(dispatchInstallAgent(os.Args[2:]))
	case "context":
		// First-hop agent context: one compact JSON record with the exact bashy
		// path, mode flags, cwd, and recommended discovery/safety commands.
		os.Exit(dispatchContext(os.Args[2:]))
	case "check":
		// Static script preflight: syntax, recursive command inventory, and
		// bashy/system/container/not-found resolution.
		os.Exit(dispatchCheck(os.Args[2:]))
	case "conform", "verify":
		// BASHY'S OWN fidelity batteries: compat (GNU Bash 5.3) / conformance (yash
		// POSIX) / compliance (Open Group VSC-PCTS, stub) / benchmark. Runs from a
		// bashy source checkout.
		//
		// Renamed from `verify` 2026-07-12. It had claimed the most general word in
		// the vocabulary for the narrowest possible thing: verifying BASHY ITSELF. A
		// project that ADOPTS bashy would never run these against its own code, yet
		// `bashy verify` is exactly what such a project would reach for to ask "does
		// my code pass?" — and get bash's conformance suites instead. `verify`
		// remains a hidden alias; the general pass/fail question is `bashy gate`.
		cmd := verifyCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "self":
		// Self-management: fetch/cache release binaries and explicitly install a
		// selected candidate. This is the bashy-side migration of outpost's
		// direct release-bootstrap lane, without touching outpost itself.
		cmd := selfCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "bootstrap", "upgrade":
		// Hidden transitional aliases. They keep old muscle memory/scripts
		// functional while `bashy self ...` becomes the documented surface.
		cmd := selfCmd()
		cmd.Use = os.Args[1]
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "git", "git-scm":
		// `bashy git` is the REAL, full git — git-for-windows MinGit on Windows,
		// system git on unix — provisioned + checksum-verified. It gives one
		// consistent, complete git across platforms (the pure-Go coreutils client
		// was a subset: no `version`, no full checkout flow). The pure-Go light
		// client lives on as `outpost git`, for BOOTSTRAPPING a bare node that has
		// outpost but no real git yet. `git-scm` is an explicit synonym.
		cmd := gitscm.NewGitSCMCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "commands":
		// Discovery: list the whole supported command surface — shell builtins,
		// the in-process coreutils userland, and the bare-name front-door verbs —
		// which are otherwise invisible to compgen/type (the handler intercepts
		// them before PATH). --json for a structured catalog.
		os.Exit(dispatchCommands(os.Args[2:]))
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
	case "node", "npm", "npx", "pnpm", "yarn":
		// Self-provisioning Node.js ecosystem (binmgr download from nodejs.org →
		// verify via SHASUMS256 → cache → exec; pnpm/yarn via the bundled corepack).
		// Pure-Go fetch + cross-platform, same shape as bashy go — no system Node.
		var cmd *cobra.Command
		switch os.Args[1] {
		case "node":
			cmd = node.NewNodeCmd()
		case "npm":
			cmd = node.NewNpmCmd()
		case "npx":
			cmd = node.NewNpxCmd()
		case "pnpm":
			cmd = node.NewPnpmCmd()
		case "yarn":
			cmd = node.NewYarnCmd()
		}
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "python", "pip", "uv":
		// Self-provisioning Python ecosystem via astral-sh/uv (one verified binary
		// that provisions CPython): python -> `uv run python`, pip -> `uv pip`.
		// Download → sha256-verify → cache → exec; no system Python.
		var cmd *cobra.Command
		switch os.Args[1] {
		case "python":
			cmd = python.NewPythonCmd()
		case "pip":
			cmd = python.NewPipCmd()
		case "uv":
			cmd = python.NewUvCmd()
		}
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "mise":
		// Polyglot runtime/version manager (jdx/mise) — managed external binary,
		// checksum-verified by binmgr. The power-user layer over the native
		// provisioners (.tool-versions / mise.toml, the long tail of languages).
		cmd := mise.NewMiseCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "cargo", "rustc", "rustup", "rust":
		// Self-provisioning Rust via the official rustup-init (sha256 sidecar
		// verified), into a bashy-owned CARGO_HOME/RUSTUP_HOME. No system Rust.
		var cmd *cobra.Command
		switch os.Args[1] {
		case "cargo":
			cmd = rust.NewCargoCmd()
		case "rustc":
			cmd = rust.NewRustcCmd()
		case "rustup":
			cmd = rust.NewRustupCmd()
		case "rust":
			cmd = rust.NewRustCmd()
		}
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "curl":
		// Platform curl (built into Windows 10+, universal on unix); a pinned,
		// checksum-verified curl.se/windows build on a bare Windows node.
		cmd := curlbin.NewCurlCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "loom":
		// Git forge: run Gitea as a managed external binary (binmgr
		// downloads/verifies/caches it; not compiled in). bashy is the "OS of
		// binaries" host.
		cmd := loom.NewLoomCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "zot":
		// OCI registry (images + Ollama models): run Zot as a managed
		// external binary (binmgr — not compiled in). Same wrap pattern as loom.
		cmd := zot.NewZotCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "seaweedfs":
		// Object/blob store (S3 gateway): run SeaweedFS as a managed
		// external binary (binmgr — not compiled in). Same wrap pattern as loom.
		cmd := seaweedfs.NewSeaweedfsCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "kopia":
		// Snapshot-backup repository server: run Kopia as a managed
		// external binary (binmgr — not compiled in). Same wrap pattern as loom.
		cmd := kopia.NewKopiaCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "act":
		// Run GitHub Actions locally via a binmgr-managed nektos/act (MIT, not
		// compiled in) — test CI on a host node before pushing. Needs a container
		// engine (bashy podman, unix host). Transparent passthrough.
		cmd := act.NewActCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "act-runner":
		// Gitea act_runner (MIT, binmgr-managed) — the PERSISTENT mesh CI daemon
		// that registers against loom/Gitea and dials OUT (NAT-friendly), distinct
		// from `bashy act` (nektos/act, one-shot local). `register --sandbox` +
		// `daemon --docker-host <bashy podman sock>` gives the tier-3 sandbox
		// executor: `runs-on: sandbox` jobs run in an OCI container.
		cmd := actrunner.NewCmd()
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
	case "kubectl":
		// Kubernetes CLI (Apache-2.0) via binmgr (dl.k8s.io) — targets the DKS
		// cluster by default (external/kube: KUBECONFIG → outpost's DKS kubeconfig).
		cmd := kubectl.NewKubectlCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "helm":
		// Helm — the kubernetes package manager (Apache-2.0) via binmgr
		// (get.helm.sh) — installs charts onto the DKS cluster (same default
		// kubeconfig as `bashy kubectl`).
		cmd := helm.NewHelmCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "sphere":
		// Sphere tier (tier 4): peer-direct pooled p2p inference/compute. Thin
		// front-door that execs the outpost mesh agent at runtime — NO build
		// dependency on outpost (bashy stays the standalone keystone). Without
		// outpost there is no p2p sphere.
		cmd := sphere.NewSphereCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "tessaro":
		// Account / front-door: sign in/out, status, open the portal. Execs the
		// outpost agent at runtime (same exec-never-link discipline as sphere);
		// `tessaro open` works even without it.
		cmd := tessaro.NewTessaroCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "login":
		// Shortcut for `bashy tessaro login` — pair this machine with Tessaro.
		cmd := tessaro.NewLoginCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "mirror":
		// Continuous one-way directory mirror (Syncthing's architecture, all
		// permissive parts: rjeczalik/notify MIT recursive watch + rclone MIT
		// transfer; our own orchestration). Node B keeps a live replica of a dir
		// on node A — point --dest at the replica's exposed rclone.
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
		// the shared real-PID model. WireExec records each `foo &` PID via
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
	if tool.Lookup(os.Args[1]) != nil {
		os.Exit(dispatchCoreutilsTool(os.Args[1], os.Args[2:], tool.Stdio{
			In:  os.Stdin,
			Out: os.Stdout,
			Err: os.Stderr,
		}))
	}
	// Declarative managed-external registry (tier-5/6 client CLIs). A registry
	// verb self-provisions (download → verify → cache → exec) and passes through.
	if e, ok := registry.Lookup(os.Args[1]); ok {
		cmd := e.NewCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	}
	if isEmbeddedSkillName(os.Args[1]) {
		cmd := coreskills.NewSkillsCmd(skillsOptions()...)
		cmd.SetArgs([]string{"show", os.Args[1]})
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy skills:", err)
			os.Exit(coreskills.ExitCode(err))
		}
		os.Exit(0)
	}
	// Unknown first token — not a front-door verb, engine/obs command, or a
	// registered coreutils tool. When it is a BARE command NAME (not an option,
	// path, or existing file), the bashy front-door is being asked to run a
	// command, so report it with the convention agents expect — GNU bash 5.3 /
	// POSIX.2 `command not found`, exit 127 (execute_cmd.c: EX_NOTFOUND) — rather
	// than falling through to the script-file open ("No such file or directory").
	// Options, paths, and real script files still flow to normal bash handling,
	// so the pure `bash` drop-in semantics are untouched.
	if isMissingCommandToken(os.Args[1]) {
		fmt.Fprintf(os.Stderr, "%s: %s: command not found\n", os.Args[0], os.Args[1])
		os.Exit(127)
	}
}

// isMissingCommandToken reports whether a first CLI token should be reported as a
// missing COMMAND (GNU/POSIX "command not found", 127) rather than a missing
// script file (bash's "No such file or directory"): a bare name that is not a
// shell option (- or + prefixed), carries no path separator, and does not exist
// on disk. Existing files and explicit paths keep bash script-file semantics.
func isMissingCommandToken(name string) bool {
	if name == "" {
		return false
	}
	if c := name[0]; c == '-' || c == '+' {
		return false // shell options are the drop-in's job
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, os.PathSeparator) {
		return false // a path → bash script-file semantics
	}
	if _, err := os.Stat(name); err == nil {
		return false // an existing file → run as a script (bash semantics)
	}
	return true
}

func isEmbeddedSkillName(name string) bool {
	for _, skillName := range skills.Names() {
		if skillName == name {
			return true
		}
	}
	return false
}

func dispatchCoreutilsTool(name string, args []string, stdio tool.Stdio) int {
	t := tool.Lookup(name)
	if t == nil {
		fmt.Fprintf(stdio.Err, "bashy: %s: No such command\n", name)
		return 127
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stdio.Err, "bashy: %s: %v\n", name, err)
		return 1
	}
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   os.Environ(),
		FS:    tool.NewLocalFS(),
		Stdio: stdio,
	}
	return t.Run(rc, args)
}

// skillsOptions assembles the skill catalog's sources, standalone-first
// ("cloud as a thin replaceable relay" — the mechanism needs no control
// plane): the embedded ring compiled into bashy, then any shared catalog
// dirs from $BASHY_SKILLS_PATH (path-list of git clones / synced folders,
// read-only), with the host-local store last (~/.config/bashy/skills;
// $BASHY_SKILLS_DIR overrides) so local installs/learning shadow all.
func skillsOptions() []coreskills.Option {
	opts := []coreskills.Option{
		coreskills.WithSource(coreskills.EmbedSource(skills.FS, coreskills.RingEmbedded)),
		coreskills.WithHostVersion("bashy", cli.BashyVersion()),
	}
	for _, dir := range filepath.SplitList(os.Getenv("BASHY_SKILLS_PATH")) {
		if dir != "" {
			opts = append(opts, coreskills.WithSource(coreskills.SharedDirSource(dir)))
		}
	}
	if dir := os.Getenv("BASHY_SKILLS_DIR"); dir != "" {
		opts = append(opts, coreskills.WithConfigDir(dir))
	}
	return opts
}

// runFleet dispatches one of the fleet registry nouns. The catalog is
// standalone-first: the compiled-in baseline answers every read with no
// store, no shared dir, and no cloudbox. $BASHY_FLEET_DIR (and the
// per-noun $BASHY_{TOOLS,MODELS,AGENTS}_DIR / _PATH) are read inside the
// package, so nothing needs wiring here.
func runFleet(noun string, args []string) {
	var cmd *cobra.Command
	exit := fleet.ExitCode
	switch noun {
	case "tools":
		cmd = fleet.NewToolsCmd()
	case "models":
		cmd = fleet.NewModelsCmd()
	case "agents":
		// `agents verify --live` actually launches each agent. The launcher lives
		// in pkg/chat, which reads the fleet registry — so the registry cannot
		// import it, and the binary is the one place both are in scope. The
		// registry declares the hole; here is where it gets filled.
		cmd = fleet.NewAgentsCmd(fleet.WithLiveProbe(liveProbeAgent))
	case "people":
		cmd = principal.NewPeopleCmd()
	case "whois":
		// whois adds exit 3 for an ambiguous name — neither "missing" nor a
		// usage error, and the caller must qualify the query to proceed.
		cmd, exit = principal.NewWhoisCmd(), principal.ExitCode
	default:
		fmt.Fprintln(os.Stderr, "bashy: unknown fleet noun:", noun)
		os.Exit(2)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "bashy %s: %v\n", noun, err)
		os.Exit(exit(err))
	}
	os.Exit(0)
}

// WireExec appends the coreutils ExecHandler so any registered tool resolves
// in-process (pure-Go-first) before PATH lookup. It is wired into the shell
// core via cli.AgentOSWireExec. Shell builtins (echo, pwd, test, …) are handled
// by the interpreter before the ExecHandler runs, so they are never shadowed —
// only external-command names (ls, cat, grep, ast, …) are intercepted.
func WireExec(opts []interp.RunnerOption, posix bool) []interp.RunnerOption {
	// --dry-run (bashy-only, inert under --posix). The handlers are installed
	// whenever NOT in posix mode (they no-op when dry-run is off) so the runtime
	// `set -o dryrun` toggle works even without the flag. EnableDryRunOption
	// makes the engine recognize `set -o dryrun`; the pure bash drop-in never
	// passes it, so it rejects the option exactly like Bash.
	//
	// Record each detached `foo &` real OS PID in the shared coreutils/pkg/jobs
	// registry so `bashy jobs/fg/bg/kill` (Dispatch above) can manage it — the
	// shared real-PID job-control model. Harmless in posix mode (recording only).
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
	// Compose the ExecHandler middleware chain, OUTERMOST first. Audit is
	// outermost so it records the final outcome after every other middleware has
	// run; the advisor is next (it reads the exit to advise); dry-run and the
	// coreutils userland handler are innermost.
	var mws []func(interp.ExecHandlerFunc) interp.ExecHandlerFunc

	// Telemetry is OUTERMOST — outside even audit — so its span covers the true
	// wall-clock and the final exit of everything below it, middleware included.
	//
	// It is a no-op unless OTEL_EXPORTER_OTLP_ENDPOINT is set: no span, no allocation,
	// no wrapper. bashy could already RUN an observability stack (`bashy otel`) and fed
	// it NOTHING — a collector with no data, and the one tier of the whole stack that
	// was invisible while every other service (ycode, outpost, cloudbox, loom) reported.
	mws = append(mws, telemetry.ExecMiddleware)

	if aw := newAuditWriter(); aw != nil {
		mws = append(mws, auditHandler(aw, auditActor(), auditHost()))
	}
	if advisorEnabled() || hintsEnabled() {
		a := newAdvisor()
		if hintsEnabled() {
			opts = append(opts, interp.WithAuditHandler(newNudger(a.mem).onAudit))
		}
		if advisorEnabled() {
			mws = append(mws, advisorHandler(a))
		}
	}
	mws = append(mws, dryRunHandler(r), coreutilsshell.Handler())
	return append(opts, interp.ExecHandlers(mws...))
}

// liveProbeAgent is the launcher behind `bashy agents verify --live`.
//
// It is a thin adapter, and the thinness is the point: it hands the work to
// chat.ProbeAgent, which drives the SAME Invoke path a real turn takes. A probe
// with its own launch logic could pass while production failed — and a probe that
// says "verified" about something that cannot run is worse than no probe at all.
func liveProbeAgent(ctx context.Context, agent string, timeout time.Duration) (string, string, bool) {
	status, note := chat.ProbeAgent(ctx, agent, timeout)

	// Feed the verdict back into the capability matrix, because the router's
	// operability gate was measuring the wrong thing: capability.Operable() is
	// exec.LookPath — "the binary is on disk". agy sat in that matrix at
	// operability 1.0 across 8 samples while rejecting its own model flag on every
	// single run. A binary on disk is not an operable agent, and until now nothing
	// in the fleet could tell the difference.
	//
	// Record against the CANONICAL binding, never the nickname the caller typed.
	// The matrix is keyed by tool:model precisely so that every name for an agent
	// folds into one row — write `opencode-kimi-k2.7-code` instead of
	// `opencode:kimi-k2.7-code` and you have fragmented one agent's evidence
	// across two rows, which is the thing MatrixKey exists to prevent.
	//
	// Best-effort: an unwritable matrix must not fail the verification the
	// operator actually asked for.
	key := agent
	if a, ok := fleet.New().Agent(agent); ok {
		key = a.MatrixKey()
	}
	if err := capability.RecordProbe(key, status.OK()); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not record probe result for %s: %v\n", key, err)
	}
	return string(status), note, status.OK()
}
