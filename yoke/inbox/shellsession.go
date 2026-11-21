package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/weave"
)

// fleetAgentNames lists the address book — every agent the catalog knows.
//
// It is the input to `bus subscriptions --reconcile`: the fleet IS the set that
// needs inboxes, because `bashy agent list` is what a human or an agent reads
// when deciding who to message. A catalog that will not load yields no names
// rather than an error; reconciliation is a repair, and a repair that fails
// loudly on a read it did not need is worse than one that does nothing.
func fleetAgentNames() []string {
	agents, _ := fleet.New().Agents()
	out := make([]string, 0, len(agents))
	for _, a := range agents {
		if n := strings.TrimSpace(a.Name); n != "" {
			out = append(out, n)
		}
	}
	return out
}

// fleetSelectAudience answers a `bashy mb send --band/--tool/--provider`
// selector from the CATALOG — the same catalog `bashy agent list` prints.
//
// That identity is the point. A selector resolved from a copy kept elsewhere is
// a second opinion about who is L4, and a second opinion can drift; answering
// from the catalog means the address book and the audience can never disagree.
//
// Criteria are ANDed, matching the flag documentation. A cascade agent has no
// plain model binding (Base is set and Model ignored), so a provider or band
// filter cannot describe it and it is skipped rather than guessed at — the same
// no-attribution-no-row rule the capability ledger uses.
func fleetSelectAudience(aud bus.Audience) ([]string, error) {
	if err := aud.Validate(); err != nil {
		return nil, err
	}
	// ROLE is answered from the SPRINT RECORDS, not the catalog, because it
	// selects on what an agent is DOING rather than on its binding. This is
	// the seam sprint #139 was blocked on: pkg/bus is transport and must not
	// learn to read the sprint store, so the host joins the two.
	//
	// Role and binding filters are mutually exclusive. Validate above refuses
	// mixed selectors before either roster can silently override a criterion.
	if role := strings.TrimSpace(aud.Role); role != "" {
		if !strings.EqualFold(role, "conductor") {
			return nil, fmt.Errorf("unknown role %q: the only role with a live roster is 'conductor' (the seated sprint managers)", role)
		}
		return weave.LiveSprintManagers()
	}
	cat := fleet.New()
	agents, errs := cat.Agents()
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("audience: resolve fleet catalog: %w", err)
	}
	var out []string
	for _, a := range agents {
		if aud.Tool != "" && !strings.EqualFold(a.Tool, aud.Tool) {
			continue
		}
		if aud.Band != 0 || aud.Provider != "" || aud.Family != "" || aud.Version != "" {
			m, ok := cat.Model(a.Model)
			if !ok {
				continue // cascade or unresolvable binding: not describable, not guessed
			}
			if aud.Band != 0 && m.Band != aud.Band {
				continue
			}
			if aud.Provider != "" && !strings.EqualFold(m.Provider, aud.Provider) {
				continue
			}
			if aud.Family != "" && !strings.EqualFold(m.Family, aud.Family) {
				continue
			}
			if aud.Version != "" && !strings.EqualFold(m.Version, aud.Version) {
				continue
			}
		}
		if n := strings.TrimSpace(a.Name); n != "" {
			out = append(out, n)
		}
	}
	return out, nil
}

// fleetResolveAgentName maps any spelling of an agent to its canonical fleet
// name — the one `bashy agent list` prints and `mb send` addresses.
//
// This is what lets a bare `bashy mb` work. A reader's environment does not
// carry its fleet name: a bashy-launched agent has
// BASHY_PRINCIPAL=dhnt:agent/<Nick>, and everything else falls back to $USER.
// Posts are addressed to the fleet name, so without this the send and read
// sides resolve different identities and a post lands where its recipient never
// looks — which is precisely what happened before this existed.
//
// An unknown string comes back EMPTY rather than guessed at, so the caller can
// fall through to using it verbatim: a human at a terminal is a legitimate board
// participant under their login name, not an agent to be resolved.
func fleetResolveAgentName(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Canonicalize any PRINCIPAL, not agents only.
	//
	// This is the board's one name-folding hook: bus.BoardIdentity routes every
	// sender and every addressee through it. Resolving agents alone meant a
	// human's name never folded, so the operator was whatever string each
	// surface happened to hand over — `qiangli` from the CLI, the cloud handle
	// from the browser — and their posts and their inbox split between the two.
	// An agent got one canonical name; a person got none.
	if canonical, _, ok := fleet.New().CanonicalPrincipal(s); ok {
		return canonical
	}
	return ""
}
