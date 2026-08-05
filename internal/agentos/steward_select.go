// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/capability"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/llmbudget"
)

// WHO SHOULD STEWARD THIS HOST, WHEN NOBODY SAID.
//
// `bashy agents list --min-band 4` already answers "who is strong enough". It
// does not answer "which of them should I spend", and for a steward that second
// question is the whole decision: a steward is not a task, it is a process that
// stays up, so it consumes its model's quota for as long as the host is
// attended. Picking the alphabetically-first frontier agent every time would
// drain one seat and leave the rest idle.
//
// The ranking below is therefore a COST-AND-QUOTA decision made on band-eligible
// candidates, in this order:
//
//  1. billing class — a seat you have already paid for before a metered API key.
//     This is the fleet's standing rule (prefer subscriptions; fall back to
//     metered only when the flat plans are spent), applied automatically instead
//     of being remembered.
//  2. measured headroom — among equals, the one with the most quota left, read
//     from the SAME meter the budget gate enforces. An agent the gate would
//     block is not a candidate at all.
//  3. band, then reliability, then name — so the choice is deterministic and a
//     rerun on an unchanged host picks the same agent.
//
// UNKNOWN HEADROOM IS NOT FULL HEADROOM. A model with no measured ceiling sorts
// BELOW one with a known ceiling and room to spare, and ABOVE one measurably
// close to its limit. It is never treated as infinite, because "no limit
// recorded" and "no limit" are different facts and only one of them is good news.

// stewardBandFloor is the lowest band that can actually hold the seat.
//
// It is not a style preference. The steward's job is judgement under evidence —
// partition authority, appoint and qualify conductors, refuse a claim that has
// no proof behind it — and the measured failure of a sub-L3 agent in an
// orchestrating seat is not that it works badly, it is that it LOOPS: it repeats
// the same tool calls, never converges, and reports success anyway. A host
// stewarded by one is worse than an unstewarded host, because it looks attended.
const stewardBandFloor = 3

// stewardDefaultBand is the band `start` selects from when the operator names
// neither an agent nor a band.
const stewardDefaultBand = 4

// stewardCandidate is one agent the selector considered, with everything the
// decision was made on — kept so the choice can be EXPLAINED rather than
// asserted.
type stewardCandidate struct {
	Name        string
	Nick        string
	Binding     string
	Tool        string
	Model       string
	Band        int
	BandSource  string
	Billing     string
	Reliability string

	// HeadroomKnown/Headroom is the fraction of the binding quota still
	// available (1.0 = untouched). Known=false means the meter has no ceiling
	// for this model — see the note above about why that is not 1.0.
	HeadroomKnown bool
	Headroom      float64
	MeterBasis    string
	MeterDetail   string

	MarginalCostMicro int64
}

// stewardSkip is a candidate that was excluded, and why. Every exclusion is
// reported: an agent that silently vanishes from a roster is indistinguishable
// from one that was never registered.
type stewardSkip struct {
	Name   string
	Reason string
}

// stewardSelection is the outcome of a pick.
type stewardSelection struct {
	Chosen    stewardCandidate
	Runners   []stewardCandidate
	Skipped   []stewardSkip
	Why       string
	BandFloor bool // the chosen band is below stewardBandFloor
	Explicit  bool // the operator named the agent; no ranking was applied
}

// selectStewardAgent resolves the agent that will hold the seat.
//
// A named --agent wins outright and is only CHECKED (does it resolve, is its
// tool operable, is its band sufficient) — never overridden. An operator who
// names an agent has a reason the catalog cannot see.
func selectStewardAgent(name, tool string, band int) (*stewardSelection, error) {
	cat := fleet.New()

	if n := strings.TrimSpace(name); n != "" {
		if tool != "" || band != 0 {
			return nil, fmt.Errorf("steward start: give --agent OR --band/--tool, not both — " +
				"--agent names one, --band/--tool pick one for you")
		}
		a, t, m, err := cat.Binding(n)
		if err != nil {
			return nil, fmt.Errorf("steward start: %w (`bashy agents list` shows the fleet)", err)
		}
		if ok, reason := capability.Operable(t.Name); !ok {
			return nil, fmt.Errorf("steward start: %s is not routable on this host: %s", n, reason)
		}
		c := describeStewardCandidate(cat, a, t, m)
		return &stewardSelection{
			Chosen:    c,
			Explicit:  true,
			BandFloor: c.Band < stewardBandFloor,
			Why:       "named by the operator",
		}, nil
	}

	if band == 0 {
		band = stewardDefaultBand
	}
	if band < 1 || band > fleet.MaxBand {
		return nil, fmt.Errorf("steward start: --band %d out of range (1-%d)", band, fleet.MaxBand)
	}

	agents, _ := cat.Agents()
	sel := &stewardSelection{}
	for _, a := range agents {
		if a.Ephemeral {
			// A clone minted for one task is a worker, not a candidate for the
			// host's standing seat.
			continue
		}
		_, t, m, err := cat.Binding(a.Name)
		if err != nil {
			continue // dangling: no band selects it, and `agents verify` owns that report
		}
		c := describeStewardCandidate(cat, a, t, m)
		if c.Band < band {
			continue
		}
		if ok, reason := capability.Operable(t.Name); !ok {
			sel.Skipped = append(sel.Skipped, stewardSkip{a.Name, reason})
			continue
		}
		// THE METER, NOT A GUESS. If the budget gate would refuse this model's
		// next turn, it cannot steward — a seat that blocks on its first write
		// is an unattended host wearing a steward's name.
		if d := llmbudget.Check(m.Name, 0); !d.Allowed() {
			sel.Skipped = append(sel.Skipped, stewardSkip{a.Name, "budget gate: " + d.Reason})
			continue
		}
		sel.Runners = append(sel.Runners, c)
	}

	if len(sel.Runners) == 0 {
		hint := ""
		if len(sel.Skipped) > 0 {
			var parts []string
			for _, s := range sel.Skipped {
				parts = append(parts, s.Name+" ("+s.Reason+")")
			}
			hint = " — skipped: " + strings.Join(parts, ", ")
		}
		return nil, fmt.Errorf("steward start: no operable agent at band L%d or above%s. "+
			"`bashy agents list --min-band %d` shows the roster; --band lowers the bar "+
			"(and --agent names one outright)", band, hint, band)
	}

	sort.SliceStable(sel.Runners, func(i, j int) bool { return lessStewardCandidate(sel.Runners[i], sel.Runners[j]) })
	sel.Chosen = sel.Runners[0]
	sel.Runners = sel.Runners[1:]
	sel.BandFloor = sel.Chosen.Band < stewardBandFloor
	sel.Why = explainStewardChoice(sel.Chosen, band)
	return sel, nil
}

func describeStewardCandidate(cat *fleet.Catalog, a fleet.Agent, t fleet.Tool, m fleet.Model) stewardCandidate {
	c := stewardCandidate{
		Name: a.Name, Nick: a.NickName(), Binding: a.MatrixKey(),
		Tool: t.Name, Model: m.Name,
		Band: m.Band, BandSource: m.BandSource,
		Billing:           m.BillingMode(),
		MarginalCostMicro: m.MarginalCostMicro(),
	}
	if a.Ledger != nil {
		c.Reliability = a.Ledger.Reliability
	}
	// A CASCADE agent serves the band its ladder REACHES, not its base model's
	// peg — the same correction `bashy agents list` makes.
	if a.BandSource == fleet.BandCascade && a.Band > 0 {
		c.Band, c.BandSource = a.Band, a.BandSource
	}
	st := llmbudget.Status(m.Name)
	c.MeterBasis = st.Basis
	if st.LimitKnown && st.Limit != nil && *st.Limit > 0 && st.Remaining != nil {
		c.HeadroomKnown = true
		c.Headroom = float64(*st.Remaining) / float64(*st.Limit)
		if c.Headroom < 0 {
			c.Headroom = 0
		}
		c.MeterDetail = fmt.Sprintf("%d of %d %s left", *st.Remaining, *st.Limit, st.Unit)
	} else {
		c.MeterDetail = fmt.Sprintf("no ceiling recorded (%d %s spent)", st.Spent, st.Unit)
	}
	_ = cat
	return c
}

// billingRank orders the ways a turn is paid for, cheapest commitment first.
// Free (your own hardware) < flat (a seat already bought, hard quota) <
// flat_then_metered (a seat already bought that starts charging when spent) <
// metered (every token is money) < unknown.
func billingRank(mode string) int {
	switch mode {
	case fleet.BillingFree:
		return 0
	case fleet.BillingFlat:
		return 1
	case fleet.BillingFlatThenMetered:
		return 2
	case fleet.BillingMetered:
		return 3
	default:
		return 4
	}
}

// headroomRank buckets quota so an unmeasured model cannot outrank a measured
// one that is genuinely fresh, nor be dismissed below one that is nearly spent.
//
//	0  measured, plenty left (>= 50%)
//	1  measured, some left (>= 20%)
//	2  no ceiling recorded — an absence, ranked in the middle deliberately
//	3  measured, nearly spent (< 20%)
func headroomRank(c stewardCandidate) int {
	if !c.HeadroomKnown {
		return 2
	}
	switch {
	case c.Headroom >= 0.5:
		return 0
	case c.Headroom >= 0.2:
		return 1
	default:
		return 3
	}
}

func lessStewardCandidate(a, b stewardCandidate) bool {
	if ra, rb := billingRank(a.Billing), billingRank(b.Billing); ra != rb {
		return ra < rb
	}
	if ha, hb := headroomRank(a), headroomRank(b); ha != hb {
		return ha < hb
	}
	if a.HeadroomKnown && b.HeadroomKnown && a.Headroom != b.Headroom {
		return a.Headroom > b.Headroom
	}
	if a.Band != b.Band {
		return a.Band > b.Band
	}
	if ra, rb := reliabilityRank(a.Reliability), reliabilityRank(b.Reliability); ra != rb {
		return ra < rb
	}
	if a.MarginalCostMicro != b.MarginalCostMicro {
		return a.MarginalCostMicro < b.MarginalCostMicro
	}
	return a.Name < b.Name
}

// reliabilityRank orders the ledger's measured verdict. UNMEASURED IS NOT
// RELIABLE: it sorts last among the three, because an agent nobody has scored is
// not evidence of anything, and this is the seat where believing otherwise costs
// the most.
func reliabilityRank(r string) int {
	switch strings.ToLower(strings.TrimSpace(r)) {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 3
	default:
		return 2
	}
}

func explainStewardChoice(c stewardCandidate, band int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "strongest cost-aware match at band L%d+: ", band)
	switch c.Billing {
	case fleet.BillingFree:
		b.WriteString("runs on your own hardware")
	case fleet.BillingFlat, fleet.BillingFlatThenMetered:
		b.WriteString("a subscription seat you have already paid for")
	case fleet.BillingMetered:
		b.WriteString("metered — no flat-rate seat at this band was available")
	default:
		b.WriteString("billing not declared")
	}
	if c.HeadroomKnown {
		fmt.Fprintf(&b, "; %.0f%% quota left (%s)", c.Headroom*100, c.MeterDetail)
	} else {
		fmt.Fprintf(&b, "; quota headroom UNKNOWN (%s)", c.MeterDetail)
	}
	return b.String()
}
