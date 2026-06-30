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
	asJSON := false
	for _, a := range args {
		switch a {
		case "--json":
			asJSON = true
		case "-h", "--help":
			fmt.Println("usage: commands [--json]")
			fmt.Println("List the supported command surface: shell builtins, the coreutils")
			fmt.Println("userland, and bashy's front-door verbs.")
			return 0
		default:
			fmt.Fprintf(os.Stderr, "commands: unknown option %q\n", a)
			return 2
		}
	}

	builtins, core, verbs := commandsCatalog()

	if asJSON {
		b, _ := json.Marshal(map[string]any{
			"schema_version": commandsSchemaVersion,
			"builtins":       builtins,
			"coreutils":      core,
			"verbs":          verbs,
		})
		fmt.Println(string(b))
		return 0
	}

	printCommandGroup(os.Stdout, "shell builtins", builtins)
	printCommandGroup(os.Stdout, "coreutils userland", core)
	printCommandGroup(os.Stdout, "bashy verbs (front-door, bare-name shims)", verbs)
	return 0
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
