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
	"sort"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/external/registry"
	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/coreutils/tool"
)

const commandsSchemaVersion = "bashy-commands-v1"

func dispatchCommands(args []string) int {
	asJSON, verbose := weavecli.IsAgent(), false // JSON by default under $BASHY_AGENTIC
	agentic, all, gnu, features := false, false, false, false
	var view, tierFilter, groupFilter, capFilter, effectFilter string
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
		switch a {
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
			fmt.Println("  --idioms       curated composites: commands naturally used together")
			fmt.Println("  --atlas        full per-command atlas records (machine surface)")
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
	if view != "" || tierFilter != "" || groupFilter != "" || capFilter != "" || effectFilter != "" || idioms || atlasFull {
		return dispatchAtlas(atlasRequest{
			view: view, tier: tierFilter, group: groupFilter, cap: capFilter, effect: effectFilter,
			idioms: idioms, full: atlasFull, asJSON: asJSON, all: all, verbose: verbose,
		})
	}

	if agentic {
		printAgenticCommands(os.Stdout)
		return 0
	}

	builtins, core, verbs := commandsCatalog()
	hidden := hiddenVerbsCatalog()
	gnuReport := gnuCoreutilsReport(core, builtins)
	if all {
		verbs = append(verbs, hidden...)
		sort.Strings(verbs)
	}
	if query != "" || features {
		info := commandFeatureReport(query, builtins, core, verbs, hidden, gnuReport)
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

	if asJSON {
		out := map[string]any{
			"schema_version": commandsSchemaVersion,
			"builtins":       builtins,
			"coreutils":      core,
			"verbs":          verbs,
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
				if s := verbSynopsis[n]; s != "" {
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
	printClassSections(os.Stdout, verbose, all)
	if gnu {
		printGNUCoreutilsReport(os.Stdout, gnuReport)
	}
	return 0
}

func printAgenticCommands(w io.Writer) {
	fmt.Fprint(w, `agentic bashy commands:
  bashy help dryrun              explain dry-run safety mode and JSON manifest
  bashy context --json           first-hop context: exact bashy path + capabilities
  BASHY_AGENTIC=1 bashy --dry-run script.sh
                                  preview external commands, rm, and truncation as JSON-lines
  bashy --dry-run -c 'commands'  human-readable dry-run preview
  bashy run --capture -- command structured command result envelope
  bashy run --check -- script.sh
                                  preflight a script, then run it with one JSON envelope
  bashy doctor                   diagnose PATH, shell, engine, and agent environment
  bashy check --agent --script script.sh
                                  JSON syntax + recursive command inventory preflight
  bashy self fetch               fetch/cache a released bashy binary
  bashy git ...                   embedded pure-Go git client
  bashy fetch --json URL          built-in URL/REST client with status envelope
  bashy commands -v              full command surface with synopses
  bashy commands grep --features  one-command resolver/capability/gap report
  bashy commands --view tier     the Command Atlas by execution tier; --atlas for records
  bashy commands --idioms        commands naturally used together (composites)
  bashy dag --list               list markdown DAG targets
  bashy graph impact SYMBOL      code-graph blast radius: what's coupled to a symbol
  bashy graph hotspots           most-connected symbols (refactor / orientation targets)
  bashy podman ...               Podman-compatible isolated container engine

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
		if s := verbSynopsis[name]; s != "" {
			out["synopsis"] = s
		}
		atlasFeatureFields(out, name, "verb", false)
	case containsString(hidden, name):
		out["class"], out["resolver"], out["available"], out["hidden"] = "verb", "bashy-front-door", true, true
		if s := verbSynopsis[name]; s != "" {
			out["synopsis"] = s
		}
		atlasFeatureFields(out, name, "verb", true)
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

func printCommandFeature(w io.Writer, info map[string]any) {
	fmt.Fprintf(w, "%s: %s via %s\n", info["name"], info["class"], info["resolver"])
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
	i := sort.SearchStrings(items, want)
	return i < len(items) && items[i] == want
}

// verbSynopsis describes the front-door verb shims (the coreutils tools carry
// their own Synopsis; builtins are standard). Brand-neutral, one line each.
var verbSynopsis = map[string]string{
	"docker":     "alias for `bashy podman` (isolated in-process container engine)",
	"podman":     "embedded, isolated in-process container engine",
	"ollama":     "managed local LLM runtime (isolated daemon, own port/models)",
	"weave":      "per-repo multi-agent workspace orchestrator",
	"sprint":     "cross-repo plan/continuity board (peer to weave)",
	"chat":       "invoke an agent with a single unattended instruction",
	"meet":       "multi-participant deliberation session with a notes-only secretary",
	"fanout":     "run parallel agents against one shared context (the blackboard pattern)",
	"supervise":  "drive a fleet against a goal of gated tasks, judged by a supervisor (conductor-as-a-verb)",
	"capability": "living agent (tool:model) × capability matrix for routing",
	"foreman":    "drive a persistent, steerable agent session (chat elevated)",
	"agent":      "agent identity and local agent helpers",
	"sdlc":       "route intake issues through agentic implementation and deployment gates",
	"web":        "web inspection helpers for SDLC verification",
	"dag":        "agent-first markdown DAG task runner",
	"schedule":   "modern cron: run a command on a cron/interval/at schedule",
	"secrets":    "managed API-key/token vault for the shell",
	"skills":     "tier-2 workspace skills, env-gated: list applicable here, probe the coordinate, show one",
	"kb":         "host-shared knowledge base: search before a task, add/retro after (all agents, all repos)",
	"tools":      "the agentic CLI harnesses this host can drive (claude, codex, opencode, ...)",
	"models":     "the inference backends the fleet can bind to (subscription, api, local)",
	"agents":     "named tool:model bindings — the enlistable unit; one agent, many nicknames",
	"people":     "human principals — who the names in prose refer to",
	"whois":      "resolve any name (person/agent/tool/model/host) and say how to reach it",
	"run":        "run a command, emit a structured result envelope (+advisor hints)",
	"commands":   "list the supported command surface (builtins, coreutils, verbs)",
	"context":    "print first-hop agent context: exact bashy path and capabilities",
	"doctor":     "diagnose the bashy environment (PATH/sh, engine, agent mode, bin cache)",
	"audit":      "the tamper-evident command audit trail: status/tail/verify/export",
	"check":      "statically check shell scripts for bashy/system command closure",
	"verify":     "run formal test batteries: compat/conformance/compliance/benchmark",
	"self":       "fetch/cache/install a released bashy binary",
	"bootstrap":  "hidden alias for bashy self",
	"upgrade":    "hidden alias for bashy self",
	"git":        "real git (git-for-windows MinGit on Windows; system git on unix), verified; `outpost git` is the pure-Go bootstrap client",
	"gh":         "GitHub CLI (managed external)",
	"act":        "run GitHub Actions locally (managed external)",
	"act-runner": "Gitea CI daemon; register --sandbox + daemon --docker-host = tier-3 sandbox executor (managed external)",
	"rclone":     "cloud-storage transfer + file server (managed external)",
	"mirror":     "continuous one-way directory mirror (managed external)",
	"loom":       "Gitea git forge (managed external)",
	"zot":        "OCI registry for images + models (managed external)",
	"seaweedfs":  "object/blob store with S3 gateway (managed external)",
	"kopia":      "snapshot-backup repository server (managed external)",
	"go":         "self-provisioning Go toolchain (download → verify → cache → exec)",
	"cmake":      "self-provisioning CMake build toolchain",
	"clang":      "self-provisioning clang/LLVM toolchain",
	"node":       "self-provisioning Node.js runtime (download from nodejs.org → verify → cache → exec)",
	"npm":        "Node.js package manager (from the provisioned Node tree)",
	"npx":        "Node.js package runner (from the provisioned Node tree)",
	"pnpm":       "pnpm package manager via the Node-bundled corepack (self-provisioning)",
	"yarn":       "yarn package manager via the Node-bundled corepack (self-provisioning)",
	"python":     "self-provisioning Python via astral uv (download → verify → cache → exec)",
	"pip":        "Python package installer via uv (self-provisioning)",
	"uv":         "uv — Python package/project manager (managed external, verified)",
	"mise":       "polyglot runtime/version manager jdx/mise (managed external, verified)",
	"cargo":      "Rust build tool / package manager (self-provisioning via rustup)",
	"rustc":      "Rust compiler (self-provisioning via rustup)",
	"rustup":     "Rust toolchain manager (self-provisioning)",
	"rust":       "Rust compiler (self-provisioning; alias of rustc)",
	"java":       "self-provisioning Temurin JDK runtime (Adoptium, verified)",
	"javac":      "self-provisioning Temurin JDK compiler (Adoptium, verified)",
	"mvn":        "Apache Maven, auto-provisioned with its JDK (sha512-verified)",
	"git-scm":    "real git (git-for-windows MinGit on Windows; system git on unix), verified",
	"curl":       "curl (platform curl; pinned+verified curl.se/windows on a bare Windows node)",
	"kubectl":    "Kubernetes CLI for the DKS cluster (managed external, Apache-2.0)",
	"helm":       "Helm chart installer for the DKS cluster (managed external, Apache-2.0)",
	"sphere":     "peer-direct pooled p2p inference/compute — the sphere tier (via outpost)",
	"tessaro":    "Tessaro account: sign in/out, status, open the portal (via outpost)",
	"login":      "sign in to Tessaro — pair this machine with the portal",
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
	verbs = append([]string{"docker"}, alwaysShimVerbs...)
	verbs = append(verbs, agentModeShimVerbs...)
	verbs = append(verbs, registry.Names()...) // declarative managed-external CLIs
	sort.Strings(verbs)
	return builtins, core, verbs
}

func hiddenVerbsCatalog() []string {
	verbs := append([]string(nil), hiddenFrontDoorVerbs...)
	sort.Strings(verbs)
	return verbs
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
	upstream := append([]string(nil), gnuCoreutilsCommands...)
	sort.Strings(upstream)
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
