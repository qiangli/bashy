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

	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/coreutils/tool"
)

const commandsSchemaVersion = "bashy-commands-v1"

func dispatchCommands(args []string) int {
	asJSON, verbose := weavecli.IsAgent(), false // JSON by default under $BASHY_AGENTIC
	agentic, all := false, false
	for _, a := range args {
		switch a {
		case "--json", "--json=true":
			asJSON = true
		case "--json=false", "--plain":
			asJSON = false
		case "--agentic":
			agentic = true
		case "--all":
			all = true
		case "-v", "--verbose":
			verbose = true
		case "-h", "--help":
			fmt.Println("usage: commands [-v] [--json|--plain|--agentic|--all]")
			fmt.Println("List the supported command surface: shell builtins, the coreutils")
			fmt.Println("userland, and bashy's front-door verbs.")
			fmt.Println("  -v             also show each coreutils tool's and verb's synopsis")
			fmt.Println("  --json         machine-readable (default under $BASHY_AGENTIC)")
			fmt.Println("  --json=false   force text even under $BASHY_AGENTIC (alias --plain)")
			fmt.Println("  --agentic      compact agent-oriented discovery and safety guide")
			fmt.Println("  --all          include hidden compatibility aliases")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "commands: unknown option %q\n", a)
			return 2
		}
	}

	if agentic {
		printAgenticCommands(os.Stdout)
		return 0
	}

	builtins, core, verbs := commandsCatalog()
	hidden := hiddenVerbsCatalog()
	if all {
		verbs = append(verbs, hidden...)
		sort.Strings(verbs)
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
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
		return 0
	}

	if verbose {
		// Builtins stay a compact list — they're standard and `help <name>`
		// describes them; the descriptive value is in the coreutils + verb sets.
		printCommandGroup(os.Stdout, "shell builtins — standard; `help <name>` for details", builtins)
		printCommandSynopses(os.Stdout, "coreutils userland", core, func(n string) string {
			if t := tool.Lookup(n); t != nil {
				return t.Synopsis
			}
			return ""
		})
		printCommandSynopses(os.Stdout, "bashy verbs (front-door, bare-name shims)", verbs, func(n string) string {
			return verbSynopsis[n]
		})
		return 0
	}

	printCommandGroup(os.Stdout, "shell builtins", builtins)
	printCommandGroup(os.Stdout, "coreutils userland", core)
	printCommandGroup(os.Stdout, "bashy verbs (front-door, bare-name shims)", verbs)
	return 0
}

func printAgenticCommands(w io.Writer) {
	fmt.Fprint(w, `agentic bashy commands:
  bashy help dryrun              explain dry-run safety mode and JSON manifest
  BASHY_AGENTIC=1 bashy --dry-run script.sh
                                  preview external commands, rm, and truncation as JSON-lines
  bashy --dry-run -c 'commands'  human-readable dry-run preview
  bashy run --capture -- command structured command result envelope
  bashy doctor                   diagnose PATH, shell, engine, and agent environment
  bashy self fetch               fetch/cache a released bashy binary
  bashy git ...                   embedded pure-Go git client
  bashy fetch --json URL          built-in URL/REST client with status envelope
  bashy commands -v              full command surface with synopses
  bashy dag --list               list markdown DAG targets
  bashy podman ...               Podman-compatible isolated container engine

dry-run JSON entry kinds:
  command   external command availability and resolved path
  destroy   destructive rm target count, bytes, and sample paths
  truncate  redirection clobber of an existing file
`)
}

// verbSynopsis describes the front-door verb shims (the coreutils tools carry
// their own Synopsis; builtins are standard). Brand-neutral, one line each.
var verbSynopsis = map[string]string{
	"docker":    "alias for `bashy podman` (isolated in-process container engine)",
	"podman":    "embedded, isolated in-process container engine",
	"ollama":    "managed local LLM runtime (isolated daemon, own port/models)",
	"weave":     "per-repo multi-agent workspace orchestrator",
	"sprint":    "cross-repo plan/continuity board (peer to weave)",
	"dag":       "agent-first markdown DAG task runner",
	"schedule":  "modern cron: run a command on a cron/interval/at schedule",
	"secrets":   "managed API-key/token vault for the shell",
	"skills":    "list/show the embedded tier-2 workspace skills",
	"run":       "run a command, emit a structured result envelope (+advisor hints)",
	"commands":  "list the supported command surface (builtins, coreutils, verbs)",
	"doctor":    "diagnose the bashy environment (PATH/sh, engine, agent mode, bin cache)",
	"check":     "statically check shell scripts for bashy/system command closure",
	"self":      "fetch/cache/install a released bashy binary",
	"bootstrap": "hidden alias for bashy self",
	"upgrade":   "hidden alias for bashy self",
	"git":       "embedded pure-Go git client (clone, status, commit, push, diff, log)",
	"gh":        "GitHub CLI (managed external)",
	"act":       "run GitHub Actions locally (managed external)",
	"rclone":    "cloud-storage transfer + file server (managed external)",
	"mirror":    "continuous one-way directory mirror (managed external)",
	"loom":      "Gitea git forge (managed external)",
	"zot":       "OCI registry for images + models (managed external)",
	"seaweedfs": "object/blob store with S3 gateway (managed external)",
	"kopia":     "snapshot-backup repository server (managed external)",
	"go":        "self-provisioning Go toolchain (download → verify → cache → exec)",
	"cmake":     "self-provisioning CMake build toolchain",
	"clang":     "self-provisioning clang/LLVM toolchain",
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
	if weavecli.IsAgent() {
		verbs = append(verbs, agentModeShimVerbs...)
	}
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
