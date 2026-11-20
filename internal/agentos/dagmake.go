package agentos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/qiangli/bash/interp"
	"github.com/qiangli/yoke/pkg/dag"
)

// Agents reach for `make` first, but a growing set of repos keeps its tasks
// in dag.md and has no makefile (Sprint 678). dagMakeHandler meets that
// habit exactly where the in-process POSIX make would have nothing to
// read: no makefile under any of its names in the effective directory, but
// a dag file there. The mapped invocation then runs `bashy dag` — one
// stderr hint per run teaches the native spelling — while every other
// shape (a makefile present, an explicit -f, --help/--version, an unmapped
// flag, certification runs) falls through to the coreutils applet with
// argv untouched, so a directory with a Makefile behaves exactly as it did
// before this rung existed. Coreutils itself never learns about dag: the
// interception lives here, ahead of coreutilsshell.Handler, mirroring
// gnuMakeHandler ahead of it.

// dagMakePlan is one parsed make invocation translated towards dag.
type dagMakePlan struct {
	dir       string // effective directory (base with the -C chain applied)
	targets   []string
	overrides []string // VAR=value operands, passed to dag as its own overrides
	dry       bool
	keep      bool
	force     bool
	quiet     bool
	jobs      int

	dagPath string   // resolved by wantsDagMake
	dagArgs []string // arguments to `bashy dag -f <dagPath>`
	list    bool     // dagArgs is the --list form (make help / bare make)
}

// parseMakeArgs translates the make flags agents actually type. Anything
// it cannot map faithfully — an explicit -f, --help/--version, an unknown
// or POSIX-only flag (-e -i -p -q -r -t -S) — reports not-ok, and the
// caller leaves the invocation to the applet unchanged.
func parseMakeArgs(baseDir string, args []string) (dagMakePlan, bool) {
	p := dagMakePlan{dir: baseDir}
	flagsDone := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() (string, bool) {
			if i+1 >= len(args) {
				return "", false
			}
			i++
			return args[i], true
		}
		switch {
		case flagsDone:
			p.operand(a)
		case a == "--":
			flagsDone = true
		case a == "-C" || a == "--directory":
			v, ok := next()
			if !ok {
				return dagMakePlan{}, false
			}
			p.dir = joinDir(p.dir, v)
		case a == "-f" || a == "--file" || a == "--makefile" ||
			strings.HasPrefix(a, "--file=") || strings.HasPrefix(a, "--makefile="):
			return dagMakePlan{}, false
		case a == "--help" || a == "-h" || a == "--version" || a == "-V":
			return dagMakePlan{}, false
		case strings.HasPrefix(a, "--directory="):
			p.dir = joinDir(p.dir, a[len("--directory="):])
		case a == "--dry-run" || a == "--just-print" || a == "--recon":
			p.dry = true
		case a == "--keep-going":
			p.keep = true
		case a == "--always-make":
			p.force = true
		case a == "--silent" || a == "--quiet":
			p.quiet = true
		case a == "--print-directory" || a == "--no-print-directory":
			// dag announces no directories; nothing to map.
		case a == "--jobs":
			// Bare -j (unbounded in GNU make) has no dag spelling; a
			// following number is the form agents mean by "-j N".
			if i+1 < len(args) {
				if n, err := strconv.Atoi(args[i+1]); err == nil && n > 0 {
					p.jobs = n
					i++
				}
			}
		case strings.HasPrefix(a, "--jobs="):
			n, err := strconv.Atoi(a[len("--jobs="):])
			if err != nil || n < 1 {
				return dagMakePlan{}, false
			}
			p.jobs = n
		case strings.HasPrefix(a, "--"):
			return dagMakePlan{}, false
		case len(a) > 1 && a[0] == '-':
			if !p.shortBundle(a[1:], next) {
				return dagMakePlan{}, false
			}
		default:
			p.operand(a)
		}
	}
	return p, true
}

// shortBundle consumes one bundle of POSIX-style short flags ("-skB",
// "-j4", "-Csub"). j/C/f end the bundle: j and C take the rest (or the
// next operand) as their value, and f refuses the whole invocation.
func (p *dagMakePlan) shortBundle(bundle string, next func() (string, bool)) bool {
	for j := 0; j < len(bundle); j++ {
		switch bundle[j] {
		case 'n':
			p.dry = true
		case 'k':
			p.keep = true
		case 'B':
			p.force = true
		case 's':
			p.quiet = true
		case 'w':
			// --print-directory: nothing to map.
		case 'j':
			rest := bundle[j+1:]
			if rest == "" {
				if v, ok := next(); ok {
					n, err := strconv.Atoi(v)
					if err != nil || n < 1 {
						// Not a number: GNU make's bare -j, and v is
						// its own operand after all.
						p.operand(v)
						return true
					}
					p.jobs = n
				}
				return true
			}
			n, err := strconv.Atoi(rest)
			if err != nil || n < 1 {
				return false
			}
			p.jobs = n
			return true
		case 'C':
			rest := bundle[j+1:]
			if rest == "" {
				v, ok := next()
				if !ok {
					return false
				}
				rest = v
			}
			p.dir = joinDir(p.dir, rest)
			return true
		default: // 'f', 'h', 'V', and every unmapped flag
			return false
		}
	}
	return true
}

func (p *dagMakePlan) operand(a string) {
	if i := strings.IndexByte(a, '='); i > 0 && isMakeVarName(a[:i]) {
		p.overrides = append(p.overrides, a)
		return
	}
	p.targets = append(p.targets, a)
}

func isMakeVarName(s string) bool {
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return s != ""
}

// makeEnvNamesFile reports whether the environment already names a
// makefile (MAKEFLAGS carrying -f, or GNU's MAKEFILES list): the applet
// would read it, so this is not the no-makefile case.
func makeEnvNamesFile(env []string) bool {
	if outputEnv(env, "MAKEFILES") != "" {
		return true
	}
	for _, tok := range strings.Fields(outputEnv(env, "MAKEFLAGS")) {
		if tok == "-f" || strings.HasPrefix(tok, "--file") || strings.HasPrefix(tok, "--makefile") {
			return true
		}
		if len(tok) > 1 && tok[0] == '-' && tok[1] != '-' && strings.ContainsRune(tok[1:], 'f') {
			return true
		}
	}
	return false
}

// wantsDagMake decides whether a make invocation rooted at baseDir falls
// through to dag, and if so returns the finished plan (dag file, argv).
// Names are compared exactly, as in wantsGNUMake, so a case-insensitive
// filesystem cannot mistake a makefile's absence for its presence.
func wantsDagMake(baseDir string, args []string, env []string) (dagMakePlan, bool) {
	if makeEnvNamesFile(env) {
		return dagMakePlan{}, false
	}
	p, ok := parseMakeArgs(baseDir, args)
	if !ok {
		return dagMakePlan{}, false
	}
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		return dagMakePlan{}, false
	}
	for _, e := range entries {
		switch e.Name() {
		case "makefile", "Makefile", "GNUmakefile":
			return dagMakePlan{}, false
		}
	}
	dagPath, err := dag.Discover(p.dir)
	if err != nil {
		return dagMakePlan{}, false
	}
	p.dagPath = dagPath
	switch {
	case len(p.targets) == 0 && !p.dry:
		p.list = true
	case len(p.targets) == 1 && p.targets[0] == "help" && !p.dry:
		// A dag help target outranks the fixed listing; a dag file dag
		// itself cannot parse stays in target mode, so its own parse
		// error is the one reported.
		if doc, err := dag.ParseFile(dagPath); err == nil {
			if _, has := doc.Lookup("help"); !has {
				p.list = true
			}
		}
	}
	if p.list {
		p.dagArgs = []string{"--list"}
		return p, true
	}
	var da []string
	if p.dry {
		da = append(da, "--dry-run")
	}
	if p.keep {
		da = append(da, "-k")
	}
	if p.force {
		da = append(da, "-B")
	}
	if p.quiet {
		da = append(da, "--quiet")
	}
	if p.jobs > 0 {
		da = append(da, "-j", strconv.Itoa(p.jobs))
	}
	da = append(da, p.targets...)
	da = append(da, p.overrides...)
	p.dagArgs = da
	return p, true
}

// dagMakeExec runs `bashy dag <args>` as a child of this same binary, with
// the caller's directory, environment and stdio — the exact front-door dag
// behavior, without a process-global chdir inside a shell that may be
// running other work. The second result is non-nil only when the child
// could not be started, which is the caller's cue to fall through to the
// applet as if this rung were absent. Tests replace this variable.
var dagMakeExec = func(ctx context.Context, dir string, env []string, stdin io.Reader, stdout, stderr io.Writer, args []string) (int, error) {
	self, err := os.Executable()
	if err != nil {
		return 0, err
	}
	cmd := exec.CommandContext(ctx, self, append([]string{"dag"}, args...)...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode(), nil
		}
		return 0, err
	}
	return 0, nil
}

const dagMakePointer = "Run: bashy dag <target> | skill: bashy dag --skill DIR | more: bashy help"

// runDagMake executes one accepted plan: the dag run, the list pointer,
// and the single fallthrough hint. handled=false only when the dag child
// could not be started.
func runDagMake(ctx context.Context, p dagMakePlan, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, bool) {
	args := append([]string{"-f", p.dagPath}, p.dagArgs...)
	code, err := dagMakeExec(ctx, p.dir, env, stdin, stdout, stderr, args)
	if err != nil {
		return 0, false
	}
	if p.list && code == 0 && !agentModeForEnv(env) {
		fmt.Fprintln(stdout, dagMakePointer)
	}
	emitDagMakeHint(stderr, env, p)
	return code, true
}

// dagMakeHintText is the teaching line, in the story's own shape:
// `make TARGET -> bashy dag TARGET (dag.md)`.
func dagMakeHintText(p dagMakePlan) string {
	makeDisp := strings.Join(p.targets, " ")
	dagDisp := makeDisp
	switch {
	case p.list:
		dagDisp = "--list"
	case len(p.targets) == 0 && p.dry:
		makeDisp, dagDisp = "-n", "--dry-run"
	}
	if makeDisp != "" {
		makeDisp = " " + makeDisp
	}
	return fmt.Sprintf("make%s -> bashy dag %s (dag.md)", makeDisp, dagDisp)
}

type dagMakeHintLine struct {
	Schema  string `json:"schema_version"`
	Kind    string `json:"kind"` // "make-dag-fallthrough"
	Tool    string `json:"tool"`
	Suggest string `json:"suggest"`
	Off     string `json:"off"`
}

// emitDagMakeHint writes the one hint for an intercepted run: a plain
// stderr line, or under BASHY_AGENTIC one bashy-hint-v1 JSON record
// (carrying the list pointer too, so dag's stdout envelope stays clean).
// BASHY_HINTS=off silences it, as on every hint surface.
func emitDagMakeHint(w io.Writer, env []string, p dagMakePlan) {
	if w == nil {
		return
	}
	switch strings.ToLower(outputEnv(env, "BASHY_HINTS")) {
	case "0", "false", "off", "no":
		return
	}
	text := dagMakeHintText(p)
	if !agentModeForEnv(env) {
		fmt.Fprintln(w, text)
		return
	}
	if p.list {
		text += "; " + dagMakePointer
	}
	b, _ := json.Marshal(dagMakeHintLine{
		Schema:  nudgeSchemaVersion,
		Kind:    "make-dag-fallthrough",
		Tool:    "make",
		Suggest: text,
		Off:     "BASHY_HINTS=off",
	})
	fmt.Fprintf(w, "%s\n", b)
}

// dagMakeHandler is the shell-rung: it sits right after gnuMakeHandler in
// wireExec's non-POSIX ring, ahead of the coreutils applet handler.
func dagMakeHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 || args[0] != "make" || certProfile() {
				return next(ctx, args)
			}
			hc := interp.HandlerCtx(ctx)
			env := handlerEnv(hc.Env)
			plan, ok := wantsDagMake(hc.Dir, args[1:], env)
			if !ok {
				return next(ctx, args)
			}
			code, handled := runDagMake(ctx, plan, env, hc.Stdin, hc.Stdout, hc.Stderr)
			if !handled {
				return next(ctx, args)
			}
			if code != 0 {
				return interp.ExitStatus(code)
			}
			return nil
		}
	}
}

// tryDagMakeFrontDoorAt is the front-door rung (`bashy make …`): the same
// acceptance and runner, rooted at an explicit directory.
func tryDagMakeFrontDoorAt(dir string, args []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, bool) {
	if certProfile() {
		return 0, false
	}
	if env == nil {
		env = os.Environ()
	}
	plan, ok := wantsDagMake(dir, args, env)
	if !ok {
		return 0, false
	}
	return runDagMake(context.Background(), plan, env, stdin, stdout, stderr)
}

func tryDagMakeFrontDoor(args []string, stdin io.Reader, stdout, stderr io.Writer) (int, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return 0, false
	}
	return tryDagMakeFrontDoorAt(dir, args, nil, stdin, stdout, stderr)
}

// commandResolverFor composes the resolver wireExec hands the interpreter:
// the registered ring's, widened so `type make` / `command -v make` name
// what actually runs — the in-process coreutils applet (plus, off the
// POSIX branch, this fallthrough) — instead of whatever make the host PATH
// happens to hold. Path stays empty on purpose: `command -v` then prints
// the bare name, which re-resolves to the applet, and `type -p` honestly
// prints nothing (there is no make file). Schema stays nil, so the exec
// ladder is untouched. Certification runs stand down entirely.
func commandResolverFor(posix bool) interp.CommandResolverFunc {
	return func(name string) (interp.ResolvedCommand, bool) {
		if name == "make" && !certProfile() {
			desc := "make is a bashy in-process applet (coreutils POSIX make"
			if !posix {
				desc += "; with no makefile in the directory it forwards to bashy dag"
			}
			desc += ")"
			return interp.ResolvedCommand{Desc: desc}, true
		}
		return registeredResolver(name)
	}
}
