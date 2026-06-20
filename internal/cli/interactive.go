// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package cli

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interactive"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// ignoreEOFLimit decodes the IGNOREEOF environment variable into the
// number of *additional* EOFs to tolerate before exiting. An unset
// or empty value disables the feature (ok=false). A non-numeric value
// behaves like bash's documented default of 10.
func ignoreEOFLimit(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n < 0 {
			n = 0
		}
		return n, true
	}
	return 10, true
}

// setHistCmd publishes the interactive command counter as $HISTCMD.
// Bash only sets HISTCMD when history is enabled (interactive mode),
// so we update it here rather than in lookupVar.
func setHistCmd(r *interp.Runner, n int) {
	if r.Vars == nil {
		r.Vars = make(map[string]expand.Variable)
	}
	r.Vars["HISTCMD"] = expand.Variable{
		Set:  true,
		Kind: expand.String,
		Str:  strconv.Itoa(n),
	}
}

// singleQuote wraps s as a single POSIX shell word with no expansion, escaping
// embedded single quotes the standard '\'' way.
func singleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func runInteractive(r *interp.Runner, stdin *os.File, stdout, stderr io.Writer) error {
	// Always bash grammar (drop-in); --posix applies POSIX *behavioral* parse
	// rules via PosixMode, not the stricter LangPOSIX grammar that would drop
	// bash extensions (arrays, ${v:off:len}, ${v^^}). See run() in main.go.
	lang := syntax.LangBash
	posixMode := *posix

	var cmdNum int
	getPrompt := func(ps string) string {
		defaultPS := `\u@\h:\w\$ `
		if ps == "PS2" {
			defaultPS = "> "
		}
		// Read PS1/PS2 from the LIVE variable scope, not r.Env: r.Env holds
		// only the initial environment, so an in-session `PS1=...` (re)assignment
		// would otherwise never reach the prompt. POSIX behavior #29 (parameter
		// expansion on PS1/PS2 in posix mode) depends on this too.
		val := r.LiveVar(ps).String()
		if val == "" {
			val = defaultPS
		}
		envGet := func(name string) string { return r.LiveVar(name).String() }
		return expandPrompt(val, envGet, cmdNum, cmdNum, *posix)
	}

	histFile := r.Env.Get("HISTFILE").String()
	if histFile == "" {
		// Bash assigns a default to $HISTFILE: ~/.sh_history in POSIX mode
		// (behavior #30), ~/.bash_history otherwise. bashy keeps its own
		// ~/.bashy_history name for the non-posix default but matches the
		// required ~/.sh_history under --posix.
		name := ".bashy_history"
		if *posix {
			name = ".sh_history"
		}
		if home, _ := os.UserHomeDir(); home != "" {
			histFile = filepath.Join(home, name)
		}
		// Expose the default to scripts as a (non-exported) shell variable, as
		// bash does, by running a real assignment through the runner: r.Vars is
		// only an output mirror (cleared at Reset), so the value must land in
		// the live variable scope to be visible to ${HISTFILE}.
		if histFile != "" {
			assign := "HISTFILE=" + singleQuote(histFile)
			p := syntax.NewParser(syntax.Variant(lang), syntax.PosixMode(posixMode))
			if prog, perr := p.Parse(strings.NewReader(assign), "histfile-init"); perr == nil {
				_ = r.Run(context.Background(), prog)
			}
		}
	}

	var eofPresses int
	return interactive.Run(context.Background(), interactive.Options{
		Runner:            r,
		Lang:              lang,
		PosixMode:         posixMode,
		Stdin:             stdin,
		Stdout:            stdout,
		Stderr:            stderr,
		PS1:               func() string { return getPrompt("PS1") },
		PS2:               func() string { return getPrompt("PS2") },
		VimMode:           r.VimMode, // `set -o vi` switches to vi editing (re-read each prompt)
		HistoryFile:       histFile,
		HistoryLimit:      1000,
		HistorySearchFold: true,
		InterruptPrompt:   "^C",
		EOFPrompt:         "exit",
		PreCommand: func(ctx context.Context, r *interp.Runner) {
			if pc := r.Env.Get("PROMPT_COMMAND").String(); pc != "" {
				pcp := syntax.NewParser(syntax.Variant(lang), syntax.PosixMode(posixMode))
				if prog, err := pcp.Parse(strings.NewReader(pc), "PROMPT_COMMAND"); err == nil {
					_ = r.Run(ctx, prog)
				}
			}
			eofPresses = 0
			cmdNum++
			setHistCmd(r, cmdNum)
		},
		// IGNOREEOF asks the shell to tolerate N additional Ctrl-D
		// presses before actually exiting on EOF. Returning true here
		// keeps the interactive loop running; false (default) lets it
		// exit on the first EOF as bash does without IGNOREEOF.
		OnEOF: func() bool {
			limit, ok := ignoreEOFLimit(r.Env.Get("IGNOREEOF").String())
			if !ok || eofPresses >= limit {
				return false
			}
			eofPresses++
			_, _ = io.WriteString(stderr, "Use \"exit\" to leave the shell.\n")
			return true
		},
		OnRunError: func(err error) {
			_, _ = io.WriteString(stderr, "bashy: "+err.Error()+"\n")
		},
	})
}
