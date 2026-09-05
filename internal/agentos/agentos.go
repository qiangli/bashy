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
	"github.com/qiangli/coreutils/pkg/ask"
	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/board"
	"github.com/qiangli/coreutils/pkg/bus"
	"github.com/qiangli/coreutils/pkg/capability"
	"github.com/qiangli/coreutils/pkg/chat"
	"github.com/qiangli/coreutils/pkg/craft"
	"github.com/qiangli/coreutils/pkg/dag"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/gate"
	"github.com/qiangli/coreutils/pkg/handoff"
	"github.com/qiangli/coreutils/pkg/herald"
	"github.com/qiangli/coreutils/pkg/jobs"
	"github.com/qiangli/coreutils/pkg/judge"
	"github.com/qiangli/coreutils/pkg/kb"
	"github.com/qiangli/coreutils/pkg/lexicon"
	"github.com/qiangli/coreutils/pkg/meet"
	"github.com/qiangli/coreutils/pkg/mirror"
	"github.com/qiangli/coreutils/pkg/pair"
	"github.com/qiangli/coreutils/pkg/policy/coord"
	"github.com/qiangli/coreutils/pkg/principal"
	"github.com/qiangli/coreutils/pkg/role/meetroom"
	"github.com/qiangli/coreutils/pkg/schedule"
	"github.com/qiangli/coreutils/pkg/sdlc"
	"github.com/qiangli/coreutils/pkg/search"
	"github.com/qiangli/coreutils/pkg/secrets"
	coreskills "github.com/qiangli/coreutils/pkg/skills"
	"github.com/qiangli/coreutils/pkg/sota"
	"github.com/qiangli/coreutils/pkg/steward"
	"github.com/qiangli/coreutils/pkg/supervise"
	"github.com/qiangli/coreutils/pkg/telemetry"
	"github.com/qiangli/coreutils/pkg/todo"
	"github.com/qiangli/coreutils/pkg/weave"
	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/coreutils/pkg/webconsole"
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
		"weave", "sprint", "todo", "handoff", "resume", "claim", "chat", "delegate", "coach", "meet", "capability", "foreman", "supervise", "agent", "sdlc", "web", "dag", "schedule", "secrets", "ask", "bus", "herald", "search", "sota", "skills", "craft", "kb", "lexicon", "define", "tools", "models", "agents", "people", "whois", "inbox", "notify", "activity", "run", "commands", "context", "doctor", "otel", "audit", "self", "check", "gate", "pair", "judge", "conform", "dhnt", "release", "apps",
		"git", "gh", "act", "act-runner", "rclone", "podman", "ollama",
		"loom", "zot", "seaweedfs", "kopia", "mirror",
		"kubectl", "helm", "sphere", "tessaro", "login", "dks",
	}
	// Direct-only front doors are callable as `bashy NAME` and belong in the
	// command catalog, but must not become bare shell shims. In particular,
	// bare `ping` must continue to resolve to the platform command.
	directFrontDoorVerbs = []string{"mb", "messages", "ping"}
	agentModeShimVerbs   = []string{"go", "cmake", "clang", "node", "npm", "npx", "pnpm", "yarn", "python", "pip", "uv", "mise", "cargo", "rustc", "rustup", "rust", "git-scm", "curl"}
	hiddenFrontDoorVerbs = []string{"bootstrap", "upgrade", "invoke", "verify"}
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
	// Chat, weave, meet and foreman all enter coreutils/chat without passing the
	// communication CLI dispatcher. Wire the receive hook at process startup so
	// every Bashy-owned session gets the same turn-boundary inbox view.
	wireMessageBoard()
	wireSprintOwner()
}

// maybeAdvertiseSkillHint is L1 of the advertisement ladder: when an
// agentic tool is driving (env markers), the repo has no agent config
// at all, and we have not hinted here before — one stderr line pointing
// at the bashy skill. Zero writes to the repo; the once-per-repo marker
// lives in the skills store.
func maybeAdvertiseSkillHint() {
	// The advertisement is a proactive hint and must honor the same documented
	// master switch as every other hint. This is also required for reproducible
	// subprocess evidence, where stderr must not depend on the driving agent.
	if !hintsEnabled() {
		return
	}
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
	store := bashySkillsDir()
	if store == "" {
		return
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

func newBusFrontDoorCmd(name string) (*cobra.Command, string, bool) {
	wireMessageBoard()
	switch name {
	case "mb", "messages":
		return bus.NewMessageBoardCmd(), "mb", true
	case "inbox":
		return newUnifiedInboxCmd(), "inbox", true
	case "notify":
		return bus.NewNotifyCmd(), "notify", true
	default:
		return nil, "", false
	}
}

// wireMeet connects every host-owned meet seam in one place. Keeping these
// assignments together makes the wiring itself testable: package-level tests
// in meet prove each callback's mechanism, while agentos tests prove the
// shipped bashy front door actually installs them.
func wireMeet() {
	meet.StartPermanentRole = startMeetPermanentRole
	meet.StartRoomSecretary = activateMeetRoomSecretary
	meet.ValidateRoomSecretary = validateMeetRoomSecretary
	meet.Notify = notifyMeetInvitation
	meet.FetchMB = fetchMeetMessageBoardPosts
	meet.PostMB = postMeetMessageBoardPost
}

// wireWebConsole connects every seam the `bashy apps` console needs, because it
// serves OTHER verbs' surfaces in its own process and therefore has to install
// their host wiring itself.
//
// It exists as a named function so the set is testable. The console's own
// wiring is exactly the kind that fails silently: with the board seams
// unconnected, `bus.ResolveSendTarget` cannot canonicalize a name through the
// fleet catalog and quietly falls through to reader/person — so a post sent
// from the browser reports success while addressing a name the CLI would have
// resolved differently. Nothing crashes and nothing logs.
func wireWebConsole() {
	wireMeet() // the Relay app is pkg/meet mounted in-process
	// pkg/bus read and posted in-process. This one call serves TWO apps —
	// Messages and Inbox — and the Inbox panel depends on the same fleet seams:
	// with FleetNames nil its roster silently loses every catalog agent and
	// shows only names the timeline happens to mention, which looks like a
	// quiet fleet rather than a missing hook.
	wireMessageBoard()
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
	// `bashy <verb> meta` — the self-description contract, answered from the
	// atlas for any verb that declares a web surface. Narrowly scoped so it
	// cannot steal the word from a verb that takes it as an operand; see
	// dispatchMeta.
	dispatchMeta(os.Args)
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
		// newSprintCmd wraps it so `sprint --help` also carries the ACTIVE
		// owner-accountability contract, not only the plan/handoff mechanics.
		cmd := newSprintCmd()
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
		lcmd := newLexiconCmd()
		lcmd.SetArgs(os.Args[2:])
		if err := lcmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy lexicon:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "define":
		// The one question an agent actually asks, at top level rather than
		// buried under `lexicon` — burying it costs more than the namespace is
		// worth, and an agentic tool reaches for the shortest name that works.
		//
		// Answers from the projected registries (verbs, agent bindings, skills)
		// AND this host's system inventory (env var names, local commands, path
		// segments), falling back to classifying a term by SHAPE when nothing
		// declared it. A term that looks like a credential is classified but
		// never echoed: repeating a key into a terminal or an agent transcript
		// is how it ends up somewhere permanent.
		dcmd := newDefineCmd()
		dcmd.SetArgs(os.Args[2:])
		if err := dcmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy define:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "todo":
		// THE task tracker. Auto-detected scope: inside a git repo → THAT repo's
		// docs/todo/ (committed, the structured replacement for an ad-hoc TODO.md);
		// otherwise the personal host list (~/.bashy/todo/<owner>/). --base-dir shows
		// another repo's list without cd; --user forces the host list. It subsumes the
		// old `issue` register (removed) as the single per-repo/per-host tracker;
		// `weave add --from-todo` seeds a run from a repo todo. The record format (YAML-
		// frontmatter markdown) is shared with the model in coreutils/pkg/issue.
		tcmd := todo.NewTodoCmd()
		tcmd.SetArgs(os.Args[2:])
		if err := tcmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy todo:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "steward":
		// Role namespace (front door for the steward role): bare `steward` and
		// `steward skill` print the existing steward operating skill; `steward
		// dashboard` mounts the same read-only machine-global console as `board`.
		// Wire the role-room seam. pkg/steward cannot import meetroom itself —
		// it sits in the cross-OS build canary that meet's transitive shell
		// interpreter cannot satisfy — so the host, which already links meet,
		// supplies the transport. Same shape as lexicon.RecordDiscovery: one
		// narrow path between two packages that must not import each other.
		steward.OpenRoom = meetroom.Assume
		steward.CloseRoom = meetroom.Release

		renderStewardSkill := func(w io.Writer) error {
			// THE SEAT FIRST, THEN THE MANUAL. An agent arriving on a host asks
			// "is anybody already stewarding this machine" before it asks "how
			// do I steward" — and the answer changes what it should do next.
			// Printing only the skill answered the second question while
			// burying the first, which was reachable only by knowing to type
			// `steward status`.
			//
			// A failure to read the seat is reported, not swallowed: silence
			// there would read as "no steward", which is the one wrong answer
			// that gets someone to seize a seat somebody else is holding.
			if err := steward.SeatSummary(w, ""); err != nil {
				fmt.Fprintf(w, "seat: cannot read (%v)\n\n", err)
			}
			sc := coreskills.NewSkillsCmd(skillsOptions()...)
			sc.SetArgs([]string{"show", "steward"})
			sc.SetOut(w)
			return sc.Execute()
		}
		scmd := board.NewStewardCommand(renderStewardSkill, nil)
		// MOUNT THE SEAT LIFECYCLE. Without this the role's front door offered
		// only its manual and a dashboard: `claim`, `takeover` and `release`
		// — the verbs that actually move the seat — were unreachable from
		// bashy, so every instruction naming them was pointing at a command
		// that did not exist. A front door that describes a role it cannot let
		// you assume is worse than none.
		for _, sub := range steward.NewStewardCmd().Commands() {
			scmd.AddCommand(sub)
		}
		// RUNNING the seat, as opposed to recording it. Everything above reads
		// or writes the journal; nothing above puts an agent on the host. See
		// internal/agentos/steward.go.
		scmd.AddCommand(newStewardStartCmd(), newStewardStopCmd())
		scmd.SetArgs(os.Args[2:])
		if err := scmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy steward:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "conductor":
		// Role namespace: bare `conductor` and `conductor skill` print the
		// conductor operating skill; `conductor dashboard` mounts the board.
		renderConductorSkill := func(w io.Writer) error {
			sc := coreskills.NewSkillsCmd(skillsOptions()...)
			sc.SetArgs([]string{"show", "conductor"})
			sc.SetOut(w)
			return sc.Execute()
		}
		ccmd := board.NewConductorCommand(renderConductorSkill, nil)
		ccmd.SetArgs(os.Args[2:])
		if err := ccmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy conductor:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "pair":
		// gate's SEMANTIC twin, and the successor to `judge`.
		//
		// judge could only TALK: it read a diff and returned approve/revise/reject. That
		// ceiling is why it needed a PANEL -- three opinions voting, because one opinion
		// is unreliable and nothing could check it.
		//
		// A pair ACTS. Give the second agent the keyboard and its finding stops being a
		// claim and becomes a thing that RUNS:
		//
		//     "this breaks on empty input"          -> a claim. Someone must adjudicate it.
		//     a test that FAILS on empty input      -> a proof. The gate reads it.
		//
		// The regress collapses. You do not need three models to vote on whether a bug is
		// real when the bug is executable -- you need one exit code. That is why `pair`
		// subsumes `judge` rather than sitting beside it, and why `judge`'s read-only
		// shackle comes off: judge had to be read-only because it could APPROVE, and an
		// agent that can both write and approve will fix the code and bless its own fix.
		// A pair may never approve. Only the gate may. So it can safely have the keyboard.
		pcmd := pair.NewPairCmd()
		pcmd.SetArgs(os.Args[2:])
		if err := pcmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy pair:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "judge":
		// Kept as an alias so the steward skill, weave, and any script keep working. It
		// maps to the one role that behaves the way judge did: prose, no keyboard, an
		// unverified claim. Reach for `pair --role break` instead.
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
	case "chat", "invoke":
		// Invoke ONE agent, ONCE, on one instruction — the primitive that unifies
		// the heterogeneous agent CLIs (resolve the tool, inject identity, force
		// bashy as its shell). Every orchestrator is built on it: sdlc, foreman and
		// meet all call it; only weave bypasses it (it drives a PTY).
		//
		// The original one-shot `chat` was renamed to `invoke` in 2026-07. Chat
		// later grew a real governed interactive session plus sessions/steer/
		// attach/interrupt/timeline, so `chat` is canonical again and `invoke`
		// remains the hidden compatibility spelling for existing one-shot callers.
		cmd := chat.NewChatCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "coach":
		// Run ONE agent under the LLM-free auto-coach: watch its tool.call stream
		// and, when it loops, ESC it out and tell it to deliver. A report channel,
		// never an author. See dhnt docs/live-agent-coaching-design.md (P0).
		cmd := chat.NewCoachCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "delegate":
		// The ergonomic front for handing a task to an agent — a DIFFERENT one, or
		// YOURSELF (the same tool, run detached to stay responsive). The lightweight
		// one-shot over the same primitive `invoke` uses; the steward's default verb.
		// Heavier ISOLATED work routes to `weave`/the conductor. See
		// bashy/docs/delegate-verb-design.md.
		cmd := chat.NewDelegateCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "meet":
		// Multi-participant deliberation session: agentic CLIs + a human take
		// turns; a dedicated notes-only secretary keeps and files the minutes.
		wireMeet()
		cmd := meet.NewMeetCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy meet:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "apps":
		// The app launcher: ONE local surface with a start page of tiles and every
		// other bashy web surface deep-linked beneath it — the shape
		// dhnt/docs/agent-interaction-surfaces-design.md settled on, where a
		// verb's --web-ui is a MODIFIER that deep-links in here rather than
		// standing up another server ("five unrelated tiles is not bashy on the
		// web: one nav, one auth, one design system").
		//
		// The tile list is not hardcoded: a verb declares a browser UI by
		// carrying an atlas.WebSurface, and `bashy commands --view web` renders
		// the same data in the terminal.
		//
		// Named for the macOS register (Apps / Terminal / Files). outpost serves
		// an unrelated GET /apps — same word, different namespace.
		wireWebConsole()
		cmd := webconsole.NewAppsCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy apps:", err)
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
	case "ping":
		// The front door to the board, and to the classic command. Arity picks:
		// no args reads, a target plus a message posts, a bare target is handed
		// to the system ping.
		//
		// MUST NOT be added to Preamble()'s bare-name shims. `bashy ping` is
		// front-door dispatch; bare `ping` in a bashy shell has to keep
		// resolving to /sbin/ping through the ExecHandler, or a drop-in shell
		// silently changes what a 40-year-old command means. It joins the
		// never-shimmed class with `time` and `kill`.
		wireMessageBoard()
		cmd := bus.NewPingCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy ping:", err)
			os.Exit(1)
		}
		os.Exit(0)
	case "mb", "messages", "inbox", "notify":
		// The host communication front doors all share pkg/bus's fleet seams:
		// identity, role/name resolution, and fleet selection. Mounting the
		// command without this wire-up makes the feature look present while
		// answering addressing questions with empty defaults.
		cmd, label, ok := newBusFrontDoorCmd(os.Args[1])
		if !ok {
			return
		}
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintf(os.Stderr, "bashy %s: %v\n", label, err)
			os.Exit(1)
		}
		os.Exit(0)
	case "leaderboard":
		// The fleet's own run evidence, ranked. A TOP-LEVEL verb rather than
		// `capability leaderboard` because it answers a different question:
		// the matrix is a routing input ("who should take this"), this is an
		// account ("what has actually happened, and how sure are we"). Folding
		// them together would invite reading a routing estimate as a
		// measurement.
		cmd := capability.NewLeaderboardCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy leaderboard:", err)
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
	case "search":
		// Web search (query → cited results) via a provider ladder — the
		// find-things primitive `bashy sota` builds on. See
		// dhnt docs/bashy-sota-research-design.md.
		cmd := search.NewSearchCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			os.Exit(1)
		}
		os.Exit(0)
	case "sota":
		// Research the current state of the art: ground a synthesis agent in real
		// `bashy search` sources and cite only those (anti-hallucination by
		// construction), or --hitchhike on the agent's own web-search tool. See
		// dhnt docs/bashy-sota-research-design.md.
		cmd := sota.NewSotaCmd()
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
	case "bus":
		// The agent notification bus — the PUSH half of how agents coordinate,
		// where `bashy kb` is the durable PULL half. `bus publish` appends an
		// addressed notification to the host room timeline; `bus watch` follows
		// it, or drains what this subscriber has not seen.
		// The catalog seam for `bus subscriptions --reconcile`. pkg/bus must not
		// import pkg/fleet — the bus is transport, the roster is policy — so the
		// host supplies the names, exactly as it does for steward.OpenRoom.
		wireMessageBoard()
		cmd := bus.NewBusCmd()
		cmd.SetArgs(os.Args[2:])
		err := cmd.Execute()
		// bus.Reported means the command already wrote the failure in the form the
		// caller asked for — printing a second, plain-text copy would corrupt the
		// JSON stream it just wrote to stderr.
		if err != nil && !bus.Reported(err) {
			fmt.Fprintln(os.Stderr, "bashy bus:", err)
		}
		os.Exit(bus.ExitCode(err))
	case "herald", "a2a":
		// Reach an agent that is NOT on this host, over A2A. Every other
		// coordination verb resolves a participant to a binary HERE; herald is
		// the path to a capability that cannot be installed here at all.
		//
		// `a2a` is a hidden alias: the protocol name is what an operator will
		// reach for first, but it is not the verb's name — herald speaks more
		// than one wire, and inetd is not called telnetd.
		//
		// The exit status is load-bearing and belongs to the GATE, not to the
		// peer's self-reported task state: 0 only when the gate passed, 2 when
		// the peer claimed completion and nothing verified it. That is what
		// lets a remote agent compose with && like any other command.
		os.Exit(herald.Run(context.Background(), os.Args[2:]))
	case "ask":
		// Ask the HUMAN for an ad-hoc value over a channel the calling program
		// does not own (controlling terminal → GUI askpass → out-of-band
		// rendezvous), and hand back a path rather than the value. Local-first:
		// no network, no pairing — unlike `secrets`, which is the cloudbox vault.
		cmd := ask.NewAskCmd()
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, err)
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
	case "craft":
		// The living skill graph (coreutils/pkg/craft): what running the
		// catalog has TAUGHT this host, as opposed to what the catalog
		// holds. `history` reads back the attestation ledger every
		// `skills run` writes — per skill, or pooled per capability so
		// interchangeable implementations share one track record.
		cmd := craft.NewCraftCmd(craftOptions()...)
		cmd.SetArgs(os.Args[2:])
		if err := cmd.Execute(); err != nil {
			fmt.Fprintln(os.Stderr, "bashy craft:", err)
			os.Exit(craft.ExitCode(err))
		}
		os.Exit(0)
	case "kb":
		// The host-shared knowledge base (coreutils/pkg/kb): the collective
		// memory of all agents on this host across all repos — OKF-style
		// wiki pages under ~/.bashy/kb. The loop: `search` before a task,
		// `add` when nothing relevant exists, `retro` (update/supersede/
		// validate) after the task completes.
		cmd := kb.NewKBCmd()
		cmd.AddCommand(newKBRecallCmd())
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
	case "dhnt":
		// Portable dhnt.pipeline/v1 and dhnt.run/v1 validation, canonical
		// encoding, worker emission, and fail-closed evidence aggregation.
		os.Exit(dispatchDhnt(os.Args[2:]))
	case "release":
		// Distribution: turn a .goreleaser.yaml into named, checksummed
		// artifacts. T0 is the local-first half in-process — `release
		// --snapshot` builds, archives, checksums and emits a bashy-release-v1
		// ledger with no network, no credentials and no tag. Stages this tier
		// does not implement are refused BY NAME by the config loader.
		os.Exit(dispatchRelease(os.Args[2:]))
	case "doctor":
		// Environment self-diagnostic: PATH/sh shadowing, a stale bashy on PATH,
		// toolchain + container engine, agent mode, bin cache. Advisory.
		os.Exit(dispatchDoctor(os.Args[2:]))
	case "activity":
		// The shared activity-event contract: subscription and status controls,
		// route explanation, and the recovery fallback. Recipients READ their
		// activity in `bashy inbox` — this verb is the control surface, not a
		// second inbox. See docs/activity-events.md.
		os.Exit(dispatchActivity(os.Args[2:]))
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
	// The org catalog pulled by `bashy skills sync`. Mounted BEFORE
	// BASHY_SKILLS_PATH so an explicitly-pointed directory still wins, and
	// below the local store so what you added or learned here always wins.
	// Without this the sync would report "N pulled" and the skills would never
	// appear — a wiring gap that reads as an empty org catalog.
	opts = append(opts, coreskills.WithSource(coreskills.SharedDirSource(coreskills.CloudRingDir())))
	for _, dir := range filepath.SplitList(os.Getenv("BASHY_SKILLS_PATH")) {
		if dir != "" {
			opts = append(opts, coreskills.WithSource(coreskills.SharedDirSource(dir)))
		}
	}
	if dir := bashySkillsDir(); dir != "" {
		opts = append(opts, coreskills.WithConfigDir(dir))
	}
	return opts
}

// wireLexicon injects everything pkg/lexicon needs from this host, in ONE
// place, for EVERY entry point into it.
//
// It is a function rather than three assignments at each dispatch site because
// the three assignments drifted, and the drift was silent. `bashy define` set
// all three; `bashy lexicon` set two — so `bashy lexicon study`, which is where
// the study verb actually lives, found RecordDiscovery nil, reported "no fact
// store wired, nothing recorded", and dropped every address it had just
// collected. Nothing failed: the command exited 0 and printed a term count, so
// the only evidence was one line of prose nobody reads on a success path.
//
// The lesson generalises past this bug. A package configured through exported
// vars has as many chances to be misconfigured as it has callers, and a hook
// left nil is indistinguishable from a hook deliberately unset. Collapsing the
// callers to one makes the wiring a property of the package pair rather than a
// thing each dispatch arm must remember.
func wireLexicon() {
	lexicon.Synopses = verbSynopsis
	lexicon.KnownCommands = atlasCommandNames()
	lexicon.RecordDiscovery = recordDiscovery
}

// newLexiconCmd builds `bashy lexicon` — the glossary + its admin verbs
// (list/resolve/emit/scan/study), whose argument sets are CLOSED.
func newLexiconCmd() *cobra.Command {
	wireLexicon()
	return lexicon.NewLexiconCmd()
}

// newDefineCmd builds `bashy define` — the one question an agent actually asks,
// at top level rather than buried under `lexicon`, because an agentic tool
// reaches for the shortest name that works.
//
// NO subcommand is ever mounted here. `define` takes an ARBITRARY token, so
// every subcommand name would permanently remove a word from the definable
// vocabulary: mount `study` and `bashy define study` stops meaning "what is the
// word study" and starts printing a help screen. The hole is invisible until
// somebody asks about that exact word. Actions belong on `lexicon`, whose
// arguments are a closed set — and TestDefineCmdHasNoSubcommands fails the
// build if one appears here.
func newDefineCmd() *cobra.Command {
	wireLexicon()
	return lexicon.NewDefineCmd()
}

// recordDiscovery routes an identity-bearing collection finding into the
// host-local fact store.
//
// The wiring lives HERE rather than inside pkg/lexicon on purpose: a glossary
// that could write to a fact store would eventually be asked to read from one,
// and identity would start flowing toward the shareable side. Keeping the two
// packages unaware of each other means the only path between them is this
// function, which is short enough to audit.
//
// A side effect worth knowing: every address recorded this way makes the fold
// admission gate STRICTER, because HostScrubber seeds from the fact store — so
// collecting identity is what teaches the system to refuse leaking it.
func recordDiscovery(d lexicon.Discovery) error {
	store := craft.OpenFacts(craftStoreDir())
	return store.Record(craft.Fact{
		Entity: craft.Entity{Kind: craft.EntityKind(d.EntityKind), Name: d.EntityName},
		Key:    d.Key,
		Value:  d.Value,
		// The verb that produced it, spelled the way an operator would type it.
		// It was "define study", which is not a command and never was — a
		// provenance field that names a nonexistent source is worse than an
		// empty one, because it looks checkable.
		Source: "lexicon study",
	})
}

// bashySkillsDir is the ONE resolver for the skills store, shared by the
// catalog, the craft graph, the learning hook, and the repo-hint marker.
//
// It has to be one function. Four call sites each spelling out the same ladder
// is not a style problem: the moment one of them grows a rung the others lack,
// the catalog and its own history point at different directories — and that
// splits silently. `craft facts` would read one store while the learn hook
// wrote another, so facts would be recorded and then simply not be there. The
// craftOptions comment already asserted the two "move together"; this is what
// makes that true rather than a convention.
//
// The ladder follows the one audit and foreman already use:
//
//	$BASHY_SKILLS_DIR   the specific override, most precise, wins
//	$BASHY_HOME/skills  the whole bashy home relocated (test isolation, a
//	                    per-workspace home, a sandboxed run)
//	~/.config/bashy/skills
//
// $BASHY_HOME was the missing rung. Setting it moved the audit log and the
// foreman state but left facts writing to the real home, which makes a store
// that looks isolated and is not — the worst version, because a test that
// believes it is hermetic will pass on data it did not create.
//
// An empty return means no home could be determined. Callers must treat it as
// "no store" rather than as a path.
func bashySkillsDir() string {
	if dir := strings.TrimSpace(os.Getenv("BASHY_SKILLS_DIR")); dir != "" {
		return dir
	}
	if home := strings.TrimSpace(os.Getenv("BASHY_HOME")); home != "" {
		return filepath.Join(home, "skills")
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "bashy", "skills")
	}
	return ""
}

func craftStoreDir() string { return bashySkillsDir() }

// atlasCommandNames is the standard command surface — the pure-Go userland plus
// bashy's own verbs — which the lexicon subtracts when enumerating this host's
// LOCAL commands.
//
// The atlas is the right source precisely because it is ratcheted: its coverage
// tests fail the build when a registered tool has no entry, so this set cannot
// silently fall behind the surface it is meant to describe. Anything on PATH and
// not in here is, by construction, something this machine has and a stock one
// does not — which is the definition of local jargon.
func atlasCommandNames() []string {
	names := atlas.ToolNames()
	return append(names, atlas.VerbNames()...)
}

// craftOptions points the living skill graph at the SAME store the skills
// catalog writes to — craft is a reader over that evidence, not a second
// store. $BASHY_SKILLS_DIR therefore moves both together, which is what keeps
// a redirected store from silently splitting the catalog from its history.
func craftOptions() []craft.Option {
	// The SAME catalog options the skills CLI gets, so `craft find` indexes
	// exactly what `skills list` shows. Two views of one store that disagree
	// about what exists surfaces only as a skill mysteriously not being found.
	opts := []craft.Option{craft.WithSkillOptions(skillsOptions()...)}
	if dir := bashySkillsDir(); dir != "" {
		opts = append(opts, craft.WithStoreDir(dir))
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
		// `agents verify --live` actually launches each agent, and `agents clone`
		// branches its conversation store. Both live in pkg/chat, which reads the
		// fleet registry — so the registry cannot import it, and the binary is the
		// one place both are in scope. The registry declares the holes; here is
		// where they get filled.
		cmd = newAgentsRosterCmd(
			fleet.WithLiveProbe(liveProbeAgent),
			fleet.WithContextCloner(chat.CloneAgentContext),
		)
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
	// R0-pre: file a presence card for the agent this shell runs under, so an
	// agent launched outside `bashy chat` stops being invisible to the address
	// book. Best-effort, silent, and never a claim — see shellsession.go.
	registerShellSession()
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
	// The agentic history recorder sits just inside audit and just outside the
	// advisor. Inside audit because audit's chained record must remain the
	// outermost account of what happened; outside the advisor because the
	// advisor SPEAKS on failure while this one only WRITES, and a store that
	// missed whatever the advisor did to the outcome would be recording a
	// different command from the one that ran.
	//
	// It records every dispatched command (the TIME plane) and folds what each
	// one taught about hosts, endpoints and accounts into the entity graph (the
	// SPACE plane). Off entirely for interactive humans, and never linked into
	// cmd/bash — WireExec returns above before any of this in posix mode.
	if execHistEnabled() {
		mws = append(mws, execHistHandler(newRecorder()))
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
	// Learning sits just inside the advisor: it reads the same exit code, but
	// where the advisor SPEAKS on failure this one LISTENS on success. Passive,
	// stderr-silent, and it never alters an outcome.
	if learnEnabled() {
		mws = append(mws, learnHandler())
	}
	// The weave isolation guard speaks BEFORE the command, where the advisor
	// speaks after it. weave already detects that the live checkout moved while
	// a run held its workspace — but only on the read path, so the person who
	// caused it learns at the next `weave list`, by which point the run is
	// unmergeable without --force. Advisory only: one line, exit untouched.
	if weaveGuardEnabled() {
		mws = append(mws, weaveGuardHandler)
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
