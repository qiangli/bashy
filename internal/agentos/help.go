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
	bashy context --json		show exact bashy path and agent capabilities
	bashy commands --agentic	show agent-oriented command discovery
	bashy check --agent --script X	validate script syntax and command closure as JSON
	bashy run --check --capture -- X	preflight a script, then run with one JSON envelope
	bashy commands grep --features	show one-command capability/gap report
	bashy commands --gnu		show GNU coreutils parity/gap inventory
	bashy doctor			diagnose shell/runtime environment
	bashy install-agent <agent>	wire claude/opencode/aider/... to use bashy as their shell
	bashy serve [socket]		warm session: reuse one process for repeated -c calls
	bashy self fetch		fetch/cache a released bashy binary
	bashy git --help		show embedded git subcommands
	bashy fetch --help		show built-in download/REST client
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
		printCommandsHelp(os.Stdout)
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
  bashy context --json
  BASHY_AGENTIC=1 bashy --dry-run script.sh
  bashy commands --agentic
  bashy check --agent --script script.sh
  bashy run --check --capture -- script.sh
  bashy commands grep --features
  bashy commands --gnu
  bashy commands --json --gnu
  bashy doctor
  bashy self fetch
  bashy git status
  bashy fetch --json https://example.com
`)
}

func printCommandsHelp(w io.Writer) {
	fmt.Fprint(w, `usage:
  bashy commands
  bashy commands -v
  bashy commands grep --features
  bashy commands --agentic
  bashy commands --gnu
  bashy commands --json --gnu

Intent:
  bashy commands is the command-surface map for humans and agents. Use it when
  you need to know what bashy can run internally before falling through to PATH.
  The shell and builtin layer is GNU Bash 5.3 compatible and POSIX conformant;
  --gnu reports only the separate native coreutils gap.

Modes:
  default        list shell builtins, bashy in-process coreutils, and front-door verbs
  -v             include one-line synopses for coreutils and bashy verbs
  COMMAND --features machine-readable one-command resolver/capability/gap report
  --agentic      compact agent guide for dry-run, run envelopes, doctor, git, fetch, dag
  --gnu          GNU coreutils parity inventory and gap scoreboard
  --json --gnu   machine-readable release metric for tracking coreutils gap closure

GNU parity fields:
  missing                 GNU command names not implemented in bashy native coreutils
  covered_by_bash_builtins GNU names already covered by Bash 5.3-compatible builtins
                           and not counted as coreutils gaps, e.g. kill/printf/test
  not_100_conformant      bashy native coreutils names without a recorded GNU
                           coreutils conformance certificate
  non_gnu_extras          useful bashy tools outside GNU coreutils, e.g. grep/sed/tree/ast

Notes:
  --gnu is intentionally conservative. A command listed as not_100_conformant
  may work well for common cases, but bashy's coreutils layer has not yet
  recorded a full GNU option/behavior conformance score for it. Bash builtins
  are treated as covered by the GNU Bash 5.3 compatibility and POSIX conformance
  suites. The goal is for each release to reduce missing and not_100_conformant
  coreutils counts.
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
