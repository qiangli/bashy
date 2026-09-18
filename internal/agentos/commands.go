// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// `bashy commands` lists the whole supported command surface in one place:
// shell builtins, the in-process coreutils userland, and the bare-name
// front-door verbs. The coreutils tools and verbs are dispatched by the
// ExecHandler before PATH, so they are otherwise invisible to `compgen`/`type`
// (which only see builtins, functions, and PATH) — this is the only way to
// discover them from inside the shell. Bashy-only (never the pure cmd/bash).
package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"slices"
	"sort"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/coreutils/tool"
	"github.com/qiangli/yoke/external/registry"
	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/fleet"
)

const commandsSchemaVersion = "bashy-commands-v1"

// commandsCRUDWords are the `commands` sub-verbs that take a NAME. Each is
// recognized only when a non-flag argument follows: bare `commands rm`
// keeps its lifelong meaning — "show rm's record" — because `rm`, `set`,
// `edit` and `verify` all name commands too. `list` and `schema` take no
// name and collide with nothing.
var (
	commandsCRUDWords     = []string{"add", "show", "set", "rm", "edit", "verify"}
	commandsNamelessWords = []string{"list", "schema"}
)

// commandsRegistryArgs reports whether args address the registered-command
// ring rather than the lister, and rewrites `show NAME` (bare) to the
// lister's own one-command report, which is what `commands NAME` already is.
func commandsRegistryArgs(args []string) ([]string, bool) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return args, false
	}
	if containsString(commandsNamelessWords, args[0]) {
		return args, true
	}
	if !containsString(commandsCRUDWords, args[0]) || len(args) < 2 || strings.HasPrefix(args[1], "-") {
		return args, false
	}
	if args[0] == "show" {
		for _, a := range args[2:] {
			if a == "--yaml" || a == "--json" || strings.HasPrefix(a, "--field") {
				return args, true // the RECORD, not the atlas report
			}
		}
		return args[1:], false // `commands show NAME` ≡ `commands NAME`
	}
	return args, true
}

// runCommandsRegistry mounts the fleet CRUD tree with bashy's holes filled:
// the whole command surface as the collision filter, and the shell as the
// script syntax probe. The index is dropped afterwards so a `NAME` typed
// right after `add NAME` resolves.
func runCommandsRegistry(args []string) int {
	defer resetRegisteredIndex()
	cmd := fleet.NewCommandsCmd(fleet.WithReservedNames(reservedCommandName), fleet.WithCommandProbe(scriptSyntaxProbe))
	cmd.Use = "commands"
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "bashy commands: %v\n", err)
		return fleet.ExitCode(err)
	}
	return 0
}

func dispatchCommands(args []string) int {
	if rest, registry := commandsRegistryArgs(args); registry {
		return runCommandsRegistry(rest)
	} else {
		args = rest
	}
	asJSON, verbose := weavecli.IsAgent(), false // JSON by default under $BASHY_AGENTIC
	agentic, all, gnu, features := false, false, false, false
	var view, tierFilter, groupFilter, capFilter, effectFilter string
	var osFilter string // "" = this host; "any" = no platform filter
	portable := false
	idioms, atlasFull := false, false
	var query string
	// valued reads a "--flag value" / "--flag=value" option; ok=false means
	// the value is missing (usage error, already reported).
	valued := func(a, name string, i *int) (string, bool, bool) {
		if a == name {
			if *i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "commands: %s requires a value\n", name)
				return "", false, true
			}
			*i++
			return args[*i], true, true
		}
		if strings.HasPrefix(a, name+"=") {
			return strings.TrimPrefix(a, name+"="), true, true
		}
		return "", false, false
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if v, ok, matched := valued(a, "--view", &i); matched {
			if !ok {
				return 2
			}
			view = v
			continue
		}
		if v, ok, matched := valued(a, "--tier", &i); matched {
			if !ok {
				return 2
			}
			tierFilter = v
			continue
		}
		if v, ok, matched := valued(a, "--group", &i); matched {
			if !ok {
				return 2
			}
			groupFilter = v
			continue
		}
		if v, ok, matched := valued(a, "--cap", &i); matched {
			if !ok {
				return 2
			}
			capFilter = v
			continue
		}
		if v, ok, matched := valued(a, "--effect", &i); matched {
			if !ok {
				return 2
			}
			effectFilter = v
			continue
		}
		if v, ok, matched := valued(a, "--os", &i); matched {
			if !ok {
				return 2
			}
			if v != "any" && v != "all" && !containsString(atlas.OSes(), v) {
				fmt.Fprintf(os.Stderr, "commands: unknown os %q (os: %s any)\n", v, strings.Join(atlas.OSes(), " "))
				return 2
			}
			if v == "all" {
				v = "any"
			}
			osFilter = v
			continue
		}
		switch a {
		case "--portable", "--xplat", "--cross-platform":
			portable = true
			continue
		case "--json", "--json=true":
			asJSON = true
		case "--json=false", "--plain":
			asJSON = false
		case "--agentic":
			agentic = true
		case "--all":
			all = true
		case "--gnu", "--gnu-coreutils", "--coreutils-gaps":
			gnu = true
		case "--features":
			features = true
			asJSON = true
		case "--idioms":
			idioms = true
		case "--atlas":
			atlasFull = true
		case "-v", "--verbose":
			verbose = true
		case "-h", "--help":
			fmt.Println("usage: commands [COMMAND] [-v] [--json|--plain|--agentic|--all|--gnu|--features]")
			fmt.Println("                [--view VIEW] [--tier T] [--group G] [--cap C] [--idioms] [--atlas]")
			fmt.Println("List the supported command surface, grouped by how each command runs:")
			fmt.Println("a builtins umbrella (shell builtins · in-process GNU coreutils · in-process")
			fmt.Println("classic tools), the exec'd downloaded externals, and bashy's native agent")
			fmt.Println("features by execution venue.")
			fmt.Println("  COMMAND        show one command's class/resolver/synopsis")
			fmt.Println("  -v             also show each coreutils tool's and verb's synopsis")
			fmt.Println("  --json         machine-readable (default under $BASHY_AGENTIC)")
			fmt.Println("  --json=false   force text even under $BASHY_AGENTIC (alias --plain)")
			fmt.Println("  --agentic      compact agent-oriented discovery and safety guide")
			fmt.Println("  --all          include hidden compatibility aliases")
			fmt.Println("  --gnu          include GNU coreutils parity/gap inventory")
			fmt.Println("  --features     machine-readable one-command feature/gap report")
			fmt.Println("Command Atlas views:")
			fmt.Printf("  --view VIEW    classic | %s\n", strings.Join(atlasViews, " | "))
			fmt.Println("  --tier T       filter by execution tier (userland/workspace/sandbox/…)")
			fmt.Println("  --group G      filter by functional group (fileutils/code-intel/…)")
			fmt.Println("  --cap C        filter by agentic capability (json/read-only/…)")
			fmt.Println("  --effect E     filter by security effect (destroy/cred/priv/remote/net/…)")
			fmt.Println("  --os OS        platform filter: darwin | linux | windows | any (default: this host)")
			fmt.Println("  --portable     only commands with full support on every platform (composes with any view)")
			fmt.Println("  --idioms       curated composites: commands naturally used together")
			fmt.Println("  --atlas        full per-command atlas records (machine surface)")
			fmt.Println("Registered commands — your own, listed and dispatched like every shipped one:")
			fmt.Println("  add NAME --set exec.0=PROG|script=BODY|download.url=…   register (schema lists every path)")
			fmt.Println("  set NAME --set PATH=VALUE | rm NAME | edit NAME | verify [NAME]")
			fmt.Println("  show NAME --yaml|--json|--field PATH   the record; bare `commands NAME` is the atlas report")
			fmt.Println("  list | schema                          the ring; every settable path")
			fmt.Println("  A CRUD word counts only when a NAME follows it: bare `commands rm` still shows rm's record.")
			fmt.Println("  A name bashy already ships is refused; a PATH program may be shadowed. sync: not yet.")
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "commands: unknown option %q\n", a)
				return 2
			}
			if query != "" {
				fmt.Fprintf(os.Stderr, "commands: only one COMMAND query is supported\n")
				return 2
			}
			query = a
		}
	}

	if view == "classic" {
		view = "" // explicit alias for the default output
	}
	if view != "" || tierFilter != "" || groupFilter != "" || capFilter != "" || effectFilter != "" || idioms || atlasFull || portable {
		return dispatchAtlas(atlasRequest{
			view: view, tier: tierFilter, group: groupFilter, cap: capFilter, effect: effectFilter,
			idioms: idioms, full: atlasFull, asJSON: asJSON, all: all, verbose: verbose,
			os: osFilter, portable: portable,
		})
	}

	if agentic {
		printAgenticCommands(os.Stdout)
		return 0
	}

	builtins, core, verbs := commandsCatalog()
	hidden := hiddenVerbsCatalog()
	gnuReport := gnuCoreutilsReport(core, builtins)
	if query != "" || features {
		// Name lookup is never platform-filtered: `bashy commands ps` on macOS
		// answers "only on linux" rather than "not found".
		info := commandFeatureReport(query, builtins, core, append(append([]string(nil), verbs...), hidden...), hidden, gnuReport)
		if asJSON {
			b, _ := json.Marshal(info)
			fmt.Println(string(b))
			if info["class"] == "not-found" {
				return 1
			}
			return 0
		}
		printCommandFeature(os.Stdout, info)
		if info["class"] == "not-found" {
			return 1
		}
		return 0
	}

	// The default surface is this host's (`--all`, like `--os any`, lifts the
	// platform filter along with the hidden one).
	hostOS := runtime.GOOS
	if all || osFilter == "any" {
		hostOS = "any"
	} else if osFilter != "" {
		hostOS = osFilter
	}
	core = supportedOn(hostOS, core)
	verbs = supportedOn(hostOS, verbs)
	if all {
		verbs = append(verbs, hidden...)
		sort.Strings(verbs)
	}
	if asJSON {
		registered := registeredNames()
		if registered == nil {
			registered = []string{}
		}
		out := map[string]any{
			"schema_version": commandsSchemaVersion,
			"builtins":       builtins,
			"coreutils":      core,
			"verbs":          verbs,
			// Additive since Sprint 179: the operator's ring, also present in
			// verbs (it dispatches at the front door). Always a list.
			"registered": registered,
		}
		if all {
			out["hidden_verbs"] = hidden
		}
		if gnu {
			out["gnu_coreutils"] = gnuReport
		}
		if verbose {
			// Additive: a flat name→synopsis map for the described commands
			// (builtins have none in the fork, so they are omitted here).
			syn := map[string]string{}
			for _, n := range core {
				if t := tool.Lookup(n); t != nil && t.Synopsis != "" {
					syn[n] = t.Synopsis
				}
			}
			for _, n := range verbs {
				if s := synopsisOf(n); s != "" {
					syn[n] = s
				}
			}
			out["synopses"] = syn
			// The by-how-it-runs grouping the human view renders (builtins
			// umbrella {shell/coreutils/classic} · external · agent-by-venue).
			// Verbose-only, so the default v1 --json schema stays stable.
			out["sections"] = classSections(all)
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
		return 0
	}

	// The default surface is organized by how a command runs: a "builtins"
	// umbrella (shell / coreutils / classic — all in-process, no fork), the
	// exec'd externals, and bashy's native agent features partitioned by venue.
	// See commands_sections.go. -v adds one-line synopses.
	printClassSections(os.Stdout, verbose, all, hostOS)
	if gnu {
		printGNUCoreutilsReport(os.Stdout, gnuReport)
	}
	return 0
}

func printAgenticCommands(w io.Writer) {
	fmt.Fprint(w, `agentic bashy commands — the 1.0.0 core first (bashy commands for the full first screen):
  bashy agent list               the registered tool:model bindings; agent whoami = this process
  bashy model list / tool list   the inference backends and the agentic CLIs this host can drive
  bashy skill list               tier-2 workspace skills applicable here
  bashy chat AGENT "..."         one governed instruction to an agent (delegate: hand off a task)
  bashy sprint / todo            the cross-repo board and this repo's committed task list
  bashy kb context --for TASK --rings repo,host --forms note,page --budget 700 --json
                                  assemble bounded knowledge for a task
  bashy kb observe --ring agent --episode E --kind KIND --ref REF
                                  journal an observation without writing a page
  bashy inbox / mb / notify      what reached you; the host board; send one subject to an agent
  bashy meet                     multi-participant deliberation with a notes-only secretary
  bashy dag --list               list markdown DAG targets; bashy weave = per-repo workspace runs
  bashy gate                     does this project pass? (the one command that decides)
  bashy graph impact SYMBOL      code-graph blast radius: what's coupled to a symbol
  bashy app / ask / browser      open the console; ask the HUMAN for a value; drive a browser
  bashy fetch --json URL         built-in URL/REST client with status envelope

discovery:
  bashy commands                 the 1.0.0 surface: 35 core + visible extras; --all for everything
  bashy commands --view origin   every name by who defined it (bash · GNU · Unix · external · yoke)
  bashy commands --view posix    the 116 POSIX-required utilities: internal (pure Go) vs bin-managed
  bashy commands --view external what is downloaded + exec'd, and the pure-Go debt
  bashy commands NAME            one command: class, origin, capabilities, gaps (--features for JSON)
  bashy commands --idioms        commands naturally used together (composites)
  bashy inspect context --json   first-hop context: exact bashy path + capabilities
  bashy inspect doctor           diagnose PATH, shell, engine, and agent environment

safety:
  bashy help dryrun              explain dry-run safety mode and JSON manifest
  BASHY_AGENTIC=1 bashy --dry-run script.sh
                                  preview external commands, rm, and truncation as JSON-lines
  bashy --dry-run -c 'commands'  human-readable dry-run preview
  bashy help output              bounded, recoverable command output (on under BASHY_AGENTIC)
  bashy sandbox ...              the tier-3 venue: isolated in-process container engine
  bashy git ...                  embedded pure-Go git client

dry-run JSON entry kinds:
  command   external command availability and resolved path
  destroy   destructive rm target count, bytes, and sample paths
  truncate  redirection clobber of an existing file
`)
}

func commandFeatureReport(name string, builtins, core, verbs, hidden []string, gnu gnuCoreutilsInventory) map[string]any {
	out := map[string]any{
		"schema_version": commandsSchemaVersion,
		"name":           name,
		"class":          "not-found",
		"resolver":       "not-found",
		"available":      false,
	}
	if name == "" {
		out["error"] = "COMMAND is required with --features"
		return out
	}
	switch {
	case containsString(builtins, name):
		out["class"], out["resolver"], out["available"] = "builtin", "bash-builtin", true
		atlasFeatureFields(out, name, "builtin", false)
	case containsString(core, name):
		out["class"], out["resolver"], out["available"] = "coreutils", "bashy-in-process", true
		if t := tool.Lookup(name); t != nil && t.Synopsis != "" {
			out["synopsis"] = t.Synopsis
		}
		atlasFeatureFields(out, name, "coreutils", false)
	case containsString(verbs, name):
		out["class"], out["resolver"], out["available"] = "verb", "bashy-front-door", true
		if s := synopsisOf(name); s != "" {
			out["synopsis"] = s
		}
		atlasFeatureFields(out, name, "verb", false)
		registeredFeatureFields(out, name)
	case containsString(hidden, name) && tool.Lookup(name) != nil:
		// A curated-hidden in-process tool (tokens, posix-gate).
		out["class"], out["resolver"], out["available"], out["hidden"] = "coreutils", "bashy-in-process", true, true
		if t := tool.Lookup(name); t != nil && t.Synopsis != "" {
			out["synopsis"] = t.Synopsis
		}
		atlasFeatureFields(out, name, "coreutils", true)
	case containsString(hidden, name):
		out["class"], out["resolver"], out["available"], out["hidden"] = "verb", "bashy-front-door", true, true
		if s := synopsisOf(name); s != "" {
			out["synopsis"] = s
		}
		atlasFeatureFields(out, name, "verb", true)
		registeredFeatureFields(out, name)
	case containsString(gnu.Missing, name):
		out["class"] = "gnu-coreutils-missing"
		out["resolver"] = "managed-container-or-system"
		out["gnu_coreutils_status"] = "missing-from-bashy-native"
	case containsString(gnu.CoveredByBuiltins, name):
		out["class"], out["resolver"], out["available"] = "builtin", "bash-builtin", true
		out["gnu_coreutils_status"] = "covered-by-bash-builtin"
	}
	for _, gap := range gnu.Not100Conformant {
		if gap.Name == name {
			out["gnu_coreutils_status"] = gap.Status
			out["gnu_coreutils_gap"] = gap.Reason
			break
		}
	}
	if name == "grep" {
		out["known_gaps"] = []string{"BRE/ERE back-references are not supported by the current RE2-backed implementation"}
		out["agent_hint"] = "avoid grep patterns with back-references, or use a GNU grep fallback/container for those scripts"
	}
	return out
}

// originLine renders the provenance + visibility line of a one-command
// report: where the command came from, whether POSIX requires it, whether it
// is 1.0.0 core, and — for a hidden one — why it is hidden and what to type
// instead.
func originLine(info map[string]any) string {
	var parts []string
	if o, ok := info["origin"].(string); ok && o != "" {
		parts = append(parts, "origin: "+atlas.OriginLabel(o))
	}
	if p, ok := info["posix"].(bool); ok && p {
		parts = append(parts, "POSIX-required")
	}
	if c, ok := info["core"].(bool); ok && c {
		parts = append(parts, "1.0.0 core")
	}
	if st, ok := info["status"].(string); ok && st == "alias" {
		if a, ok := info["alias_of"].(string); ok && a != "" {
			parts = append(parts, "hidden spelling of `bashy "+a+"`")
		} else {
			parts = append(parts, "hidden spelling")
		}
	} else if st, ok := info["status"].(string); ok && st != "" {
		parts = append(parts, st+" (hidden; `bashy commands --all` lists it)")
	} else if h, ok := info["hidden"].(bool); ok && h {
		if a, ok := info["alias_of"].(string); ok && a != "" {
			parts = append(parts, "hidden alias of `bashy "+a+"`")
		} else {
			parts = append(parts, "hidden")
		}
	}
	if use, ok := info["use"].(string); ok && use != "" && info["status"] != "alias" {
		parts = append(parts, "use `bashy "+use+"`") // an alias line already names it
	}
	switch {
	case info["unsupported_here"] != nil:
		oses, _ := info["os"].([]string)
		parts = append(parts, fmt.Sprintf("NOT supported on %s (only: %s)", info["unsupported_here"], strings.Join(oses, " ")))
	case info["portable"] == true:
		parts = append(parts, "portable (windows · macOS · linux)")
	default:
		if oses, ok := info["os"].([]string); ok && len(oses) > 0 && len(oses) < 3 {
			parts = append(parts, "only on "+strings.Join(oses, " "))
		}
		if p, ok := info["partial"].([]string); ok && len(p) > 0 {
			parts = append(parts, "partial on "+strings.Join(p, " "))
		}
	}
	return strings.Join(parts, " · ")
}

func printCommandFeature(w io.Writer, info map[string]any) {
	fmt.Fprintf(w, "%s: %s via %s\n", info["name"], info["class"], info["resolver"])
	if line := originLine(info); line != "" {
		fmt.Fprintf(w, "  %s\n", line)
	}
	if s, ok := info["synopsis"].(string); ok && s != "" {
		fmt.Fprintf(w, "  %s\n", s)
	}
	if gap, ok := info["gnu_coreutils_gap"].(string); ok && gap != "" {
		fmt.Fprintf(w, "  GNU coreutils gap: %s\n", gap)
	}
	if hint, ok := info["agent_hint"].(string); ok && hint != "" {
		fmt.Fprintf(w, "  agent hint: %s\n", hint)
	}
}

func containsString(items []string, want string) bool {
	return slices.Contains(items, want)
}

// verbSynopsis describes the front-door verb shims (the coreutils tools carry
// their own Synopsis; builtins are standard). Brand-neutral, one line each.
var verbSynopsis = map[string]string{
	"oci":         "the sandboxing pillar (O3: ollama · oci · otel): the tier-3 container engine by its standard name — embedded, isolated, in-process (RAW engine, not outpost's filtered sandbox app)",
	"docker":      "vendor spelling of `bashy oci` (the embedded container engine)",
	"sandbox":     "the tier-3 venue, by its popular name: same engine as `bashy oci`",
	"podman":      "vendor spelling of `bashy oci` (the engine underneath)",
	"ollama":      "the LLM pillar (O3: ollama · oci · otel): managed local LLM runtime — isolated daemon, own port/models",
	"weave":       "per-repo multi-agent workspace orchestrator",
	"sprint":      "cross-repo plan/continuity board (peer to weave)",
	"handoff":     "pause this session and hand the work to another agent, a scheduler, or tomorrow",
	"resume":      "pick up a handed-off session — any tool, any machine",
	"claim":       "who is working in this project — and hold it while you write",
	"define":      "what is this word on THIS system? verb, agent, env var, command, path, address — or unknown",
	"lexicon":     "what do this project's words mean HERE? (verbs + agent bindings, projected)",
	"gate":        "does this project pass? (the one command that decides)",
	"pair":        "two agents + a gate: one proposes, one BREAKS it (writes the failing test) — a proof, not an opinion",
	"judge":       "panel verdict on work with NO gate to run (a plan, a design). For code, use `pair` — it acts",
	"todo":        "task list, git-repo aware: this repo's committed docs/todo/ (or a --base-dir), else the host's ~/.bashy/todo/",
	"invoke":      "hidden compatibility alias for bashy chat",
	"delegate":    "hand a task to an agent — another one, or YOURSELF (same tool, run detached to stay responsive)",
	"coach":       "run an agent under an LLM-free auto-coach that ESCs it out of doomed tool-loops and tells it to deliver",
	"search":      "web search (query → cited results) via a provider ladder (tavily/brave/serper) — the find-things primitive",
	"sota":        "research the current state of the art: ground a synthesis agent in real bashy-search sources (cite only those), or --hitchhike on the agent's own subscription web search",
	"conform":     "bashy's OWN fidelity batteries: compat/conformance/compliance/benchmark",
	"chat":        "talk to an agent in a governed live session, or send one unattended instruction",
	"meet":        "multi-participant deliberation session with a notes-only secretary",
	"app":         "open bashy's apps in a browser: Terminal, Files, Meet, and every declared surface",
	"apps":        "hidden alias for bashy app",
	"supervise":   "drive a fleet against a goal of gated tasks, judged by a supervisor (conductor-as-a-verb)",
	"capability":  "living agent (tool:model) × capability matrix for routing",
	"leaderboard": "rank this host's agents on the runs they actually completed",
	"mb":          "host message board: read what was posted to you, post to others",
	"messages":    "hidden alias for bashy mb",
	"issue":       "hidden alias for bashy todo",
	"ping":        "read or send on the host message board, or run system ICMP ping when given only a host",
	"out":         "recover complete command output by its digest-prefix handle",
	"full":        "run one command with output reduction disabled",
	"awd":         "run one command in another directory and return (the awd builtin, from the front door)",
	"inbox":       "read/watch every inbound source, or query an explicit durable agent/human mailbox",
	"notify":      "send one subject-only notification to an agent or role",
	"activity":    "activity-event contract: subscribe to system activity, see why an event reached you",
	"foreman":     "drive a persistent, steerable agent session (chat elevated)",
	"sdlc":        "route intake issues through agentic implementation and deployment gates",
	"web":         "web inspection helpers for SDLC verification",
	"dag":         "agent-first markdown DAG task runner",
	"transpile":   "compile Bash++ source to ordinary Go with source maps",
	"schedule":    "modern cron: run a command on a cron/interval/at schedule",
	"secret":      "managed API-key/token vault for the shell",
	"secrets":     "hidden alias for bashy secret",
	"bus":         "agent notification bus: `bus publish` a change, `bus watch` for one (kb holds facts; the bus carries changes)",
	"herald":      "reach an agent that is not on this host, over A2A: `herald add` a peer, `herald send` it a task, and gate the result",
	"ask":         "ask the HUMAN operator for an ad-hoc value (a token, an OTP) — never the model; returns a path, not the value",
	"skill":       "tier-2 workspace skills, env-gated: list applicable here, probe the coordinate, show one",
	"skills":      "hidden alias for bashy skill",
	"craft":       "the living skill graph: what running skills has taught this host, per skill or per capability",
	"kb":          "ring-aware knowledge stages: context/search, observe, validate, and candidate note add (JSON-capable)",
	"tool":        "the agentic CLI harnesses this host can drive (claude, codex, opencode, ...)",
	"model":       "the inference backends the fleet can bind to (subscription, api, local)",
	"agent":       "active weave assignments; `agent list` shows registered tool:model bindings, `agent whoami` this process's identity",
	"person":      "human principals — who the names in prose refer to",
	"tools":       "hidden alias for bashy tool",
	"models":      "hidden alias for bashy model",
	"agents":      "hidden alias for bashy agent",
	"people":      "hidden alias for bashy person",
	"whois":       "resolve any name (person/agent/tool/model/host) and say how to reach it",
	"run":         "run a command, emit a structured result envelope (+advisor hints)",
	"agentic":     "run one action in agentic mode; return a skill proposal when caller input is required",
	"dhnt":        "validate, lower, emit, and aggregate portable dhnt pipeline/run evidence",
	"release":     "build → archive → checksum this project's release artifacts from .goreleaser.yaml (`--snapshot`: no tag, no network)",
	"commands":    "the 1.0.0 command surface: core first, then every command by origin, tier, POSIX, or bin-managed status; add|set|rm|… registers your own",
	"command":     "hidden front-door spelling of `commands` (in-shell `command` stays the POSIX builtin)",
	"inspect":     "self-inspection: resource map, gate decisions, and the index of what answers what",
	"resource":    "host totals and active weave disk, CPU, GPU, and memory usage",
	"resources":   "hidden alias for bashy resource",
	"context":     "hidden alias for bashy inspect context",
	"doctor":      "hidden alias for bashy inspect doctor",
	"otel":        "the telemetry pillar (O3: ollama · oci · otel): query OTel telemetry with bounded agent-readable summaries",
	"audit":       "hidden alias for bashy inspect audit",
	"check":       "statically check shell scripts for bashy/system command closure and --bashsharp null safety",
	"verify":      "run formal test batteries: compat/conformance/compliance/benchmark",
	"self":        "fetch/cache/install a released bashy binary",
	"bootstrap":   "hidden alias for bashy self",
	"upgrade":     "hidden alias for bashy self",
	"git":         "real git (git-for-windows MinGit on Windows; system git on unix), verified; `outpost git` is the pure-Go bootstrap client",
	"gh":          "GitHub CLI (managed external)",
	"act":         "run GitHub Actions locally (managed external)",
	"act-runner":  "Gitea CI daemon; register --sandbox + daemon --docker-host = tier-3 sandbox executor (managed external)",
	"rclone":      "cloud-storage transfer + file server (managed external)",
	"mirror":      "continuous one-way directory mirror (managed external)",
	"loom":        "Gitea git forge (managed external)",
	"zot":         "OCI registry for images + models (managed external)",
	"seaweedfs":   "object/blob store with S3 gateway (managed external)",
	"kopia":       "snapshot-backup repository server (managed external)",
	"go":          "self-provisioning Go toolchain (download → verify → cache → exec)",
	"cmake":       "self-provisioning CMake build toolchain",
	"clang":       "self-provisioning clang/LLVM toolchain",
	"node":        "self-provisioning Node.js runtime (download from nodejs.org → verify → cache → exec)",
	"npm":         "Node.js package manager (from the provisioned Node tree)",
	"npx":         "Node.js package runner (from the provisioned Node tree)",
	"pnpm":        "pnpm package manager via the Node-bundled corepack (self-provisioning)",
	"yarn":        "yarn package manager via the Node-bundled corepack (self-provisioning)",
	"python":      "self-provisioning Python via astral uv (download → verify → cache → exec)",
	"pip":         "Python package installer via uv (self-provisioning)",
	"uv":          "uv — Python package/project manager (managed external, verified)",
	"mise":        "polyglot runtime/version manager jdx/mise (managed external, verified)",
	"cargo":       "Rust build tool / package manager (self-provisioning via rustup)",
	"rustc":       "Rust compiler (self-provisioning via rustup)",
	"rustup":      "Rust toolchain manager (self-provisioning)",
	"rust":        "Rust compiler (self-provisioning; alias of rustc)",
	"java":        "self-provisioning Temurin JDK runtime (Adoptium, verified)",
	"javac":       "self-provisioning Temurin JDK compiler (Adoptium, verified)",
	"mvn":         "Apache Maven, auto-provisioned with its JDK (sha512-verified)",
	"git-scm":     "real git (git-for-windows MinGit on Windows; system git on unix), verified",
	"curl":        "curl (platform curl; pinned+verified curl.se/windows on a bare Windows node)",
	"kubectl":     "Kubernetes CLI for the DKS cluster (managed external, Apache-2.0)",
	"helm":        "Helm chart installer for the DKS cluster (managed external, Apache-2.0)",
	"dks":         "provision and manage the dedicated rootful DKS (k3s) machine",
	"sphere":      "tier-word spelling of `bashy peer` (the sphere tier's front door)",
	"peer":        "the sphere tier: peer-direct pooled p2p inference/compute across your own machines (via outpost)",
	"tessaro":     "Tessaro account: sign in/out, status, open the portal (via outpost)",
	"login":       "sign in to Tessaro — pair this machine with the portal",
}

// merge the declarative registry's synopses into verbSynopsis so registry CLIs
// (doctl, …) carry a synopsis in `bashy commands` without a hand-maintained line.
func init() {
	for _, e := range registry.All() {
		if verbSynopsis[e.Name] == "" {
			verbSynopsis[e.Name] = e.Synopsis
		}
	}
}

// commandsCatalog gathers the three command sources, each sorted: shell
// builtins, the coreutils userland, and the front-door verb shims (the
// agent-mode-only provisioners are included only in agent mode, mirroring the
// Preamble).
func commandsCatalog() (builtins, core, verbs []string) {
	builtins = interp.BuiltinNames()
	sort.Strings(builtins)
	core = tool.Names() // Names() already sorts; be defensive
	sort.Strings(core)
	verbs = append([]string{"docker", "sandbox"}, alwaysShimVerbs...)
	verbs = append(verbs, directFrontDoorVerbs...)
	verbs = append(verbs, agentModeShimVerbs...)
	verbs = append(verbs, registry.Names()...)  // declarative managed-external CLIs
	verbs = append(verbs, registeredNames()...) // the operator's ring (`commands add`)
	sort.Strings(verbs)
	// The curated (experimental) names are callable and shimmed exactly as
	// before; they are only kept out of what is TAUGHT. See curatedHiddenVerbs.
	core = withoutCuratedHidden(core)
	verbs = withoutCuratedHidden(verbs)
	return builtins, core, verbs
}

// supportedOn drops the names the atlas says goos cannot run (mkfifo on
// windows, ps on darwin). The DEFAULT listings are this host's — a listing
// that names a command that will fail is advertising a failure — while
// `bashy commands NAME` answers for every name and `--os any` lists all.
func supportedOn(goos string, names []string) []string {
	if goos == "any" {
		return names
	}
	out := names[:0:0]
	for _, n := range names {
		if e, ok := atlas.Lookup(n); ok && !e.SupportedOn(goos) {
			continue
		}
		out = append(out, n)
	}
	return out
}

// unsupportedHere returns the catalogued names the atlas says this host
// cannot run — the default listing's footer counts them.
func unsupportedHere(goos string) []string {
	var out []string
	_, core, verbs := commandsCatalog()
	for _, n := range append(core, verbs...) {
		if e, ok := atlas.Lookup(n); ok && !e.SupportedOn(goos) {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return slices.Compact(out)
}

func withoutCuratedHidden(names []string) []string {
	out := names[:0:0]
	for _, n := range names {
		if !isCuratedHidden(n) {
			out = append(out, n)
		}
	}
	return out
}

// hiddenVerbsCatalog is everything `--all` adds back: the compatibility
// aliases (no shim) and the curated experimental commands (shim kept). Both
// verbs and in-process tools; atlasCatalog tells them apart by the tool
// registry.
func hiddenVerbsCatalog() []string {
	verbs := append([]string(nil), hiddenFrontDoorVerbs...)
	verbs = append(verbs, curatedHiddenVerbs...)
	verbs = append(verbs, registeredHiddenNames()...)
	sort.Strings(verbs)
	return verbs
}

// synopsisOf is the one synopsis lookup every listing path uses: an
// in-process tool's own, a verb's from the static table, else a registered
// record's — the ring is data, so its prose cannot live in a Go map.
func synopsisOf(name string) string {
	if t := tool.Lookup(name); t != nil && t.Synopsis != "" {
		return t.Synopsis
	}
	if s := verbSynopsis[name]; s != "" {
		return s
	}
	if r, ok := registeredLookup(name); ok {
		return r.Synopsis
	}
	return ""
}

// printCommandSynopses prints "name — synopsis" lines under a titled header,
// names left-aligned to a common width for scannability.
func printCommandSynopses(w io.Writer, title string, names []string, syn func(string) string) {
	fmt.Fprintf(w, "%s (%d):\n", title, len(names))
	width := 0
	for _, n := range names {
		if len(n) > width {
			width = len(n)
		}
	}
	for _, n := range names {
		if s := syn(n); s != "" {
			fmt.Fprintf(w, "  %-*s  %s\n", width, n, s)
		} else {
			fmt.Fprintf(w, "  %s\n", n)
		}
	}
	fmt.Fprintln(w)
}

// printCommandGroup prints a titled, count-prefixed, wrapped column block.
func printCommandGroup(w io.Writer, title string, names []string) {
	fmt.Fprintf(w, "%s (%d):\n", title, len(names))
	const width = 78
	line := "  "
	for _, n := range names {
		if len(line)+len(n)+1 > width && line != "  " {
			fmt.Fprintln(w, strings.TrimRight(line, " "))
			line = "  "
		}
		line += n + " "
	}
	if strings.TrimSpace(line) != "" {
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
	fmt.Fprintln(w)
}

type gnuCoreutilsSummary struct {
	UpstreamCommands         int `json:"upstream_commands"`
	BashyNativeCommands      int `json:"bashy_native_commands"`
	MissingCommands          int `json:"missing_commands"`
	CoveredByBashBuiltins    int `json:"covered_by_bash_builtins"`
	Not100Conformant         int `json:"not_100_conformant"`
	NonGNUExtras             int `json:"non_gnu_extras"`
	CertifiedFullyConformant int `json:"certified_fully_conformant"`
}

type gnuCoreutilsGap struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

type gnuCoreutilsInventory struct {
	Summary           gnuCoreutilsSummary `json:"summary"`
	Upstream          []string            `json:"upstream"`
	BashyNative       []string            `json:"bashy_native"`
	Missing           []string            `json:"missing"`
	CoveredByBuiltins []string            `json:"covered_by_bash_builtins"`
	Not100Conformant  []gnuCoreutilsGap   `json:"not_100_conformant"`
	NonGNUExtras      []string            `json:"non_gnu_extras"`
}

func gnuCoreutilsReport(core, builtins []string) gnuCoreutilsInventory {
	upstream := atlas.GNUCoreutilsUpstream() // sorted
	coreSet := sliceSet(core)
	builtinSet := sliceSet(builtins)
	certified := sliceSet(gnuCoreutilsFullyConformant)

	var native, missing, covered []string
	var not100 []gnuCoreutilsGap
	for _, name := range upstream {
		switch {
		case coreSet[name]:
			native = append(native, name)
			if !certified[name] {
				not100 = append(not100, gnuCoreutilsGap{
					Name:   name,
					Status: "unverified",
					Reason: "bashy implements this GNU command name, but no GNU coreutils option/behavior conformance certification is recorded yet",
				})
			}
		case builtinSet[name]:
			covered = append(covered, name)
		default:
			missing = append(missing, name)
		}
	}

	upSet := sliceSet(upstream)
	var extras []string
	for _, name := range core {
		if !upSet[name] {
			extras = append(extras, name)
		}
	}
	sort.Strings(native)
	sort.Strings(missing)
	sort.Strings(covered)
	sort.Strings(extras)
	sort.Slice(not100, func(i, j int) bool { return not100[i].Name < not100[j].Name })

	return gnuCoreutilsInventory{
		Summary: gnuCoreutilsSummary{
			UpstreamCommands:         len(upstream),
			BashyNativeCommands:      len(native),
			MissingCommands:          len(missing),
			CoveredByBashBuiltins:    len(covered),
			Not100Conformant:         len(not100),
			NonGNUExtras:             len(extras),
			CertifiedFullyConformant: len(gnuCoreutilsFullyConformant),
		},
		Upstream:          upstream,
		BashyNative:       native,
		Missing:           missing,
		CoveredByBuiltins: covered,
		Not100Conformant:  not100,
		NonGNUExtras:      extras,
	}
}

func printGNUCoreutilsReport(w io.Writer, r gnuCoreutilsInventory) {
	fmt.Fprintf(w, "GNU coreutils parity (%d upstream):\n", r.Summary.UpstreamCommands)
	fmt.Fprintf(w, "  bashy native: %d\n", r.Summary.BashyNativeCommands)
	fmt.Fprintf(w, "  missing: %d\n", r.Summary.MissingCommands)
	fmt.Fprintf(w, "  covered by bash builtins: %d\n", r.Summary.CoveredByBashBuiltins)
	fmt.Fprintf(w, "  not 100%% conformant/certified: %d\n", r.Summary.Not100Conformant)
	fmt.Fprintf(w, "  non-GNU bashy extras: %d\n\n", r.Summary.NonGNUExtras)
	printCommandGroup(w, "GNU commands missing from bashy native coreutils", r.Missing)
	printCommandGroup(w, "GNU commands covered by bash builtins", r.CoveredByBuiltins)
	if len(r.Not100Conformant) > 0 {
		names := make([]string, 0, len(r.Not100Conformant))
		for _, gap := range r.Not100Conformant {
			names = append(names, gap.Name)
		}
		printCommandGroup(w, "GNU commands implemented but not yet certified 100% conformant", names)
	}
	printCommandGroup(w, "bashy coreutils extras outside GNU coreutils", r.NonGNUExtras)
}

// gnuCoreutilsFullyConformant records command names that have been certified
// against a GNU coreutils option/behavior harness. Keep this conservative: an
// empty list is better than claiming conformance without a reproducible score.
var gnuCoreutilsFullyConformant = []string{}
