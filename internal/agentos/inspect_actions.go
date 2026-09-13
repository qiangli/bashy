// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// inspect_actions.go — `bashy inspect actions`: the catalog of what this bashy
// can RUN, one row per action, in the one vocabulary pkg/lexicon's action
// facet gives every runnable thing (a command, a skill, an agent binding).
//
// An aspect of inspect, never a verb: it is a read-only self-view (subject =
// bashy, effects = {read}). And it is NOT a second projection: the rows are the
// facets of the SAME lexicon store `bashy define` answers from, listed rather
// than looked up — so the two cannot disagree about what running a name
// amounts to. The generic half only: identity, contract, latitude, authority,
// declared effects, executor. No path, no host, no Location — a facet row is
// shareable, a filesystem layout is not.
package agentos

import (
	"fmt"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/fleet"
	"github.com/qiangli/coreutils/pkg/lexicon"
	coreskills "github.com/qiangli/coreutils/pkg/skills"
)

// inspectActionKinds is the closed --kind vocabulary, in the order the four
// families are taught. lexicon also names dag-target; nothing projects it yet,
// so it is not offered — a filter that can only ever match nothing is a
// promise the output cannot keep.
var inspectActionKinds = []string{lexicon.ActionCommand, lexicon.ActionScript, lexicon.ActionAgent, lexicon.ActionSkill}

// inspectActionRow is one action's generic facet. Field order follows the
// facet's; Name is the concept's label, because a prose-only skill has no
// identity and an agent's identity (tool:model) is not its name.
type inspectActionRow struct {
	Kind            string   `json:"kind"`
	Name            string   `json:"name"`
	Identity        string   `json:"identity,omitempty"`
	Contract        string   `json:"contract"`
	Latitude        string   `json:"latitude"`
	Authority       string   `json:"authority"`
	EffectsDeclared []string `json:"effects_declared,omitempty"`
	Executor        string   `json:"executor"`
}

// inspectActionCounts is the per-family tally `inspect context` carries. Every
// key is always present: a family with zero members (script, today) is stated
// as 0 rather than omitted, so an absent count can never read as "unknown".
type inspectActionCounts struct {
	Command int `json:"command"`
	Script  int `json:"script"`
	Agent   int `json:"agent"`
	Skill   int `json:"skill"`
}

// lexiconSkillRows is the conversion pkg/lexicon leaves to its embedding
// shell: the catalog's skills as executor-free SkillRows (lexicon must not
// import pkg/skills — see lexicon.SkillRow). The catalog is the one
// skillsOptions assembles, so the rows are exactly what `bashy skill list`
// sees; a second source list here would drift from it.
func lexiconSkillRows() []lexicon.SkillRow {
	cat, ps := coreskills.NewCatalog(skillsOptions()...)
	listed, err := cat.List(ps)
	if err != nil {
		return nil
	}
	rows := make([]lexicon.SkillRow, 0, len(listed))
	for _, l := range listed {
		rows = append(rows, lexiconSkillRow(l.Skill))
	}
	return rows
}

// lexiconSkillRow fills one SkillRow from a skills.Skill: name, description,
// the SKILL.md metadata, and the parsed face only when it is valid — an
// invalid face carries no identity, so the row must not claim one.
func lexiconSkillRow(sk coreskills.Skill) lexicon.SkillRow {
	row := lexicon.SkillRow{Name: sk.Name, Description: sk.Description, Meta: sk.Meta}
	if sk.Dhnt.Valid() {
		row.FaceValid = true
		row.Identity = sk.Dhnt.Identity
		row.EffectCap = sk.Dhnt.EffectCap
		row.HasJudgeStep = sk.Dhnt.HasJudgeStep
	}
	return row
}

// actionStore is the lexicon store the action rows are read from: the verbs
// and agent bindings Build projects, the userland (AddStandardTools), and the
// skill catalog (AddSkills). Host is left empty on purpose — a facet is the
// generic half, and no row here renders it.
func actionStore() *lexicon.Store {
	s := lexicon.Build(fleet.New(), verbSynopsis, "", lexicon.Overlay{})
	s.AddStandardTools(atlas.ToolNames(), lexicon.Overlay{})
	s.AddSkills(lexiconSkillRows(), lexicon.Overlay{})
	return s
}

// collectInspectActions lists every concept that has a facet, optionally one
// kind of them. Sorted by kind, then identity, then name — deterministic, so
// two hosts' `inspect actions --json` diff into what one can run and the other
// cannot.
func collectInspectActions(kind string) []inspectActionRow {
	rows := []inspectActionRow{}
	for _, c := range actionStore().Concepts {
		a := c.Action
		if a == nil || (kind != "" && a.Kind != kind) {
			continue
		}
		rows = append(rows, inspectActionRow{
			Kind: a.Kind, Name: c.PrefLabel, Identity: a.Identity,
			Contract: a.Contract, Latitude: a.Latitude, Authority: a.Authority,
			EffectsDeclared: a.EffectsDeclared, Executor: a.Executor,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Kind != rows[j].Kind {
			return rows[i].Kind < rows[j].Kind
		}
		if rows[i].Identity != rows[j].Identity {
			return rows[i].Identity < rows[j].Identity
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

// collectInspectActionCounts tallies the four families from the same rows.
func collectInspectActionCounts() inspectActionCounts {
	var n inspectActionCounts
	for _, r := range collectInspectActions("") {
		switch r.Kind {
		case lexicon.ActionCommand:
			n.Command++
		case lexicon.ActionScript:
			n.Script++
		case lexicon.ActionAgent:
			n.Agent++
		case lexicon.ActionSkill:
			n.Skill++
		}
	}
	return n
}

// validInspectActionKind reports whether k is in the closed --kind vocabulary.
func validInspectActionKind(k string) bool {
	for _, v := range inspectActionKinds {
		if v == k {
			return true
		}
	}
	return false
}

// printInspectActions renders the rows for a human: one line per action, the
// facet's one-liner (contract · latitude · authority · effects · executor)
// after the kind and the name.
func printInspectActions(rows []inspectActionRow, kind string) {
	what := "actions"
	if kind != "" {
		what = kind + " actions"
	}
	fmt.Printf("bashy inspect %s (%d):\n", what, len(rows))
	if len(rows) == 0 {
		if kind != "" {
			fmt.Printf("  none projected yet — %s is a family this host can name but nothing fills today\n", kind)
		}
		return
	}
	w := 0
	for _, r := range rows {
		if len(r.Name) > w {
			w = len(r.Name)
		}
	}
	for _, r := range rows {
		facet := r.Contract + " · " + r.Latitude + " · " + r.Authority
		if len(r.EffectsDeclared) > 0 {
			facet += " · effects " + strings.Join(r.EffectsDeclared, ",")
		}
		id := ""
		if r.Identity != "" && r.Identity != "verb:"+r.Name {
			id = "  [" + r.Identity + "]"
		}
		fmt.Printf("  %-8s %-*s  %s · via %s%s\n", r.Kind, w, r.Name, facet, r.Executor, id)
	}
}
