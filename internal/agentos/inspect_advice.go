// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// inspect_advice.go — `bashy inspect advice`: the advice (policy-applied
// decorator) configuration, one row per active rule.
//
// An aspect of inspect, never a verb: a read-only self-view (subject = bashy,
// effects = {read}). And it is NOT a second policy parser: the decision comes
// from advice.FromEnv(os.Environ()) — the same loader the interpreter's advice
// wiring goes through — and the rows from Rules.All, the accessor the package
// publishes for exactly this surface. This file only renders what that call
// decided, and names the signal that decided it (the mode aspect's rule: the
// explanation is derived, the decision is not).
//
// The off-states are answers, not absences: advice is opt-in (BASHY_ADVICE
// unset → off, the default) and ALWAYS off under VSC_PROFILE=cert — a cert run
// carries zero rules and never even opens the named file, so `inspect advice`
// under cert with a garbage BASHY_ADVICE still exits 0 with state=off. A
// configured file that fails to load is an error (exit 1), not a silent off.
package agentos

import (
	"fmt"
	"os"
	"strings"

	"github.com/qiangli/yoke/pkg/policy/advice"
)

// inspectAdviceArg is one decorator argument, rendered the way the decorator
// line spells it (strings quoted, ints and bools bare) so the arg column and
// the spec column can never disagree.
type inspectAdviceArg struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// inspectAdviceRuleRow is one active rule in file order. Selectors carry
// omitempty because empty IS the answer ("not selecting on this axis");
// PreambleExcluded is always stated since both of its values are meaningful
// (the default excludes the host preamble; "exclude": [] opts in).
type inspectAdviceRuleRow struct {
	ID               string             `json:"id"`
	Name             string             `json:"name,omitempty"`
	File             string             `json:"file,omitempty"`
	Agentic          *bool              `json:"agentic,omitempty"`
	PreambleExcluded bool               `json:"preamble_excluded"`
	Decorator        string             `json:"decorator"`
	Args             []inspectAdviceArg `json:"args,omitempty"`
	Spec             string             `json:"spec"` // the decorator line the rule stands for
}

// inspectAdviceReport is the advice aspect's rows payload. State and
// decided_by come first because "off" must be distinguishable from "on, zero
// rules" — an agent diffing two hosts needs to know which one never loaded a
// file and which one loaded an empty one.
type inspectAdviceReport struct {
	State     string                 `json:"state"`      // on | off
	DecidedBy string                 `json:"decided_by"` // the signal that decided the state
	Rules     []inspectAdviceRuleRow `json:"rules"`      // active rules, file order; empty when off
}

// collectInspectAdvice renders the decision advice.FromEnv made. The switch
// mirrors FromEnv's own precedence only to label the off-reason (cert vs
// unset); it never re-reads or re-validates the rules file — FromEnv's return
// value is the single decision, exactly as the gate deciders are for `mode`.
func collectInspectAdvice() (inspectAdviceReport, int) {
	rules, err := advice.FromEnv(os.Environ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy inspect advice: %v\n", err)
		return inspectAdviceReport{}, 1
	}
	rep := inspectAdviceReport{State: "on", Rules: []inspectAdviceRuleRow{}}
	// The same env reads FromEnv made, verbatim: cert is an exact match.
	profile, hasProfile := os.LookupEnv("VSC_PROFILE")
	path, hasPath := os.LookupEnv(advice.EnvVar)
	switch {
	case hasProfile && profile == "cert":
		rep.State = "off"
		rep.DecidedBy = "VSC_PROFILE=cert (certification run; zero rules"
		if hasPath && strings.TrimSpace(path) != "" {
			rep.DecidedBy += "; " + advice.EnvVar + "=" + path + " is never opened"
		}
		rep.DecidedBy += ")"
	case rules == nil: // not cert, so BASHY_ADVICE is unset or empty
		rep.State = "off"
		rep.DecidedBy = advice.EnvVar + " unset (advice is opt-in; default off)"
	default:
		rep.DecidedBy = inspectEnvSignal(advice.EnvVar, "")
		for _, r := range rules.All() {
			row := inspectAdviceRuleRow{
				ID:               r.ID,
				Name:             r.Name,
				File:             r.File,
				Agentic:          r.Agentic,
				PreambleExcluded: r.ExcludePreamble,
				Decorator:        r.Spec.Decorator,
				Spec:             r.Spec.String(),
			}
			for _, a := range r.Spec.Args { // already name-sorted by the package
				row.Args = append(row.Args, inspectAdviceArg{Name: a.Name, Value: a.Value.String()})
			}
			rep.Rules = append(rep.Rules, row)
		}
	}
	return rep, 0
}

// printInspectAdvice renders the report for a human: one line per rule, the
// stable ID, the selectors that chose it, and the decorator line it stands for.
func printInspectAdvice(rep inspectAdviceReport) {
	if rep.State == "off" {
		fmt.Printf("bashy inspect advice — off (%s)\n", rep.DecidedBy)
		return
	}
	fmt.Printf("bashy inspect advice — on (%s), %d rules:\n", rep.DecidedBy, len(rep.Rules))
	if len(rep.Rules) == 0 {
		fmt.Println("  none — the rules file carries zero rules")
		return
	}
	wID, wSel := 0, 0
	sels := make([]string, len(rep.Rules))
	for i, r := range rep.Rules {
		sels[i] = inspectAdviceSelectors(r)
		if len(r.ID) > wID {
			wID = len(r.ID)
		}
		if len(sels[i]) > wSel {
			wSel = len(sels[i])
		}
	}
	for i, r := range rep.Rules {
		fmt.Printf("  %-*s  %-*s  %s\n", wID, r.ID, wSel, sels[i], r.Spec)
	}
}

// inspectAdviceSelectors renders a rule's selectors as one column: each set
// axis with its value, and the preamble opt-in when it deviates from the
// default. Validation guarantees at least one axis is set.
func inspectAdviceSelectors(r inspectAdviceRuleRow) string {
	var parts []string
	if r.Name != "" {
		parts = append(parts, "name "+r.Name)
	}
	if r.File != "" {
		parts = append(parts, "file "+r.File)
	}
	if r.Agentic != nil {
		parts = append(parts, fmt.Sprintf("agentic=%t", *r.Agentic))
	}
	if !r.PreambleExcluded {
		parts = append(parts, "preamble in scope")
	}
	return strings.Join(parts, " · ")
}
