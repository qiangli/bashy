// Copyright (c) 2017, Daniel Martí <mvdan@mvdan.cc>
// See LICENSE for licensing information

package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"
)

// Sprint 118, W1 (Bashy half): explicit Go-source input dispatch.
//
// `bashy --bashpp --source=go original.go` runs an UNCHANGED Go program: the
// bytes are handed to the sh front end (mvdan.cc/sh/v3/gosource), which lexes,
// parses and type-checks them with Go's own rules and returns a positioned
// Bash++ AST. This file owns the selection, its refusals and the collection of
// input files. It deliberately contains no Go lexer, parser or type checker —
// reimplementing Go source parsing here is exactly what the sprint contract
// forbids.
//
// The load itself is reached through the [GoSourceLoad] hook, in the shape the
// package already uses for AgentOSDispatch: the pure `bash` drop-in leaves it
// nil (it structurally cannot link the AgentOS surface, and must not grow a Go
// type checker), and cmd/bashy wires it to internal/agentos. While the front
// end is absent the hook stays nil and every Go-source invocation fails with a
// Go-source diagnostic. It never falls back to the shell parser: that fallback
// is what made a malformed Go program look like a passing shell script.
//
// Design of record: docs/plan-source-go-dispatch.md.

// GoSourceFile is one original input file handed to the front end. Data is the
// file's exact bytes; nothing in this package rewrites, reformats, strips or
// appends to them.
type GoSourceFile struct {
	Name string
	Data []byte
}

// GoSourceOrigin describes where one original file landed in the loaded
// program's position space. It mirrors gosource.SourceInfo so a caller can map
// a position back to an original file without holding a live program.
type GoSourceOrigin struct {
	Name   string
	SHA256 string
	Base   uint
	Size   uint
}

// GoSourceOptions are the load-time knobs Bashy sets.
type GoSourceOptions struct {
	// RunMain requests the package's init functions and main entry calls.
	// Loading never executes anything either way; false is what makes
	// --check safe, since the resulting program carries no entry calls.
	RunMain bool
	// Dir is the directory the source came from, used to resolve module
	// imports. It is never the source identity — that stays the file name.
	Dir string
}

// GoSourceProgram is the loaded program: a positioned Bash++ AST plus the
// provenance needed to point a diagnostic or a source map back at the original
// bytes.
type GoSourceProgram struct {
	File          *syntax.File
	Package       string
	Main          string
	InitFunctions []string
	Origins       []GoSourceOrigin
	// FrontEnd is the front end's version string, recorded in evidence.
	FrontEnd string
}

// SourceAt maps a position in the loaded program back to the original file and
// the byte offset within that file. It is the same interval arithmetic the sh
// front end applies to its own Sources, kept here so the transpile source map
// can be produced from Origins alone.
func (p *GoSourceProgram) SourceAt(pos syntax.Pos) (string, uint, bool) {
	if p == nil {
		return "", 0, false
	}
	for _, o := range p.Origins {
		if pos.Offset() >= o.Base && pos.Offset() <= o.Base+o.Size {
			return o.Name, pos.Offset() - o.Base, true
		}
	}
	return "", 0, false
}

// GoSourceLoad is the injection point for the sh Go front end. It is nil in
// the pure bash drop-in, which structurally cannot link internal/agentos;
// cmd/bashy sets it there, unconditionally. A nil hook is a refusal, never a
// shell fallback.
var GoSourceLoad func(files []GoSourceFile, opts GoSourceOptions) (*GoSourceProgram, error)

// GoSourcePackageFiles selects the Go files of a directory recipe using the Go
// toolchain's own rules (`//go:build`, `// +build`, _GOOS/_GOARCH suffixes,
// the host build context). It is wired from internal/agentos for the same
// reason as GoSourceLoad: go/build belongs to the AgentOS half, not to the
// pure drop-in. Nil means this build cannot accept a directory recipe.
var GoSourcePackageFiles func(dir string) ([]string, error)

// GoSourceModuleDir carries the SOURCE's module directory to the runner,
// separately from the runner's working directory.
//
// It is wired to sh's `interp.GoSourceModuleDir(dir)` (landed in sh de4ff069)
// from internal/agentos, alongside the other Go-source hooks. It exists
// because the two directories are genuinely different: the harness runs every
// mode from a fresh runtime directory holding only the declared assets, while
// the Go source it names lives, with its `go.mod`, somewhere else. The
// interpreter resolves a Bash++ import by running `go list`; without this
// option it does so in the runner's working directory, so a module import
// resolved only when the two happened to coincide.
//
// Bashy deliberately does NOT close that gap by moving the program's working
// directory to the source: that would resolve the import and break every
// relative asset path in the same stroke, turning a loud failure into a wrong
// answer. The hook takes the same directory the type-checking half already
// resolves modules against through lower.NewModuleImporter, so both halves
// agree by construction. A nil hook (no Go front end linked) leaves the
// interpreter's previous cwd-relative behaviour untouched.
var GoSourceModuleDir func(dir string) interp.RunnerOption

// ErrGoSourceUnavailable reports that this build has no Go front end linked.
var ErrGoSourceUnavailable = errors.New(
	"the Go source front end (mvdan.cc/sh/v3/gosource) is not available in this build")

// GoSourceLanguage names an input language for --source.
type GoSourceLanguage string

const (
	// GoSourceLangShell is the default: the input is shell (or Bash++) source.
	GoSourceLangShell GoSourceLanguage = "sh"
	// GoSourceLangGo selects unchanged Go source input.
	GoSourceLangGo GoSourceLanguage = "go"
)

// GoSourceSelection is the raw, unvalidated result of scanning the command
// line: what the user spelled, before it is checked against the resolved
// dialect and POSIX profile.
type GoSourceSelection struct {
	// Language is the last --source value seen, "" when the flag is absent.
	Language string
	// LanguageSeen distinguishes an absent --source from --source=sh.
	LanguageSeen bool
	// Check records --check.
	Check bool
	// Files records every --go-file, in the order given. Bashy does not sort
	// them; the front end owns file ordering.
	Files []string
}

// Requested reports whether any flag in this group was spelled at all.
func (s GoSourceSelection) Requested() bool {
	return s.LanguageSeen || s.Check || len(s.Files) > 0
}

// GoSourceContext is everything resolution needs beyond the raw selection.
type GoSourceContext struct {
	// Binary is the entry point resolving the selection.
	Binary BashPPBinary
	// BashPP is the resolved Bash++ state, not merely the --bashpp flag.
	BashPP bool
	// Posix is the resolved startup POSIX profile, from any of its spellings.
	Posix bool
	// HasOperand reports whether a file/directory operand was given.
	HasOperand bool
	// ShellOnlyMode names a selected mode that only exists for shell input
	// (--pretty-print, --dump-strings, --dump-po-strings). It is refused
	// rather than ignored: those modes are implemented by constructing the
	// SHELL parser, so accepting one would judge Go bytes by shell syntax —
	// the exact confusion --source=go exists to end.
	ShellOnlyMode string
}

// GoSourceResolution is the validated selection.
type GoSourceResolution struct {
	// Enabled reports that the input is Go source.
	Enabled bool
	// Check requests semantic validation with no execution.
	Check bool
	// Files are the explicit --go-file inputs, empty for operand/stdin input.
	Files []string
}

// goSourceError is a refusal that the caller reports on stderr and turns into
// exit status 2, bash's usage-error status. It may wrap a sentinel so a caller
// can test the reason without matching on text.
type goSourceError struct {
	msg string
	err error
}

func (e *goSourceError) Error() string { return e.msg }
func (e *goSourceError) Unwrap() error { return e.err }

func goSourceErrorf(format string, args ...any) error {
	return &goSourceError{msg: fmt.Sprintf(format, args...)}
}

// IsGoSourceError reports whether err is a Go-source selection refusal.
func IsGoSourceError(err error) bool {
	var e *goSourceError
	return errors.As(err, &e)
}

// stripGoSourceInvocationFlags removes the Go-source selector flags from an
// argv-shaped slice and returns what they said. They are consumed here, before
// Go's flag package runs, for the same reason --bashpp is: they are invocation
// selectors, not shell options, and leaving them in argv would either make
// `flag` reject them or turn them into the script's positional arguments.
//
// Scanning follows commandLineBashPP's rule exactly: stop at `--`, at `-c`
// (whose operand is a command string, not a further flag), or at the first
// operand that does not start with `-`.
func stripGoSourceInvocationFlags(args []string) ([]string, GoSourceSelection, error) {
	out := make([]string, 0, len(args))
	var sel GoSourceSelection
	options := true
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if i == 0 {
			out = append(out, arg)
			continue
		}
		if options && (arg == "-c" || arg == "--" || arg == "-" || !strings.HasPrefix(arg, "-")) {
			options = false
		}
		if !options {
			out = append(out, arg)
			continue
		}
		switch {
		case arg == "--check":
			sel.Check = true
			continue
		case arg == "--source":
			value, ok := goSourceFlagValue(args, &i)
			if !ok {
				return nil, sel, goSourceErrorf("bashy: --source: missing argument")
			}
			sel.Language, sel.LanguageSeen = value, true
			continue
		case strings.HasPrefix(arg, "--source="):
			sel.Language, sel.LanguageSeen = strings.TrimPrefix(arg, "--source="), true
			continue
		case arg == "--go-file":
			value, ok := goSourceFlagValue(args, &i)
			if !ok {
				return nil, sel, goSourceErrorf("bashy: --go-file: missing argument")
			}
			sel.Files = append(sel.Files, value)
			continue
		case strings.HasPrefix(arg, "--go-file="):
			sel.Files = append(sel.Files, strings.TrimPrefix(arg, "--go-file="))
			continue
		}
		out = append(out, arg)
		// Another option's value is not an operand, so the scan must step
		// over it and keep looking for selectors behind it.
		if invocationFlagTakesValue(arg) && i+1 < len(args) {
			i++
			out = append(out, args[i])
		}
	}
	return out, sel, nil
}

// invocationFlagTakesValue reports whether an invocation option consumes the
// following argv token.
//
// Every scanner that walks the option prefix of argv — commandLineBashPP,
// stripBashPPInvocationFlags and stripGoSourceInvocationFlags — must agree on
// this set, because each of them stops at the first token that does not look
// like an option. When one scanner does not know that `--source` takes a
// value, it reads the value (`go`) as the script operand and stops, so every
// selector spelled after it is silently dropped: `bashy --source go --bashpp
// x.go` used to die with "flag provided but not defined: -bashpp". Sharing
// the set is what makes the separated and `=`-joined spellings equivalent in
// either order.
func invocationFlagTakesValue(arg string) bool {
	switch arg {
	case "-o", "-O", "--rcfile", "--init-file", "-bashy-plus-o", "-bashy-plus-O",
		"--source", "--go-file":
		return true
	}
	return false
}

// goSourceFlagValue consumes the separated value of a `--flag value` pair.
func goSourceFlagValue(args []string, i *int) (string, bool) {
	if *i+1 >= len(args) {
		return "", false
	}
	*i++
	return args[*i], true
}

// ResolveGoSource validates a raw selection against the resolved dialect and
// POSIX profile. Every refusal is deliberate and none of them degrade into
// shell dispatch.
//
// Go-source input lives strictly inside Bash++: Classic and POSIX activation
// semantics are untouched by this feature, so an invocation that does not
// spell --source=go reaches byte-identical code. POSIX mode is refused rather
// than ignored because --posix is a conformance selection — Sprint 114 made
// the bash drop-in's Bash++/POSIX combination inert precisely so a POSIX claim
// cannot be affected by an extension, and accepting Go input under it would
// reopen that. The bash drop-in refuses outright: cmd/bash cannot link the
// AgentOS hook that carries the front end, and must not grow a Go type checker.
func ResolveGoSource(sel GoSourceSelection, ctx GoSourceContext) (GoSourceResolution, error) {
	lang := GoSourceLangShell
	if sel.LanguageSeen {
		switch GoSourceLanguage(sel.Language) {
		case GoSourceLangShell, GoSourceLangGo:
			lang = GoSourceLanguage(sel.Language)
		default:
			return GoSourceResolution{}, goSourceErrorf(
				"bashy: --source: unknown input language %q (expected \"sh\" or \"go\")", sel.Language)
		}
	}
	if lang != GoSourceLangGo {
		if sel.Check {
			return GoSourceResolution{}, goSourceErrorf("bashy: --check requires --source=go")
		}
		if len(sel.Files) > 0 {
			return GoSourceResolution{}, goSourceErrorf("bashy: --go-file requires --source=go")
		}
		return GoSourceResolution{}, nil
	}
	// POSIX is checked before Bash++ so the conformance refusal is the one
	// reported: on the bash drop-in, --posix already forces the Bash++
	// selector inert, and "requires --bashpp" would be a misleading answer to
	// an invocation that spelled it.
	if ctx.Posix {
		return GoSourceResolution{}, goSourceErrorf("bashy: --source=go is not available in POSIX mode")
	}
	if !ctx.BashPP {
		return GoSourceResolution{}, goSourceErrorf("bashy: --source=go requires --bashpp")
	}
	if ctx.Binary != BashPPBinaryBashy {
		return GoSourceResolution{}, goSourceErrorf("bashy: --source=go requires the bashy front door")
	}
	if len(sel.Files) > 0 && ctx.HasOperand {
		return GoSourceResolution{}, goSourceErrorf(
			"bashy: --go-file cannot be combined with a file operand")
	}
	if ctx.ShellOnlyMode != "" {
		return GoSourceResolution{}, goSourceErrorf(
			"bashy: %s cannot be combined with --source=go", ctx.ShellOnlyMode)
	}
	return GoSourceResolution{Enabled: true, Check: sel.Check, Files: sel.Files}, nil
}

// GoSourceInput is a collected set of original files plus the directory used
// to resolve module imports.
type GoSourceInput struct {
	Files []GoSourceFile
	Dir   string
}

// CollectGoSources reads the original bytes for one invocation. It reads; it
// never edits. Callers pass exactly one of: explicit --go-file names, a file or
// directory operand, a command string, or a reader for stdin.
func CollectGoSources(res GoSourceResolution, operand, command string, stdin io.Reader) (GoSourceInput, error) {
	switch {
	case len(res.Files) > 0:
		return readGoSourceFiles(res.Files)
	case command != "":
		data, err := io.ReadAll(strings.NewReader(command))
		if err != nil {
			return GoSourceInput{}, err
		}
		return GoSourceInput{Files: []GoSourceFile{{Name: "-c", Data: data}}, Dir: goSourceWorkingDir()}, nil
	case operand != "" && operand != "-":
		info, err := os.Stat(operand)
		if err != nil {
			return GoSourceInput{}, err
		}
		if info.IsDir() {
			names, err := goSourcePackageFiles(operand)
			if err != nil {
				return GoSourceInput{}, err
			}
			in, err := readGoSourceFiles(names)
			if err != nil {
				return GoSourceInput{}, err
			}
			in.Dir = operand
			return in, nil
		}
		return readGoSourceFiles([]string{operand})
	default:
		if stdin == nil {
			return GoSourceInput{}, goSourceErrorf("bashy: --source=go: no input")
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return GoSourceInput{}, err
		}
		return GoSourceInput{Files: []GoSourceFile{{Name: "-", Data: data}}, Dir: goSourceWorkingDir()}, nil
	}
}

func goSourceWorkingDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	return dir
}

func readGoSourceFiles(names []string) (GoSourceInput, error) {
	in := GoSourceInput{Files: make([]GoSourceFile, 0, len(names))}
	for _, name := range names {
		data, err := os.ReadFile(name)
		if err != nil {
			return GoSourceInput{}, err
		}
		in.Files = append(in.Files, GoSourceFile{Name: name, Data: data})
	}
	if len(in.Files) > 0 {
		in.Dir = filepath.Dir(in.Files[0].Name)
	}
	return in, nil
}

// goSourcePackageFiles selects a directory recipe's Go files through the
// [GoSourcePackageFiles] hook, i.e. through go/build in internal/agentos.
//
// The selection is the Go toolchain's own: `//go:build` lines, `// +build`
// comments and _GOOS/_GOARCH filename suffixes all apply, against the host
// build context. That is required for `bashy --bashpp --source=go ./pkg` to
// mean what `go build ./pkg` means on the same host — a package carrying
// main_darwin.go and main_linux.go builds natively either way, and collecting
// both would reject it as a duplicate main.
//
// Explicit --go-file recipes deliberately keep their bypass: a caller that
// names the files has already made the selection (the Tour's OMIT programs
// are passed that way by their own oracle).
func goSourcePackageFiles(dir string) ([]string, error) {
	if GoSourcePackageFiles == nil {
		return nil, &goSourceError{
			msg: "bashy: --source=go: " + ErrGoSourceUnavailable.Error(),
			err: ErrGoSourceUnavailable,
		}
	}
	names, err := GoSourcePackageFiles(dir)
	if err != nil {
		if IsGoSourceError(err) {
			return nil, err
		}
		return nil, goSourceErrorf("bashy: --source=go: %v", err)
	}
	if len(names) == 0 {
		return nil, goSourceErrorf("bashy: --source=go: no Go source files in %s", dir)
	}
	return names, nil
}

// LoadGoSource reaches the front end. A nil hook is reported as a Go-source
// diagnostic, so an unsupported build refuses rather than silently handing the
// bytes to the shell parser.
func LoadGoSource(in GoSourceInput, opts GoSourceOptions) (*GoSourceProgram, error) {
	if GoSourceLoad == nil {
		return nil, &goSourceError{
			msg: "bashy: --source=go: " + ErrGoSourceUnavailable.Error(),
			err: ErrGoSourceUnavailable,
		}
	}
	if len(in.Files) == 0 {
		return nil, goSourceErrorf("bashy: --source=go: no input")
	}
	if opts.Dir == "" {
		opts.Dir = in.Dir
	}
	return GoSourceLoad(in.Files, opts)
}

// resolveStartupGoSource validates the captured selection once the Bash++ and
// POSIX profiles are known. It is called from runAll before any input is
// looked at, so a refusal cannot reach a shell code path.
func resolveStartupGoSource() error {
	if startupGoSourceErr != nil {
		return startupGoSourceErr
	}
	if !startupGoSourceSel.Requested() {
		return nil
	}
	binary := BashPPBinaryBash
	if AgentOSBashPPDefault {
		binary = BashPPBinaryBashy
	}
	res, err := ResolveGoSource(startupGoSourceSel, GoSourceContext{
		Binary: binary,
		BashPP: startupBashPP.Enabled,
		// Every spelling of the POSIX selection counts: the flag, `-o posix`,
		// the inherited environment, and the argv0=sh certification route.
		Posix:         commandLinePosixMode() || effectiveStartupPosix(),
		HasOperand:    *command == "" && !*readStdin && flag.NArg() > 0,
		ShellOnlyMode: startupShellOnlyMode(),
	})
	if err != nil {
		return err
	}
	startupGoSource = res
	return nil
}

// startupShellOnlyMode names the selected mode that only exists for shell
// input, or "" when none is selected. These modes build the SHELL parser
// before any input dispatch happens, which is how `--source=go --pretty-print
// x.go` used to report a shell parse error ("a command can only contain words
// and redirects") for a perfectly good Go program.
func startupShellOnlyMode() string {
	switch {
	case *pretty:
		return "--pretty-print"
	case *dumpPO:
		return "--dump-po-strings"
	case *dumpStrs || *dumpShort:
		return "--dump-strings"
	}
	return ""
}

// goSourceDiagnosticText renders one of Bashy's own Go-source errors: a
// refusal already names the shell, anything else (an unreadable file, say)
// gets the shell's name added.
func goSourceDiagnosticText(err error) string {
	msg := err.Error()
	if !IsGoSourceError(err) && !strings.HasPrefix(msg, "bashy: ") {
		msg = "bashy: " + msg
	}
	return msg
}

// goSourceFailure reports one of Bashy's own Go-source errors and exits 2,
// bash's usage-error status. There is no path from here back into shell
// parsing: a refused or unreadable Go input is the end of the invocation.
func goSourceFailure(err error) error {
	if err == nil {
		return nil
	}
	fmt.Fprintln(os.Stderr, goSourceDiagnosticText(err))
	return interp.ExitStatus(2)
}

// goSourceLoadFailure reports the FRONT END's diagnostic verbatim. Go's
// file:line:col text is what a differential harness compares against the Go
// toolchain, so Bashy adds no prefix that would have to be stripped again.
// Bashy's own refusals — notably "no front end in this build" — still get the
// shell's name, because they are not something the Go toolchain ever says.
func goSourceLoadFailure(err error) error {
	if IsGoSourceError(err) {
		return goSourceFailure(err)
	}
	fmt.Fprintln(os.Stderr, err)
	return interp.ExitStatus(2)
}

// runGoSourceInvocation runs one `--source=go` invocation end to end.
//
// Shell startup files are deliberately not loaded, and neither are the login
// shell's exit hooks: a Go program's semantics are the Go specification's, and
// a ~/.bashyrc that defined a function or an alias must not be able to change
// what it means — nor may a ~/.bash_logout append shell effects to it after
// main returns. `bashy --login --bashpp --source=go p.go` runs p.go, and
// nothing else.
func runGoSourceInvocation() error {
	operand := ""
	if *command == "" && !*readStdin && flag.NArg() > 0 {
		operand = flag.Arg(0)
	}
	var stdin io.Reader
	if operand == "" && *command == "" && len(startupGoSource.Files) == 0 {
		stdin = os.Stdin
	}
	in, err := CollectGoSources(startupGoSource, operand, *command, stdin)
	if err != nil {
		return goSourceFailure(err)
	}
	// --check is semantic-only. `-n` / `-o noexec` means the same thing for
	// this input, so a build-only corpus row may spell either. RunMain stays
	// false so the loaded program carries no entry calls at all — the absence
	// of the calls, not a later branch, is what makes "executes nothing" true.
	noExec := startupGoSource.Check || cmdlineNoExec() || AgentOSCommandLineNoExec(resolvedStartupPosix())
	prog, err := LoadGoSource(in, GoSourceOptions{RunMain: !noExec, Dir: in.Dir})
	if err != nil {
		return goSourceLoadFailure(err)
	}
	if noExec {
		return nil
	}
	r, err := newRunner()
	if err != nil {
		return err
	}
	argv0 := in.Files[0].Name
	posArgs := flag.Args()
	if operand != "" && len(posArgs) > 0 {
		// The operand is the program, not one of its arguments — the same
		// split the existing `--bashpp script args…` interface uses.
		posArgs = posArgs[1:]
	}
	if *command != "" && len(posArgs) > 0 {
		argv0 = posArgs[0]
		posArgs = posArgs[1:]
	}
	if err := interp.WithArgv0(argv0)(r); err != nil {
		return err
	}
	// The module context travels with the SOURCE, never with the working
	// directory. See [GoSourceModuleDir].
	if GoSourceModuleDir != nil && in.Dir != "" {
		if err := GoSourceModuleDir(in.Dir)(r); err != nil {
			return err
		}
	}
	if len(posArgs) > 0 {
		if err := interp.Params(append([]string{"--"}, posArgs...)...)(r); err != nil {
			return err
		}
	}
	r.Reset()
	return r.Run(context.Background(), prog.File)
}
