// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"sort"
)

// decoratorCatalog is the one description of the supported decorator set —
// what `bashy inspect decorators` prints and what docs/decorators.md must
// match (a test pins the two to nativeDecoratorSet's keys plus the engine's
// own timed, minus the internal attest). "Minimum" marks the supported set:
// what dag needs or a script author reaches for constantly. Everything
// predefined is redefinable: a script function of the same name shadows the
// native.
//
// internalDecorators are registered for the mechanism, not for authors —
// attest is the rung advice attaches to an agentic{} function — and stay out
// of the catalog. engineDecorators are supplied by the sh engine, not this
// registry, and are catalogued because an author may use them.
var (
	internalDecorators = map[string]bool{"attest": true}
	engineDecorators   = map[string]bool{"timed": true}
)

type decoratorEntry struct {
	Name      string `json:"name"`
	Args      string `json:"args"`
	Does      string `json:"does"`
	Connects  string `json:"connects_to"`
	Exit      string `json:"exit"`
	Advisable bool   `json:"advisable"`
	Minimum   bool   `json:"minimum"`
}

var decoratorCatalog = []decoratorEntry{
	{"require", "'<check>', …", "precondition: every check (a shell command) must exit 0 or the body does not run", "dag Require:", "3", false, true},
	{"ensure", "'<check>', …", "postcondition: judged after the body with $STATUS and $RESULT; failing invalidates the result", "dag Ensure:", "3", false, true},
	{"guard", "\"read,net\" | effects: \"…\"", "the effect cap for everything the call dispatches; a command over the cap is denied before it runs; nested guards only narrow", "atlas effects, dag Effects: (advisory there)", "126", true, true},
	{"trace", "—", "one OTel span `call <name>` around the call; argument count only, never values", "bashy otel", "—", true, true},
	{"retry", "n: 3, backoff: \"1s\"", "re-runs the chain until it succeeds or n attempts are spent", "pkg/autoretry", "the last attempt's", false, true},
	{"timeout", "\"10s\" | d: \"10s\"", "cancels the chain at the deadline; under @retry each attempt re-arms", "context deadline", "124", false, true},
	{"memo", "[\"1h\" | ttl: \"1h\"]", "same name + args in one process returns the cached Results and Status without running the body; failures are not cached; printed output is not replayed", "process-wide map", "the cached", false, true},
	{"auth", "via: \"<cmd>\", as: \"<principal>\"", "runs via once per process (default: bashy tessaro status); exit 0 = authenticated, first stdout line = principal, exported as BASHY_PRINCIPAL; as: must match", "bashy login / tessaro status", "77", true, true},
	{"effects", "\"net,write\" | effects: \"…\"", "declares what the function does: denied at the call boundary when a @guard does not allow it; inside, its own cap, and the classification for commands the atlas does not know", "atlas vocabulary; @guard", "126", false, true},
	{"contain", "net: \"deny\"", "runs every external child the function starts with the network enforced off (Linux network namespace, macOS Seatbelt), so a cap without net admits an interpreter; in-process native tools are not children; unsupported OS fails closed", "bashy contain --net deny -- CMD", "125", false, true},
	{"confirm", "—", "human-in-the-loop allow per operation: high-impact atoms (destroy, cred, priv, spend, unknown) take their answer from --confirm=TOKEN:yes / --what-if or the call yields", "atlas effects, bashy ask", "6", false, true},
	{"timed", "—", "measures the call: one `@timed: <fn>: status=N duration=D` line on stderr (engine-supplied)", "the sh engine", "—", false, true},
}

func collectInspectDecorators() []decoratorEntry {
	out := append([]decoratorEntry(nil), decoratorCatalog...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Minimum != out[j].Minimum {
			return out[i].Minimum
		}
		return false
	})
	return out
}

func printInspectDecorators(rows []decoratorEntry) {
	fmt.Println("bashy inspect decorators — the supported Bash# set (a script `func <name>(c *Call)` shadows any of them):")
	for _, d := range rows {
		tag := ""
		if engineDecorators[d.Name] {
			tag = "  (engine-supplied)"
		}
		adv := "author-only"
		if d.Advisable {
			adv = "advisable"
		}
		fmt.Printf("  @%-8s %-32s exit %-16s %s%s\n", d.Name, d.Args, d.Exit, adv, tag)
		fmt.Printf("           %s\n           connects to: %s\n", d.Does, d.Connects)
	}
	fmt.Println("Catalog: docs/decorators.md")
}
