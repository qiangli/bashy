// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// decorators.go — the native Bash++ decorator set (trace · guard · retry, plus
// the contract clauses require · ensure in contracts.go) and
// the registration-time policy-advice wiring, injected into the shell through
// the existing wireExec seam (never --posix, never cmd/bash).
//
// The engine (mvdan.cc/sh) exposes only the slots — interp.Decorators mirrors
// CommandResolver, interp.Advice fires at every function registration
// (declaration, eval, source, redefinition alike) — and every implementation
// lives here, riding machinery that already exists:
//
//   - @trace()  → one span per decorated call through the OTel GLOBAL
//     provider, the same library/binary boundary telemetry.ExecMiddleware
//     draws (a library emits through the global provider; only the binary
//     configures one). Call.Args/Results are NEVER attributes: typed cells
//     can carry secrets, and the shape-over-content redaction that keeps an
//     argv presentable cannot see into an arbitrary typed value.
//   - @guard(effects: "read,net") → an advice.Cap on the ctx handed to
//     Next(). WithCap INTERSECTS with any outer cap (deny-only: a nested
//     guard can never widen), and the shipped auditHandler — outermost in the
//     ExecHandler chain, caps enforced even with a nil writer — refuses any
//     command whose atlas effects exceed it, before the command
//     runs, writing Decision "deny".
//   - @retry(n:, backoff:) → SOURCE-ONLY (never advisable; the rules loader
//     refuses it and the native double-checks), a bounded, cancellation-aware
//     loop over Next() on autoretry.Backoff's default schedule. No new retry
//     engine.
package agentos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/lower/shellrt"

	"github.com/qiangli/yoke/pkg/autoretry"
	"github.com/qiangli/yoke/pkg/policy/advice"
	"github.com/qiangli/yoke/pkg/policy/audit"
)

// decoratorTracerName identifies this instrumentation library in every span
// the trace decorator emits.
const decoratorTracerName = "github.com/qiangli/bashy/internal/agentos"

// retryAttemptLimit bounds an author's @retry(n: …): the loop must stay
// bounded whatever the source says, and 100 attempts against Backoff's 3s cap
// is already minutes of wall clock — anything larger is a supervisor's job,
// not a decorator's.
const retryAttemptLimit = 100

// nativeDecorators is the standard cross-cutting set registered on every
// agentic (non-posix) shell. Registration is inert outside Bash++: decorator
// syntax does not parse in plain Bash, and a script-defined function of the
// same name shadows a native (resolution order is the engine's).
// Install the compiled slot once at host startup, never during a call.
func init() { shellrt.Decorators = compiledNativeDecorators() }

func nativeDecorators(stderr io.Writer, sink *attestSink) map[string]interp.DecoratorFunc {
	out := map[string]interp.DecoratorFunc{}
	for name, fn := range nativeDecoratorSet(stderr, sink.attesting) {
		out[name] = adaptInterpreterDecorator(fn)
	}
	return out
}

// nativeDecoratorSet is the one list both hosts register from: the
// cross-cutting three above, effect-derived confirmation (confirm.go), the two
// contract clauses (contracts.go), which
// name a failed clause on stderr, and the pass-through `attest` rung advice
// puts on agentic functions. Every one is wrapped through attesting (attest.go)
// so the outermost native rung on a call appends that call's receipt, once.
// attesting is a function rather than a resolved *attestSink so the compiled
// host (below) can defer resolving its sink past this call.
func nativeDecoratorSet(stderr io.Writer, attesting func(nativeDecoratorFunc) nativeDecoratorFunc) map[string]nativeDecoratorFunc {
	set := map[string]nativeDecoratorFunc{
		"trace":   traceDecorator,
		"guard":   guardDecorator,
		"retry":   retryDecorator,
		"confirm": confirmDecorator, // confirm.go: effect-derived --what-if / --confirm
		"attest":  attestDecorator,
	}
	for name, fn := range contractDecorators(stderr) {
		set[name] = fn
	}
	for name, fn := range set {
		set[name] = attesting(fn)
	}
	return set
}

// nativeDecoratorCall shares the implementation while preserving each engine's
// continuation. The adapters synchronize mutable fields at every Next boundary.
type nativeDecoratorCall struct {
	Name, Site, Caller, Advised string
	Args, Results               []any
	Status                      int
	Agentic                     bool
	Next                        func(context.Context)
	// Run evaluates shell source in the call's frame (Call.Run on either
	// engine): the current Args as $1..$n, vars bound, status returned.
	Run func(context.Context, string, map[string]string) int
	// key is the engine's own Call, shared by every rung of one chain and by
	// nothing else — what attest.go keys one call's frame on.
	key any
}
type nativeDecoratorFunc func(context.Context, *nativeDecoratorCall, []interp.DecoratorArg) error

func adaptInterpreterDecorator(fn nativeDecoratorFunc) interp.DecoratorFunc {
	return func(ctx context.Context, c *interp.Call, args []interp.DecoratorArg) error {
		call := &nativeDecoratorCall{Name: c.Name, Site: c.Site, Caller: c.Caller, Advised: c.Advised, Args: c.Args, Results: c.Results, Status: c.Status, Agentic: c.Agentic, key: c}
		flush := func() { c.Args, c.Results, c.Status = call.Args, call.Results, call.Status }
		call.Next = func(next context.Context) {
			flush()
			c.Next(next)
			call.Args, call.Results, call.Status = c.Args, c.Results, c.Status
		}
		call.Run = func(ctx context.Context, src string, vars map[string]string) int {
			flush()
			return c.Run(ctx, src, vars)
		}
		defer flush()
		return fn(ctx, call, args)
	}
}

// compiledNativeDecorators is the same native set for a compiled host. Such a
// host must also wire its effect-aware command handler into the shell backend.
// The compiled host has no advice slot, so its agentic functions are attested
// only when a source decorator puts them in a chain. Its attest sink resolves
// lazily (compiledAttestSink, attest.go) on the first decorated call rather
// than here at init time, since init() runs before anything — an embedder, a
// test — can shape the process environment attestation reads.
func compiledNativeDecorators() map[string]shellrt.DecoratorFunc {
	result := map[string]shellrt.DecoratorFunc{}
	attesting := func(fn nativeDecoratorFunc) nativeDecoratorFunc {
		return func(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
			return compiledAttestSink(os.Stderr).attesting(fn)(ctx, c, args)
		}
	}
	for name, fn := range nativeDecoratorSet(os.Stderr, attesting) {
		result[name] = func(ctx context.Context, c *shellrt.Call, args []shellrt.DecoratorArg) error {
			call := &nativeDecoratorCall{Name: c.Name, Site: c.Site, Caller: c.Caller, Advised: c.Advised, Args: c.Args, Results: c.Results, Status: c.Status, Agentic: c.Agentic, key: c}
			flush := func() { c.Args, c.Results, c.Status = call.Args, call.Results, call.Status }
			call.Next = func(next context.Context) {
				flush()
				c.Next(next)
				call.Args, call.Results, call.Status = c.Args, c.Results, c.Status
			}
			call.Run = func(ctx context.Context, src string, vars map[string]string) int {
				flush()
				return c.Run(ctx, src, vars)
			}
			defer flush()
			converted := make([]interp.DecoratorArg, len(args))
			for i, arg := range args {
				converted[i] = interp.DecoratorArg{Name: arg.Name, Value: arg.Value}
			}
			return fn(ctx, call, converted)
		}
	}
	return result
}

// traceDecorator emits one `call <name>` span around the rest of the chain,
// following ExecMiddleware's conventions: global provider, no attribute work
// unless the span records, the status as the point. Arg VALUES never become
// attributes — only the count.
func traceDecorator(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
	if len(args) != 0 {
		return errors.New("trace takes no arguments")
	}
	ctx, span := otel.Tracer(decoratorTracerName).Start(ctx, "call "+c.Name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("call.name", c.Name),
			attribute.Int("call.argc", len(c.Args)),
		),
	)
	defer span.End()
	if !span.IsRecording() {
		c.Next(ctx)
		return nil
	}
	if c.Site != "" {
		span.SetAttributes(attribute.String("call.site", c.Site))
	}
	if c.Caller != "" {
		span.SetAttributes(attribute.String("call.caller", c.Caller))
	}
	if c.Agentic {
		span.SetAttributes(attribute.Bool("call.agentic", true))
	}
	// Provenance when policy put this trace here rather than the author.
	if c.Advised != "" {
		span.SetAttributes(attribute.String("call.advised", c.Advised))
	}
	// Who ran this — the same accountable identity the audit ledger records.
	if p := audit.ActorFromEnv().Human; p != "" {
		span.SetAttributes(attribute.String("agent.principal", p))
	}
	started := time.Now()
	c.Next(ctx)
	span.SetAttributes(
		attribute.Int64("call.duration_ms", time.Since(started).Milliseconds()),
		attribute.Int("call.status", c.Status),
	)
	if c.Status != 0 {
		span.SetStatus(codes.Error, "status "+strconv.Itoa(c.Status))
	}
	return nil
}

// guardDecorator narrows the effect cap for everything the rest of the chain
// dispatches. It never skips Next — the deny is per COMMAND, made by the
// shipped auditHandler when a command's atlas effects exceed the
// cap riding this ctx; a body that dispatches nothing over the cap runs
// unchanged.
func guardDecorator(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
	if len(args) != 1 || (args[0].Name != "" && args[0].Name != "effects") {
		return fmt.Errorf("guard takes exactly one argument, effects: %q", "read,net")
	}
	cap, err := advice.ParseCap(args[0].Value)
	if err != nil {
		return err
	}
	c.Next(advice.WithCap(ctx, cap))
	return nil
}

// retryDecorator re-runs the rest of the chain until it succeeds or the
// bounded attempts are spent, waiting autoretry's default backoff (or a fixed
// source-given one) between attempts. Cancellation ends the loop immediately
// and quietly: the last observed status stands, and a Ctrl-C is not a
// decorator diagnostic.
func retryDecorator(ctx context.Context, c *nativeDecoratorCall, args []interp.DecoratorArg) error {
	// Decision of record: retry is never advisable. The rules loader refuses
	// it; this refusal covers any other AdviceFunc an embedder installs.
	if c.Advised != "" {
		return errors.New("retry is source-only and never applied by advice")
	}
	attempts, fixed, err := retryPolicy(args)
	if err != nil {
		return err
	}
	for attempt := 1; ; attempt++ {
		c.Next(ctx)
		if c.Status == 0 || attempt >= attempts || ctx.Err() != nil {
			return nil
		}
		delay := autoretry.Backoff(attempt)
		if fixed >= 0 {
			delay = fixed
		}
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return nil
			case <-timer.C:
			}
		}
	}
}

// retryPolicy resolves @retry's arguments: n (total attempts, default
// autoretry.MaxAttempts) and backoff (a fixed duration overriding the default
// escalating schedule). Positional order matches the documented signature
// retry(c, n, backoff).
func retryPolicy(args []interp.DecoratorArg) (attempts int, fixed time.Duration, err error) {
	attempts, fixed = autoretry.MaxAttempts, -1
	positional := 0
	seen := map[string]bool{}
	for _, a := range args {
		name := a.Name
		if name == "" {
			switch positional {
			case 0:
				name = "n"
			case 1:
				name = "backoff"
			}
			positional++
		}
		if seen[name] {
			return 0, 0, fmt.Errorf("retry argument %q was supplied more than once", name)
		}
		seen[name] = true
		switch name {
		case "n":
			v, convErr := strconv.Atoi(strings.TrimSpace(a.Value))
			if convErr != nil || v < 1 || v > retryAttemptLimit {
				return 0, 0, fmt.Errorf("retry n must be an integer in 1..%d, got %q", retryAttemptLimit, a.Value)
			}
			attempts = v
		case "backoff":
			d, convErr := time.ParseDuration(strings.TrimSpace(a.Value))
			if convErr != nil || d < 0 {
				return 0, 0, fmt.Errorf("retry backoff must be a non-negative duration (e.g. %q), got %q", "1s", a.Value)
			}
			fixed = d
		default:
			return 0, 0, fmt.Errorf("retry takes n and backoff, not %q", name)
		}
	}
	return attempts, fixed, nil
}

// newAdviceCallback returns the interp.Advice registration callback. The
// rules load LAZILY, on the first registration the engine consults advice
// for — and the engine consults it only while Bash++ is active — so:
//
//   - a plain-Bash or POSIX script never opens the rules file at all;
//   - a cert run (VSC_PROFILE=cert) never does either, by FromEnv's own
//     contract, whatever BASHY_ADVICE names;
//   - a configured file that fails to load warns once and refuses all calls —
//     a policy that silently does not apply is the absence-of-evidence
//     failure this codebase keeps re-finding, so the off-state is spoken.
//
// Preamble exclusion is structural in this shell: the preamble's bare-name
// shims are installed by registerDefaultFuncs writing Runner.Funcs directly,
// which never crosses the engine's registration seam, so a bare "*" rule
// reaches only functions user code registered. Idempotence and ordering are
// the engine's: advised specs apply outermost, in the file order For()
// returns, deduplicated by the stable rule ID on every re-registration.
//
// With attest on (attest.go), every AGENTIC registration additionally gets the
// native `attest` rung, outermost, so an agentic{} function with no decorator
// of its own still completes into the evidence ledger. It rides this seam
// because it is the one place the engine tells the host about a registration
// — and only while Bash++ is active, which is what keeps Bash OFF an identity.
func newAdviceCallback(env []string, stderr io.Writer, attest bool) interp.AdviceFunc {
	var once sync.Once
	var rules *advice.Rules
	var loadErr error
	return func(name, file string, agentic bool) []interp.DecoratorSpec {
		once.Do(func() {
			loaded, err := advice.FromEnv(env)
			if err != nil {
				loadErr = err
				fmt.Fprintf(stderr, "bashy: advice: %v — advised calls refused\n", err)
				return
			}
			rules = loaded
		})
		if loadErr != nil {
			return []interp.DecoratorSpec{{ID: "invalid-policy", Name: "__bashy_invalid_advice"}}
		}
		var specs []interp.DecoratorSpec
		if attest && agentic {
			specs = append(specs, interp.DecoratorSpec{ID: attestAdviceID, Name: "attest"})
		}
		advised := rules.For(advice.Query{Name: name, File: file, Agentic: agentic})
		if len(advised) == 0 {
			return specs
		}
		for _, a := range advised {
			spec := interp.DecoratorSpec{ID: a.RuleID, Name: a.Spec.Decorator}
			for _, arg := range a.Spec.Args {
				spec.Args = append(spec.Args, interp.DecoratorArg{Name: arg.Name, Value: adviceArgValue(arg.Value)})
			}
			specs = append(specs, spec)
		}
		return specs
	}
}

// adviceArgValue renders a rules-file value as the RAW native value a
// DecoratorArg carries. advice.Value.String() spells the value the way a
// decorator LINE does — strings %q-quoted — and a quoted `"read,net"` handed
// to ParseCap is a load-time-validated rule failing at call time, not a cap.
func adviceArgValue(v advice.Value) string {
	switch v.Kind {
	case advice.KindInt:
		return strconv.FormatInt(v.Int, 10)
	case advice.KindBool:
		return strconv.FormatBool(v.Bool)
	default:
		return v.Str
	}
}
