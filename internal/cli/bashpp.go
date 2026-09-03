// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package cli

import (
	"fmt"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// Sprint 98, Story #125 (B1): the resolved Bash++ dialect/selector model.
//
// This file is a self-contained preparation slice. It is not called from
// main.go or anywhere else in this package — activation is held for the
// parser/runtime integration story (B2/B3, #126) once bashpp_test.go's
// callers are the only callers. Nothing here changes what any existing
// invocation does.
//
// # Parser entry-path audit
//
// Every site in this package that currently builds a [syntax.Parser] or
// otherwise pins a [syntax.LangVariant], and whether it is a candidate for
// Bash++ dialect wiring once B1 is activated:
//
//   - main.go, run() (lang := syntax.LangBash/LangPOSIX, ~line 2529) plus its
//     parseOpts/bashyParseOpts closure (~2464) and the statement-recovery
//     loop's parseOnce (~2654) and the direct -c parse (~2708). This is the
//     primary script/-c/stdin execution path and the intended B2 integration
//     point — the one place this story's resolver is built for but does not
//     yet reach.
//   - main.go, bashyParseOpts (~2464): translates a requested LangPOSIX into
//     LangBash+PosixMode so `--posix` keeps bash grammar. A resolved Bash++
//     selector composes with this (LangBashPP+PosixMode), it does not bypass
//     it — see [BashPPResolution.LangVariant].
//   - interactive.go, runInteractive (~62, 105, 141): the readline-backed
//     interactive REPL. Delegates per-line parsing to
//     mvdan.cc/sh/v3/interactive with a fixed Lang for the whole session,
//     matching the design doc's "selected before the file is parsed" rule
//     for interactively typed input too.
//   - forced_interactive.go, runForcedInteractiveExec (~224): the non-TTY
//     `bash -i` emulation; same session-wide Lang shape as the interactive
//     REPL. runnerExpand (~191) parses a synthetic `${...}` snippet for
//     prompt/HISTFILE bookkeeping, not user source, and is expected to stay
//     plain LangBash regardless of the resolved dialect.
//   - session.go, RunSessionCommand (~98): the live-session socket command
//     path (the agentic surface's warm-session analogue of -c).
//   - main.go, completeStmtBeforeLine (~3994): a diagnostic-only re-parse
//     used solely to shape bash-format parse-error output; intentionally
//     bash-fixed, not a candidate for dialect wiring.
//   - main.go, registerDefaultFuncs (~831) and importBashFuncs (~873): parse
//     bashy-authored preamble source and inherited BASH_FUNC_* environment
//     functions respectively. Both are always plain Bash by construction —
//     not user Bash++ source — and stay out of scope.
//   - main.go, the BASH_EXECUTION_STRING bookkeeping assignment (~2621): an
//     internal synthetic assignment, not user source.
//
// # Precedence
//
// explicit CLI > environment > .bpp extension > binary default
//
// each tier is winner-take-all: the first tier that expresses an opinion
// decides the result and lower tiers are not consulted.

// BashPPBinary identifies which of the two compiled entry points
// ([BashPPBinaryBash], cmd/bash's pure Bash 5.3 drop-in, or
// [BashPPBinaryBashy], cmd/bashy's AgentOS shell) is resolving the dialect.
// It is the "binary default" tier and also the certification-refusal gate:
// only the bash binary is the certification profile.
type BashPPBinary string

const (
	BashPPBinaryBash  BashPPBinary = "bash"
	BashPPBinaryBashy BashPPBinary = "bashy"
)

// bashPPBinaryDefault is the last-resort tier: Bash++ off unless the file
// looks like Bash++ or something asked for it, except on bashy, whose
// product default is Bash++ on (independently overridable, same as its
// agentic surface).
func (b BashPPBinary) bashPPDefault() bool {
	return b == BashPPBinaryBashy
}

// BashPPSource names which precedence tier decided a [BashPPResolution].
type BashPPSource string

const (
	BashPPSourceCLI           BashPPSource = "cli"            // --bashpp / --bash++ / --no-bashpp
	BashPPSourceEnv           BashPPSource = "env"            // BASHY_BASHPP=1|0
	BashPPSourceExtension     BashPPSource = "extension"      // .bpp
	BashPPSourceBinaryDefault BashPPSource = "binary-default" // bash off, bashy on
)

// Explicit reports whether the source is a deliberate user request (a CLI
// flag or an environment variable) rather than an inferred default (the file
// extension or the binary's product default).
func (s BashPPSource) Explicit() bool {
	return s == BashPPSourceCLI || s == BashPPSourceEnv
}

// BashPPSelector is the input to [ResolveBashPP]: everything the precedence
// chain reads to decide the initial grammar for one parse. It takes an
// argv-shaped slice and an env lookup func, rather than reading os.Args and
// os.Environ directly, so resolution stays a pure, independently testable
// function — main.go supplies the real values when B2 wires this in.
type BashPPSelector struct {
	// Binary is which compiled entry point is asking. Required.
	Binary BashPPBinary
	// Args is an os.Args-shaped slice (Args[0] is the program name, not
	// scanned). May be nil.
	Args []string
	// LookupEnv resolves an environment variable by name, in the shape of
	// os.LookupEnv. May be nil, meaning no environment tier.
	LookupEnv func(name string) (value string, ok bool)
	// Filename is the script path about to be parsed ("" for -c/stdin/
	// interactive input, which never carry a .bpp extension).
	Filename string
	// Posix is the already-resolved startup POSIX mode (effectiveStartupPosix
	// in main.go), not merely the --posix flag's literal presence.
	Posix bool
}

// BashPPResolution is the resolved dialect selector: whether Bash++ is on,
// and which precedence tier decided it.
type BashPPResolution struct {
	Enabled bool
	Source  BashPPSource
	Posix   bool
}

// ParserOptions composes the selected grammar with the POSIX semantic profile.
// Keeping both decisions in one API prevents callers from selecting LangBashPP
// while accidentally dropping syntax.PosixMode(true).
func (r BashPPResolution) ParserOptions(base syntax.LangVariant, extra ...syntax.ParserOption) []syntax.ParserOption {
	posix := r.Posix || base == syntax.LangPOSIX
	if base == syntax.LangPOSIX {
		base = syntax.LangBash
	}
	if r.Enabled {
		base = syntax.LangBashPP
	}
	return append([]syntax.ParserOption{syntax.Variant(base), syntax.PosixMode(posix)}, extra...)
}

// BashPPCertificationError reports that a selector resolved Bash++ on while
// also being the certification profile (the bash binary under --posix),
// which must fail clearly rather than silently leak extended grammar into a
// certification-labeled invocation.
type BashPPCertificationError struct {
	Source BashPPSource
}

func (e *BashPPCertificationError) Error() string {
	return fmt.Sprintf(
		"bashpp: --posix on the bash binary is the certification profile and must run with Bash++ off; "+
			"Bash++ was selected via %s — drop --posix, use the bashy binary, or add --no-bashpp",
		e.Source,
	)
}

// ResolveBashPP resolves the initial Bash++ dialect for one selector,
// applying the documented precedence (explicit CLI > environment > .bpp
// extension > binary default) and then the certification refusal: the bash
// binary under POSIX mode is the certification profile
// ("docs/bashpp-posix-superset-syntax.md" — certification uses the
// standalone bash binary with --posix and without a Bash++ selector), and an
// invocation that resolves Bash++ on there is refused rather than silently
// running with extended grammar under a certification-labeled flag. bashy
// under --posix is unaffected: `bashy --posix --bashpp` is a supported
// combined mode, since bashy is never the certification binary.
func ResolveBashPP(sel BashPPSelector) (BashPPResolution, error) {
	enabled, source := resolveBashPPTiers(sel)
	if sel.Binary == BashPPBinaryBash && sel.Posix && enabled {
		return BashPPResolution{}, &BashPPCertificationError{Source: source}
	}
	return BashPPResolution{Enabled: enabled, Source: source, Posix: sel.Posix}, nil
}

func resolveBashPPTiers(sel BashPPSelector) (bool, BashPPSource) {
	if enabled, seen := commandLineBashPP(sel.Args); seen {
		return enabled, BashPPSourceCLI
	}
	if enabled, seen := envBashPP(sel.LookupEnv); seen {
		return enabled, BashPPSourceEnv
	}
	if strings.HasSuffix(sel.Filename, ".bpp") {
		return true, BashPPSourceExtension
	}
	return sel.Binary.bashPPDefault(), BashPPSourceBinaryDefault
}

// commandLineBashPP resolves the last of --bashpp/--bash++/--no-bashpp on
// the command line, mirroring commandLinePosixMode's shape in main.go:
// scanning stops at "--", at "-c" (whose operand is a command string, not a
// further flag), or at the first operand that does not start with "-" (the
// script path, after which remaining words are script arguments). --bashpp
// and --bash++ are exact aliases per the design of record; neither creates a
// separate mode.
func commandLineBashPP(args []string) (enabled, seen bool) {
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--":
			return enabled, seen
		case "--bashpp", "--bash++":
			enabled, seen = true, true
		case "--no-bashpp":
			enabled, seen = false, true
		case "-c":
			return enabled, seen
		case "-o", "-O", "--rcfile", "--init-file", "-bashy-plus-o", "-bashy-plus-O":
			// These invocation options consume the following token. It is not
			// a script operand, so selectors after it must still be scanned.
			if i+1 < len(args) {
				i++
			}
		default:
			if !strings.HasPrefix(args[i], "-") {
				return enabled, seen
			}
		}
	}
	return enabled, seen
}

// envBashPP resolves BASHY_BASHPP=1|0. Any other value (unset, or set to
// something other than "1"/"0") is treated as this tier having no opinion,
// falling through to the next precedence tier, rather than as an error —
// this mirrors how the equivalent POSIXLY_CORRECT/SHELLOPTS checks in
// main.go treat presence, not spelling validation, as the signal.
func envBashPP(lookupEnv func(string) (string, bool)) (enabled, seen bool) {
	if lookupEnv == nil {
		return false, false
	}
	raw, ok := lookupEnv("BASHY_BASHPP")
	if !ok {
		return false, false
	}
	switch raw {
	case "1":
		return true, true
	case "0":
		return false, true
	default:
		return false, false
	}
}
