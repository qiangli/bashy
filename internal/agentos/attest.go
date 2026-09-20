// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// attest.go — function-call attestation (Sprint 216, B18): when a decorated
// Bash# function or an agentic{} function completes, ONE receipt is appended
// to the yoke skills/craft evidence ledger — the same skills.AttestRecord a
// `bashy skill run` writes, into the same <store>/attest/<name>.jsonl, read
// back by the same `bashy craft history`. No store of its own, no second
// record type, no model call.
//
// Why the ledger and not a new log: pkg/craft was written as a READER over
// evidence that already existed, precisely to stop the eighth parallel outcome
// log. A function completing under a contract is evidence of exactly the kind
// a contracted skill run is — a named thing, at a coordinate, whose checks
// held or did not — so it belongs in the same place, where the read side
// already pools by name and coordinate.
//
// What one receipt carries, in the record's own fields:
//
//	Name           the function (a method is Type.Method) — the ledger file
//	Attest.Skill   "fn:<name>", the identity that ran
//	Attest.Passed  every clause check that held, as "<clause>:<check>"
//	Attest.Failed  the check that failed, the same way
//	Attest.Valid   the call completed (status 0) and no check failed
//	Status         the call's exit status — 6 (skills.AttestYield) is an
//	               agentic yield, recorded as a HANDOFF, never as a completion
//	ContextKey     this host's environment coordinate (skills.HostCoordinate)
//	Tier           the executor, "bashy@<version>" (the skills convention)
//	StoreRevision  craft.Revision of the store, taken BEFORE the append
//
// How one call becomes one receipt, whatever it is decorated with: every
// native decorator is wrapped by attestSink.attesting. The OUTERMOST native
// rung on a call opens an attestFrame on the ctx it hands down the chain;
// inner rungs on the SAME call (keyed on the engine's Call, so a decorated
// callee inside the body opens its own) find the frame and contribute
// verdicts to it; when the outermost rung returns, the frame is appended
// once. A clause under @retry re-records the same check per attempt and the
// last verdict wins. An agentic{} function with no source decorators is
// reached by an advised rung — newAdviceCallback adds the native `attest`
// decorator (a pure pass-through) to every agentic registration, so the
// frame opens with nothing to contribute but the completion itself.
//
// A plain, undecorated, non-agentic function never enters a chain and never
// writes: identity. Bash OFF never registers decorators or consults advice,
// and --posix / cmd/bash never link this file: structural.
//
// Off switch: BASHY_ATTEST=0|false|no|off. No store (an empty skills dir) is
// also off. An append that fails is spoken once on stderr — a ledger that
// silently stopped accruing would be an absence read as a clean record.
package agentos

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	dhntskills "github.com/dhnt/dhnt/skills"
	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/bashy/internal/cli"
	"github.com/qiangli/yoke/pkg/craft"
	coreskills "github.com/qiangli/yoke/pkg/skills"
)

// attestAdviceID is the rule id of the advised `attest` rung on an agentic
// function. It is bashy's own, not a rules-file id: a rules file cannot spell
// a colon-namespaced id, so it can neither collide with nor shadow it.
const attestAdviceID = "bashy:attest"

// attestSink is where one shell's function receipts go. It resolves the store
// and the coordinate lazily and once — the coordinate runs host probes, and a
// script that never completes a decorated call must never pay for them.
type attestSink struct {
	storeDir string
	tier     string
	stderr   io.Writer

	once  sync.Once
	coord string

	spoken sync.Once // the first failed append is reported; the rest are not noise worth a line each
}

// compiledAttestSink resolves the compiled host's sink from the PROCESS
// environment on the first decorated call, not at package init. The compiled
// decorator registry (decorators.go's init()) is built once at host startup —
// before an embedder or a test can set BASHY_SKILLS_DIR / BASHY_ATTEST — so
// capturing os.Environ() there would freeze whatever was ambient when the
// binary started. Deferring to first use still resolves at most once (the
// coordinate's host probes are paid for once), it just waits until something
// has had a chance to configure the environment first.
var (
	compiledAttestOnce sync.Once
	compiledAttestVal  *attestSink
)

func compiledAttestSink(stderr io.Writer) *attestSink {
	compiledAttestOnce.Do(func() { compiledAttestVal = newAttestSink(os.Environ(), stderr) })
	return compiledAttestVal
}

// newAttestSink resolves the sink for one shell from its environment. A nil
// sink means attestation is off, and every method on a nil sink is a no-op.
func newAttestSink(env []string, stderr io.Writer) *attestSink {
	switch strings.ToLower(strings.TrimSpace(outputEnv(env, "BASHY_ATTEST"))) {
	case "0", "false", "no", "off":
		return nil
	}
	dir := attestStoreDir(env)
	if dir == "" {
		return nil
	}
	tier := "local"
	if v := cli.BashyVersion(); v != "" {
		tier = "bashy@" + v
	}
	return &attestSink{storeDir: dir, tier: tier, stderr: stderr}
}

// attestStoreDir is the skills store this shell's receipts land in — the same
// resolution skills.DefaultStoreDir makes, read from the shell's env rather
// than the process's so an embedder's env is honoured, falling back to the
// process's when the env names nothing.
func attestStoreDir(env []string) string {
	if d := strings.TrimSpace(outputEnv(env, "BASHY_SKILLS_DIR")); d != "" {
		return d
	}
	if h := strings.TrimSpace(outputEnv(env, "BASHY_HOME")); h != "" {
		return filepath.Join(h, "skills")
	}
	return coreskills.DefaultStoreDir()
}

// coordinate is this host's environment coordinate, computed on first use.
func (s *attestSink) coordinate() string {
	s.once.Do(func() { s.coord = coreskills.HostCoordinate(skillsOptions()...) })
	return s.coord
}

// attestFrame is the evidence one decorated call accumulates across its rungs.
type attestFrame struct {
	key     any // the engine's Call: the identity every rung of one chain shares
	order   []string
	verdict map[string]bool
}

type attestFrameKey struct{}

// attestFrameOf returns the frame the ctx carries for this call, or nil when
// the ctx carries none or carries another call's (a decorated callee inside a
// decorated body sees its caller's frame and must not write into it).
func attestFrameOf(ctx context.Context, c *nativeDecoratorCall) *attestFrame {
	f, _ := ctx.Value(attestFrameKey{}).(*attestFrame)
	if f == nil || c.key == nil || f.key != c.key {
		return nil
	}
	return f
}

// record notes one clause check's verdict as "<clause>:<check>". The same id
// recorded again (a @retry attempt) replaces the verdict and keeps its place.
func (f *attestFrame) record(clause, check string, held bool) {
	id := clause + ":" + check
	if _, seen := f.verdict[id]; !seen {
		f.order = append(f.order, id)
	}
	f.verdict[id] = held
}

// attestRecord notes a check's verdict on the frame the call carries, if any.
// Called from the clauses (contracts.go) after each check.
func attestRecord(ctx context.Context, c *nativeDecoratorCall, clause, check string, held bool) {
	if f := attestFrameOf(ctx, c); f != nil {
		f.record(clause, check, held)
	}
}

// attesting wraps one native decorator so the outermost native rung on a call
// opens the frame and appends it, and every inner one contributes through it.
func (s *attestSink) attesting(fn nativeDecoratorFunc) nativeDecoratorFunc {
	if s == nil {
		return fn
	}
	return func(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
		if attestFrameOf(ctx, c) != nil || c.key == nil {
			return fn(ctx, c, args)
		}
		f := &attestFrame{key: c.key, verdict: map[string]bool{}}
		err := fn(context.WithValue(ctx, attestFrameKey{}, f), c, args)
		if err != nil {
			// A decorator that refused its own arguments broke the chain; the
			// call never ran and there is no completion to attest.
			return err
		}
		s.append(c, f)
		return nil
	}
}

// attestDecorator is the pass-through rung advice puts on an agentic{}
// function so its completion is attested. It takes no arguments and does
// nothing but run the rest of the chain; attesting() around it does the rest.
func attestDecorator(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
	if len(args) != 0 {
		return fmt.Errorf("attest takes no arguments")
	}
	c.Next(ctx)
	return nil
}

// append writes the call's receipt. The store revision is taken BEFORE the
// append, so the receipt names the store it landed on, not the one it made.
func (s *attestSink) append(c *nativeDecoratorCall, f *attestFrame) {
	status := c.Status
	att := dhntskills.Attestation{Skill: "fn:" + c.Name, Tier: s.tier}
	for _, id := range f.order {
		if f.verdict[id] {
			att.Passed = append(att.Passed, id)
		} else {
			att.Failed = append(att.Failed, id)
		}
	}
	att.Valid = status == 0 && len(att.Failed) == 0
	rec := coreskills.AttestRecord{
		At:            time.Now().UTC(),
		Name:          c.Name,
		Tier:          s.tier,
		ContextKey:    s.coordinate(),
		Attest:        att,
		Status:        &status,
		StoreRevision: craft.Revision(s.storeDir).String(),
	}
	if _, err := coreskills.AppendAttest(s.storeDir, rec); err != nil {
		s.spoken.Do(func() {
			w := s.stderr
			if w == nil {
				w = os.Stderr
			}
			fmt.Fprintf(w, "bashy: attest: %v — function receipts are not being recorded\n", err)
		})
	}
}
