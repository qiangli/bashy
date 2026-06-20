module github.com/qiangli/bashy

go 1.25.0

require (
	github.com/qiangli/coreutils v0.0.0
	golang.org/x/term v0.44.0
	mvdan.cc/sh/v3 v3.13.1
)

require (
	github.com/creack/pty/v2 v2.0.1 // indirect
	github.com/ergochat/readline v0.1.3 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/odvcencio/gotreesitter v0.16.0 // indirect
	github.com/spf13/cobra v1.10.2 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	golang.org/x/sys v0.46.0 // indirect
	golang.org/x/text v0.36.0 // indirect
)

// Bashy is built on the qiangli/sh fork of mvdan.cc/sh (the one carrying the
// unmerged Bash 5.3 interp patches). Resolve it as a flat sibling: inside the
// dhnt umbrella ../sh is the sh submodule; in a standalone clone, clone
// github.com/qiangli/sh next to this repo as ./sh.
replace mvdan.cc/sh/v3 => ../sh

// The coreutils hub supplies the AgentOS pure-Go userland + `yc` verbs that
// the `bashy` binary injects as in-process commands. Same flat-sibling rule
// as ../sh: the coreutils submodule inside the dhnt umbrella, or a sibling
// clone of github.com/qiangli/coreutils standalone.
replace github.com/qiangli/coreutils => ../coreutils

replace github.com/ergochat/readline => ../readline
