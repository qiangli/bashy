package agentos

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/qiangli/yoke/external/registry"
	"github.com/qiangli/yoke/pkg/atlas"
	"github.com/qiangli/yoke/pkg/policy/advice"
	"mvdan.cc/sh/v3/interp"
)

// text_fences.go — bashy's side of the Bash# text fences (B30,
// bashsharp/docs/fenced-text-blocks-plan.md). The engine owns the grammar,
// the rows and the runtimes; bashy supplies what only the host shell has:
//
//   - the PROCESSORS. A text row names its tool (podman, tofu, kubectl,
//     helm) and the fence resolves it through the island tool resolver, never
//     PATH. Here every such tool is bashy's own front-door verb, re-entered
//     through this executable: `bashy podman` provisions the embedded engine
//     and its helpers, `bashy tofu` is a registry row — pinned release,
//     digest-verified, cached — so the fence inherits exactly the provisioning
//     and environment handling the verb has.
//   - the EFFECT GATE. A verb that declares effect atoms (`apply` → net,
//     write, spend) is the author's side of the guard coin, exactly as
//     `@effects` is: under a `@guard` cap the declaration exceeds, the call
//     is denied at the boundary with status 126 before the processor runs.
//   - the REGISTERED-RUNNER predicate. A `!runner` may name a command bashy
//     dispatches itself — an atlas verb or tool, an operator-registered
//     command, a registry row — and nothing on PATH.
//
// All three are off under the certification profile, where the engine keeps
// its standalone behavior (PATH tools, no cap, functions and builtins only).

// fenceTools are the text-row processors, each answered by bashy's own verb.
var fenceTools = []string{"podman", "tofu", "kubectl", "helm", "skills", "cargo", "uv", "npm", "cmake", "go", "make"}

func selfVerb(verb string) func(ctx context.Context) ([]string, string, error) {
	return func(context.Context) ([]string, string, error) {
		exe, err := os.Executable()
		if err != nil {
			return nil, "", fmt.Errorf("bashy %s: cannot locate this executable: %v", verb, err)
		}
		return []string{exe, verb}, "selected bashy " + verb + " (provisioned front-door verb)", nil
	}
}

func init() {
	for _, tool := range fenceTools {
		islandToolchains[tool] = selfVerb(tool)
	}
}

// fenceEffectGate denies a declared-effects foreign call that exceeds the
// cap riding its context — the same decision and wording effectsDecorator
// makes for a function's own declaration.
func fenceEffectGate(ctx context.Context, qualified string, effects []string) error {
	outer, ok := advice.CapFrom(ctx)
	if !ok {
		return nil
	}
	if over := outer.Exceeded(effects); len(over) > 0 {
		return fmt.Errorf("%s: declared effects %s exceed the guard (%s not allowed by %s)",
			qualified, strings.Join(effects, ","), strings.Join(over, ","), outer)
	}
	return nil
}

// fenceRunnerRegistered reports whether bashy dispatches a command of that
// name itself: an atlas verb or tool, an operator-registered command, or a
// managed-external registry row. PATH programs are never runners.
func fenceRunnerRegistered(name string) bool {
	if _, ok := atlas.Lookup(name); ok {
		return true
	}
	if _, ok := registry.Lookup(name); ok {
		return true
	}
	for _, registered := range registeredNames() {
		if registered == name {
			return true
		}
	}
	return false
}

// installFenceSeams wires the gate and the runner predicate into the engine
// once per process, beside the island tool resolver.
func installFenceSeams() {
	if certProfile() {
		return
	}
	interp.ForeignEffectGate = fenceEffectGate
	interp.FenceRunnerRegistered = fenceRunnerRegistered
}
