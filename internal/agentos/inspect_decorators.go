// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"sort"
)

// decoratorCatalog is the one description of the predefined decorator set —
// what `bashy inspect decorators` prints and what docs/decorators.md must
// match (a test pins the two to nativeDecoratorSet's keys). "Minimum" marks
// the catalogued set: what dag needs or a script author reaches for
// constantly. Everything predefined is redefinable: a script function of the
// same name shadows the native.
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
	{"guard", "effects: \"read,net\"", "the effect cap for everything the call dispatches; a command over the cap is denied by the audit handler when BASHY_AUDIT is on", "atlas effects, dag Effects: (advisory there)", "126", true, true},
	{"trace", "—", "one OTel span `call <name>` around the call; argument count only, never values", "bashy otel", "—", true, true},
	{"retry", "n: 3, backoff: \"1s\"", "re-runs the chain until it succeeds or n attempts are spent", "pkg/autoretry", "the last attempt's", false, true},
	{"timeout", "\"10s\" | d: \"10s\"", "cancels the chain at the deadline; under @retry each attempt re-arms", "context deadline", "124", false, true},
	{"memo", "[\"1h\" | ttl: \"1h\"]", "same name + args in one process returns the cached Results and Status without running the body; failures are not cached; printed output is not replayed", "process-wide map", "the cached", false, true},
	{"auth", "via: \"<cmd>\", as: \"<principal>\"", "runs via once per process (default: bashy tessaro status); exit 0 = authenticated, first stdout line = principal, exported as BASHY_PRINCIPAL; as: must match", "bashy login / tessaro status", "77", true, true},
	{"confirm", "—", "effect-derived --what-if / --confirm per operation; high-impact atoms need an answer or the call yields", "atlas effects, bashy ask", "6", false, false},
	{"attest", "—", "pass-through rung advice puts on agentic{} functions so the call is attested; not for authors", "craft ledger", "—", true, false},
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
	fmt.Println("bashy inspect decorators — the predefined Bash# set (a script `func <name>(c *Call)` shadows any of them):")
	for _, d := range rows {
		tag := ""
		if !d.Minimum {
			tag = "  (present, not in the minimum set)"
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
