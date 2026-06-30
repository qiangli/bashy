// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"io"
	"os"
)

// Usage returns bashy-only help appended to the GNU-compatible shell usage.
func Usage() string {
	return `
Bashy AgentOS extensions:
	--dryrun, --dry-run	preview external commands and destructive file ops
	BASHY_AGENTIC=1		emit agent-readable JSON-lines for supported features

Bashy front-door help:
	bashy help dryrun		show dry-run examples and JSON manifest fields
	bashy commands --agentic	show agent-oriented command discovery
	bashy doctor			diagnose shell/runtime environment
	bashy git --help		show embedded git subcommands
`
}

func dispatchHelp(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" {
		printGeneralHelp(os.Stdout)
		return 0
	}
	switch args[0] {
	case "dryrun", "dry-run", "--dryrun", "--dry-run":
		printDryRunHelp(os.Stdout)
		return 0
	case "commands":
		fmt.Fprintln(os.Stdout, "usage: bashy commands [-v] [--json|--plain|--agentic]")
		fmt.Fprintln(os.Stdout, "Use `bashy commands --agentic` for agent-oriented discovery.")
		return 0
	default:
		fmt.Fprintf(os.Stderr, "bashy help: unknown topic %q\n", args[0])
		fmt.Fprintln(os.Stderr, "known topics: dryrun, commands")
		return 2
	}
}

func printGeneralHelp(w io.Writer) {
	fmt.Fprint(w, `usage: bashy help [topic]

Topics:
  dryrun    preview commands and destructive file operations without effects
  commands  discover bashy command surfaces

Common agent entry points:
  bashy --dry-run -c 'rm -rf build'
  BASHY_AGENTIC=1 bashy --dry-run script.sh
  bashy commands --agentic
  bashy doctor
  bashy git status
`)
}

func printDryRunHelp(w io.Writer) {
	fmt.Fprint(w, `usage:
  bashy --dryrun script.sh
  bashy --dry-run script.sh
  bashy --dryrun -c 'commands'
  bashy --dry-run -c 'commands'

Runtime toggle:
  set -o dryrun
  rm -rf build
  set +o dryrun

Human mode:
  Prints external commands without running them and reports destructive rm or
  truncating redirections. Builtins, assignments, cd, and expansions still run.

Agent JSON-lines mode:
  BASHY_AGENTIC=1 bashy --dry-run script.sh

Manifest entries:
  {"kind":"command","command":"go","available":true,"resolved":"...","args":["go","test"]}
  {"kind":"command","command":"docker","available":false,"args":["docker","build","."]}
  {"kind":"destroy","op":"rm","recursive":true,"paths":["build"],"files":12,"bytes":4096}
  {"kind":"truncate","path":"config.yaml","bytes":4096}

Notes:
  --dryrun and --dry-run are aliases.
  The feature is bashy-only and inert under --posix.
  Dry-run is linear-path accurate: skipped external commands return success.
`)
}
