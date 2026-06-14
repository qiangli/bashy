module github.com/qiangli/bashy

go 1.25.0

require (
	golang.org/x/term v0.41.0
	mvdan.cc/sh/v3 v3.13.1
)

require (
	github.com/ergochat/readline v0.1.3 // indirect
	golang.org/x/sys v0.42.0 // indirect
	golang.org/x/text v0.9.0 // indirect
)

// Bashy is built on the qiangli/sh fork of mvdan.cc/sh (the one carrying the
// unmerged Bash 5.3 interp patches). Resolve it as a flat sibling: inside the
// dhnt umbrella ../sh is the sh submodule; in a standalone clone, clone
// github.com/qiangli/sh next to this repo as ./sh.
replace mvdan.cc/sh/v3 => ../sh
