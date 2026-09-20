// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// confirm.go — @confirm: effect-derived --what-if / --confirm on a Bash#
// function (Sprint 216, B16: PowerShell's SupportsShouldProcess borrowed
// without the shell).
//
// A function declared `@confirm()` gains two call-site flags it does not
// implement itself. Both are GENERATED from the one effect source the surface
// already has — the Command Atlas effects a dispatched command declares
// (curated verbs, in-process tools, `bashy commands add` records) — and they
// are decided PER OPERATION, at the same ExecHandler rung that enforces the
// @guard cap, never once for the whole call:
//
//   - `fn --what-if ARG…` describes every governed operation the body would
//     dispatch — `what-if: fn: would run <argv> (effects: …)` — and runs none
//     of them. Pure/read commands run, so the description is exact.
//   - `fn ARG…` confirms each HIGH-IMPACT operation before it runs: one whose
//     declared effects include destroy, spend, cred or priv, or one the atlas
//     does not classify at all (fail closed, as Cap.Exceeded does). The
//     question goes to the human through `bashy ask` — the channel the calling
//     program does not own. `yes` runs it; anything else refuses it with
//     status 126 and the body continues, as PowerShell continues after "No".
//   - When no channel reaches a human — bashy orchestrated the run
//     (BASHY_AGENTIC), a first-party harness owns input (BASHY_ASK_HANDLER),
//     or there is neither a usable terminal nor an attended askpass — the
//     operation is NOT guessed. The call yields: exit 6
//     (weavecli.ExitInputRequired), one line naming the operation, its
//     token and the resume form; nothing with a side effect runs after the
//     yield point. See rfcs/0001-agentic-yield.md.
//   - `fn --confirm=TOKEN:yes,TOKEN:no ARG…` is the resume form: the answers
//     a harness (or a human at a later terminal) supplies per operation. A
//     token is derived from the operation's exact argv, so an answer applies
//     to that operation and no other, whatever order the replay dispatches
//     them in. Bare `--confirm` is accepted and means the default.
//
// PowerShell mapping (pinned in confirm_test.go): pure/read = no ShouldProcess
// call; write/net/exec/remote/persist = ConfirmImpact Medium (auto-confirmed
// under the default High preference, but shown by -WhatIf); destroy/spend/
// cred/priv = ConfirmImpact High (always confirmed). There is no
// $ConfirmPreference knob and no other common parameter: the gate set is a
// property of the effect vocabulary, not of a preference.
//
// Only the FIRST argument is inspected, so a function's own flags are never
// stolen. @confirm is source-only (the advice loader knows no such decorator
// and the native refuses an advised call regardless). In plain Bash and POSIX
// mode decorator syntax does not parse and this file is inert; cmd/bash never
// links it.
package agentos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/weavecli"
	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/ctty"
)

// confirmDeclinedStatus is the status an operation reports when the human (or
// a pre-supplied answer) refused it: the shell's own "found but not run"
// status, the same one Yoke's dag uses for an effect-cap denial.
const confirmDeclinedStatus = 126

// confirmMode is the call-site mode the first argument selected.
type confirmMode int

const (
	confirmDefault confirmMode = iota // high-impact operations are confirmed
	confirmWhatIf                     // governed operations are described, none runs
)

// confirmState rides the context handed to Next(): the handler rung reads it
// for every command the body dispatches.
type confirmState struct {
	fn      string
	mode    confirmMode
	answers map[string]bool // operation token → yes/no, from --confirm=
	yielded bool
}

type confirmKey struct{}

func confirmStateFrom(ctx context.Context) (*confirmState, bool) {
	st, ok := ctx.Value(confirmKey{}).(*confirmState)
	return st, ok
}

// confirmDecorator is the @confirm native: bind the call-site flag from the
// first argument, run the rest of the chain with the state on the context,
// and seal the call as a yield (exit 6) when an operation needed an answer
// nobody could give.
func confirmDecorator(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
	if c.Advised != "" {
		return errors.New("confirm is source-only and never applied by advice")
	}
	if len(args) != 0 {
		return errors.New("confirm takes no arguments: the confirmed operations are derived from the effects each command declares")
	}
	st := &confirmState{fn: c.Name, answers: map[string]bool{}}
	if len(c.Args) > 0 {
		if flag, ok := c.Args[0].(string); ok {
			consumed := true
			switch {
			case flag == "--what-if":
				st.mode = confirmWhatIf
			case flag == "--confirm":
			case strings.HasPrefix(flag, "--confirm="):
				if err := parseConfirmAnswers(flag[len("--confirm="):], st.answers); err != nil {
					return err
				}
			default:
				consumed = false
			}
			if consumed {
				c.Args = c.Args[1:]
			}
		}
	}
	c.Next(context.WithValue(ctx, confirmKey{}, st))
	if st.yielded {
		c.Status = weavecli.ExitInputRequired
	}
	return nil
}

var confirmAnswerRE = regexp.MustCompile(`^([0-9a-f]{8}):(yes|y|no|n)$`)

// parseConfirmAnswers reads the resume form's answer list: TOKEN:yes|no,
// comma-separated, case-insensitive on the answer. An empty list is refused —
// `--confirm=` says nothing — and so is a token answered both ways.
func parseConfirmAnswers(spec string, into map[string]bool) error {
	bad := func() error {
		return fmt.Errorf("confirm: --confirm=%s: want TOKEN:yes or TOKEN:no per operation, comma-separated (--what-if lists the tokens)", spec)
	}
	if strings.TrimSpace(spec) == "" {
		return bad()
	}
	for _, item := range strings.Split(spec, ",") {
		m := confirmAnswerRE.FindStringSubmatch(strings.ToLower(strings.TrimSpace(item)))
		if m == nil {
			return bad()
		}
		yes := m[2] == "yes" || m[2] == "y"
		if prev, seen := into[m[1]]; seen && prev != yes {
			return fmt.Errorf("confirm: --confirm=%s: %s is answered both yes and no", spec, m[1])
		}
		into[m[1]] = yes
	}
	return nil
}

// confirmImpact is PowerShell's ConfirmImpact, read off the declared effects.
type confirmImpact int

const (
	impactNone   confirmImpact = iota // pure/read only: not a governed operation
	impactMedium                      // a side effect the default preference auto-confirms
	impactHigh                        // destroy/spend/cred/priv, or unclassified: always confirmed
)

// confirmGate is the high-impact set: the atlas effects that require a
// human's answer. It is a selection FROM the vocabulary, not a vocabulary.
var confirmGate = map[string]bool{
	atlas.EffDestroy: true,
	atlas.EffSpend:   true,
	atlas.EffCred:    true,
	atlas.EffPriv:    true,
}

var knownEffect = func() map[string]bool {
	m := map[string]bool{}
	for _, e := range atlas.Effects() {
		m[e] = true
	}
	return m
}()

// impactOf classifies declared effects. No declaration at all — a command
// absent from the atlas and the operator's ring — is high: the same
// fail-closed reading Cap.Exceeded gives an unclassified command.
func impactOf(effects []string) confirmImpact {
	if len(effects) == 0 {
		return impactHigh
	}
	out := impactNone
	for _, e := range effects {
		switch {
		case e == atlas.EffPure, e == atlas.EffRead:
		case confirmGate[e], !knownEffect[e]:
			return impactHigh
		default:
			out = impactMedium
		}
	}
	return out
}

// declaredEffects resolves a dispatched command name to the effects it
// declares, in the shell's own resolution order: the atlas (curated verbs and
// in-process tools), then the operator's registered ring. nil means
// unclassified.
func declaredEffects(arg0 string) []string {
	name := normalizeCommandName(baseName(arg0))
	if e, ok := atlas.Lookup(name); ok {
		return e.Effects
	}
	if rec, ok := registeredLookup(name); ok {
		return atlas.RegisteredEntry(rec.AtlasSpec()).Effects
	}
	return nil
}

// confirmToken identifies one operation by its exact argv: eight hex digits
// of SHA-256 over the NUL-joined words. Stable across replays, distinct
// across operations, short enough to type.
func confirmToken(args []string) string {
	sum := sha256.Sum256([]byte(strings.Join(args, "\x00")))
	return hex.EncodeToString(sum[:4])
}

var plainWordRE = regexp.MustCompile(`^[A-Za-z0-9_./=:+@%,-]+$`)

// renderArgv spells argv the way a person would type it, quoting only what
// needs quoting.
func renderArgv(args []string) string {
	words := make([]string, len(args))
	for i, a := range args {
		if plainWordRE.MatchString(a) {
			words[i] = a
		} else {
			words[i] = shellQuote(a)
		}
	}
	return strings.Join(words, " ")
}

func renderEffects(effects []string) string {
	if len(effects) == 0 {
		return "unknown"
	}
	return strings.Join(effects, ",")
}

// normalizeCommandName strips platform-specific executable extensions (e.g.,
// .exe on Windows) so Windows absolute paths like C:\Program Files\Git\usr\bin\cat.exe
// map to the registered command "cat" in the atlas.
func normalizeCommandName(name string) string {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".exe") {
		return strings.ToLower(name[:len(name)-4])
	}
	return name
}

// confirmHandler is the ExecHandler rung that decides each governed
// operation. It sits just outside the dry-run and userland handlers so
// in-process tools are governed too, and inside audit so the ledger still
// records what happened to the command.
func confirmHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			st, ok := confirmStateFrom(ctx)
			if !ok || len(args) == 0 {
				return next(ctx, args)
			}
			effects := declaredEffects(args[0])
			impact := impactOf(effects)
			if impact == impactNone {
				return next(ctx, args)
			}
			stderr := interp.HandlerCtx(ctx).Stderr
			op, tok, eff := renderArgv(args), confirmToken(args), renderEffects(effects)
			if st.mode == confirmWhatIf {
				gate := ""
				if impact == impactHigh {
					gate = "; confirm " + tok
				}
				fmt.Fprintf(stderr, "what-if: %s: would run %s (effects: %s%s)\n", st.fn, op, eff, gate)
				return nil
			}
			if st.yielded {
				// Nothing with a side effect runs past the yield point: the
				// harness replays the whole call once it has the answer.
				fmt.Fprintf(stderr, "%s: not run after yield: %s\n", st.fn, op)
				return interp.ExitStatus(weavecli.ExitInputRequired)
			}
			if impact != impactHigh {
				return next(ctx, args)
			}
			yes, answered := st.answers[tok]
			if !answered {
				var err error
				yes, err = confirmAsk(st.fn, op, eff)
				if err != nil {
					st.yielded = true
					emitConfirmYield(stderr, st.fn, tok, op, eff, err)
					return interp.ExitStatus(weavecli.ExitInputRequired)
				}
			}
			if yes {
				return next(ctx, args)
			}
			fmt.Fprintf(stderr, "%s: declined %s: %s\n", st.fn, tok, op)
			return interp.ExitStatus(confirmDeclinedStatus)
		}
	}
}

// emitConfirmYield writes the one line a harness needs: the operation, its
// token, why no answer arrived, and the exact resume form. A JSON envelope
// (error.code "input_required") only under explicit BASHY_AGENTIC — a format
// is a contract, never sniffed.
func emitConfirmYield(w io.Writer, fn, tok, op, eff string, cause error) {
	mode := weavecli.OutputPlain
	if weavecli.IsAgent() {
		mode = weavecli.OutputJSON
	}
	err := fmt.Errorf("input required: confirm %s: %s (effects: %s): %v; resume with %s --confirm=%s:yes ... or --confirm=%s:no ...",
		tok, op, eff, cause, fn, tok, tok)
	weavecli.EmitError(w, mode, fn, weavecli.ExitInputRequired, err)
}

// errConfirmUnattended is the yield cause when nobody can be asked.
var errConfirmUnattended = errors.New("no channel reaches a human")

// confirmAsk is the one seam to the human. Tests replace it; nothing else
// does. It reports (yes, nil) for an answer and a non-nil error when the
// question could not be put to anyone — the yield.
var confirmAsk = askHumanToConfirm

// askHumanToConfirm puts the question through `bashy ask` (self re-exec: the
// verb owns the terminal/askpass channels, the framing that names the
// requester, and the sanitization). It never waits on the rendezvous rung:
// an unattended call yields at once instead of blocking for a claim that
// may never come — the harness is the one that can reach the human.
func askHumanToConfirm(fn, op, effects string) (bool, error) {
	if weavecli.IsAgent() {
		return false, fmt.Errorf("%w (BASHY_AGENTIC)", errConfirmUnattended)
	}
	probe := ctty.CurrentProbe(ctty.ChannelAuto)
	var ch ctty.Channel
	switch {
	case probe.HandlerSet:
		return false, fmt.Errorf("%w (the harness owns input: %s)", errConfirmUnattended, ctty.HandlerEnv)
	case probe.TTY:
		ch = ctty.ChannelTTY
	case probe.GUI:
		ch = ctty.ChannelGUI
	default:
		return false, errConfirmUnattended
	}
	prompt := fmt.Sprintf("%s wants to run: %s (effects: %s). Type yes to proceed, anything else to refuse", fn, op, effects)
	cmd := exec.Command(bashySelfPath(), "ask",
		"--prompt", prompt, "--name", "CONFIRM", "--secret=false", "--stdout",
		"--channel", string(ch))
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("bashy ask: %w", err)
	}
	a := strings.ToLower(strings.TrimSpace(string(out)))
	return a == "y" || a == "yes", nil
}
