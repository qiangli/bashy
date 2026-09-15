// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// decorators.go — the native Bash++ decorator set (trace · guard · retry) and
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
//     command whose projected atlas effects exceed it, before the command
//     runs, writing Decision "deny".
//   - @retry(n:, backoff:) → SOURCE-ONLY (never advisable; the rules loader
//     refuses it and the native double-checks), a bounded, cancellation-aware
//     loop over Next() on autoretry.Backoff's default schedule. No new retry
//     engine.
//
// The decorator bodies are deliberately self-contained — they read only
// (ctx, *Call, args) and never Runner state — so the forthcoming shellrt
// decorator-registry slot (compiled programs; sh-side, being added by the sh
// compiler work) can register the same implementations through a thin adapter
// instead of a second native set.
package agentos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/autoretry"
	"github.com/qiangli/coreutils/pkg/policy/advice"
	"github.com/qiangli/coreutils/pkg/policy/audit"
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
func nativeDecorators() map[string]interp.DecoratorFunc {
	return map[string]interp.DecoratorFunc{
		"trace": traceDecorator,
		"guard": guardDecorator,
		"retry": retryDecorator,
	}
}

// traceDecorator emits one `call <name>` span around the rest of the chain,
// following ExecMiddleware's conventions: global provider, no attribute work
// unless the span records, the status as the point. Arg VALUES never become
// attributes — only the count.
func traceDecorator(ctx context.Context, c *interp.Call, args []interp.DecoratorArg) error {
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
// shipped auditHandler when a command's projected atlas effects exceed the
// cap riding this ctx; a body that dispatches nothing over the cap runs
// unchanged.
func guardDecorator(ctx context.Context, c *interp.Call, args []interp.DecoratorArg) error {
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
func retryDecorator(ctx context.Context, c *interp.Call, args []interp.DecoratorArg) error {
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
//   - a configured file that fails to load WARNS once and advises nothing —
//     a policy that silently does not apply is the absence-of-evidence
//     failure this codebase keeps re-finding, so the off-state is spoken.
//
// Preamble exclusion is structural in this shell: the preamble's bare-name
// shims are installed by registerDefaultFuncs writing Runner.Funcs directly,
// which never crosses the engine's registration seam, so a bare "*" rule
// reaches only functions user code registered. Idempotence and ordering are
// the engine's: advised specs apply outermost, in the file order For()
// returns, deduplicated by the stable rule ID on every re-registration.
func newAdviceCallback(env []string, stderr io.Writer) interp.AdviceFunc {
	var once sync.Once
	var rules *advice.Rules
	return func(name, file string, agentic bool) []interp.DecoratorSpec {
		once.Do(func() {
			loaded, err := advice.FromEnv(env)
			if err != nil {
				fmt.Fprintf(stderr, "bashy: advice: %v — no advice applied\n", err)
				return
			}
			rules = loaded
		})
		advised := rules.For(advice.Query{Name: name, File: file, Agentic: agentic})
		if len(advised) == 0 {
			return nil
		}
		specs := make([]interp.DecoratorSpec, 0, len(advised))
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
