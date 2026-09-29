// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/policy/advice"
	"github.com/qiangli/yoke/pkg/policy/audit"
)

// The compliance audit: a tamper-evident, hash-chained record of every command
// the shell dispatches, with agent attribution and secrets stripped. It is the
// evidence half of the security uplift — the artifact a security team needs to
// answer "what did the agents run here, and prove it." It records; it never
// blocks unless a guard effect cap is present, and never reaches across an execve (that is
// the OS sandbox). Opt-in, off by default, and — like every AgentOS feature —
// never linked into the lean `cmd/bash` drop-in or active under --posix.
//
// Layering note: this is distinct from the sh engine's low-level
// BASHY_AUDIT_LOG raw-event hook. That writes one AuditEvent per simple command
// (builtins included) with no outcome; this writes the enriched, chained
// compliance record (resolved argv, atlas effects, exit, duration, actor). Two
// layers, two env vars.

// auditEnabled reports whether BASHY_AUDIT turns the compliance audit on. The
// value is either a boolean (default path) or an explicit log path.
func auditEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BASHY_AUDIT"))) {
	case "", "0", "false", "off", "no":
		return false
	default:
		return true
	}
}

// auditPath is where the log lives: the BASHY_AUDIT value when it names a path,
// else a per-user default under the bashy home.
func auditPath() string {
	v := strings.TrimSpace(os.Getenv("BASHY_AUDIT"))
	switch strings.ToLower(v) {
	case "1", "true", "on", "yes":
		// a boolean-on: use the default path
	default:
		if v != "" {
			return v // an explicit path
		}
	}
	if home := strings.TrimSpace(os.Getenv("BASHY_HOME")); home != "" {
		return filepath.Join(home, "audit", "audit.jsonl")
	}
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return filepath.Join(h, ".bashy", "audit", "audit.jsonl")
	}
	return filepath.Join(os.TempDir(), "bashy-audit", "audit.jsonl")
}

// effectsFor returns the Command Atlas security effects for a dispatched
// command name, so each audit record is self-describing about what the command
// could do. An external command the atlas does not know contributes no effects
// — recorded honestly rather than guessed.
func effectsFor(cmd string) []string {
	if e, ok := atlas.Lookup(baseName(cmd)); ok {
		return e.Effects
	}
	return nil
}

// effectsForCtx is effectsFor with the author's word as the fallback: inside
// an @effects function a command the atlas does not know carries the
// declaration (advice.WithVouch) instead of "unknown".
func effectsForCtx(ctx context.Context, cmd string) []string {
	if e := effectsFor(cmd); e != nil {
		return e
	}
	if v, ok := advice.VouchFrom(ctx); ok {
		return v
	}
	return nil
}

// auditHandler is the outermost ExecHandler middleware: it runs the command,
// then appends one chained record capturing the resolved argv (secrets masked),
// the atlas effects, the outcome, and the actor. It always returns the
// command's real result unchanged unless a guard cap denies execution. Caps
// are enforced even with a nil audit writer; logging itself is best-effort.
func auditHandler(w *audit.Writer, actor audit.Actor, host string) func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 {
				return next(ctx, args)
			}
			start := time.Now()

			effects := containedEffects(ctx, args[0], effectsForCtx(ctx, args[0]))
			decision := "allow"
			var err error

			if cap, ok := advice.CapFrom(ctx); ok {
				if over := cap.Exceeded(effects); len(over) > 0 {
					decision = "deny"
					reportCapDenial(ctx, args[0], effects, over, cap)
				}
			}

			if decision == "deny" {
				// 126 like the @effects boundary denial (capDeniedStatus): the
				// command was found but refused — distinct from the command's
				// own failure, which a silent 1 was indistinguishable from.
				err = interp.ExitStatus(capDeniedStatus)
			} else {
				err = next(ctx, args)
			}

			if w != nil {
				status, _ := exitStatusOf(err)
				argv, masked := audit.Redact(args)
				rec := audit.Record{
					Time:       start.UTC().Format(time.RFC3339Nano),
					Actor:      actor,
					Argv:       argv,
					Binary:     baseName(args[0]),
					Cwd:        safeHandlerDir(ctx),
					Effects:    effects,
					Host:       host,
					Decision:   decision,
					Exit:       status,
					DurationMs: time.Since(start).Milliseconds(),
					Redactions: masked,
				}
				// Best-effort: a failed append must not break the command. It is
				// still notable — the record simply does not land, and Verify will
				// show a shorter chain, not a corrupt one.
				_, _ = w.Append(rec)
			}
			return err
		}
	}
}

// reportCapDenial tells the caller WHY a command was refused under an effect
// cap (@guard / @effects): which effects it needs that the cap lacks, where its
// classification came from, and what would admit it. A denial is never silent
// — an agent that sees only a bare non-zero status retries blind (Sprint 322
// #1169: `python` under @effects("read,exec") exited 1 with empty output three
// times).
func reportCapDenial(ctx context.Context, cmd string, effects, over []string, cap advice.Cap) {
	stderr := safeHandlerStderr(ctx)
	if stderr == nil {
		return
	}
	name := baseName(cmd)
	if len(effects) == 0 {
		fmt.Fprintf(stderr, "bashy: %s: denied by the effect cap %s: its effects are unknown (not in the Command Atlas); "+
			"run it inside an @effects(\"...\") function that declares what it does\n", name, cap)
		return
	}
	why := "declared by the enclosing @effects"
	if e, ok := atlas.Lookup(name); ok {
		why = "the Command Atlas classifies it " + strings.Join(e.Effects, ",")
		if e.Group == atlas.GroupToolchains {
			why += "; a toolchain runs arbitrary code that can write: build outputs, caches such as __pycache__, its own provisioning"
		}
		if len(e.Effects) != len(effects) {
			why += "; net is dropped under @contain"
		}
	}
	fix := "declare " + strings.Join(over, ",") + " in @effects/@guard to run it"
	if slices.Contains(over, atlas.EffNet) && !containNetFrom(ctx) {
		fix += ", or add @contain(net: \"deny\") to run it without network"
	}
	fmt.Fprintf(stderr, "bashy: %s: denied by the effect cap %s: needs %s (%s); %s\n",
		name, cap, strings.Join(over, ","), why, fix)
}

// safeHandlerStderr is the command's stderr, or nil when ctx carries no
// handler context (a handler driven directly, as unit tests do).
func safeHandlerStderr(ctx context.Context) (w io.Writer) {
	defer func() {
		if recover() != nil {
			w = nil
		}
	}()
	return interp.HandlerCtx(ctx).Stderr
}

// newAuditWriter opens the configured audit log, or returns nil (disabled or
// unopenable — auditing degrades to off, it never aborts the shell).
func newAuditWriter() *audit.Writer {
	if !auditEnabled() {
		return nil
	}
	w, err := audit.Open(auditPath())
	if err != nil {
		return nil
	}
	return w
}

// auditActor resolves the accountable identity for this shell session, adding
// the tool/model the fleet launcher recorded on the way in.
func auditActor() audit.Actor {
	a := audit.ActorFromEnv()
	if a.Model == "" {
		a.Model = strings.TrimSpace(os.Getenv("BASHY_AGENT_MODEL"))
	}
	return a
}

func auditHost() string {
	h, _ := os.Hostname()
	return h
}
