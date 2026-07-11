// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"io"
	"strings"

	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/tool"
)

// The `bashy commands` default surface is organized by HOW a command runs —
// bashy's operational reality — with classical provenance as the sub-grouping:
//
//	builtins   — in-process, no fork (the bashy core)
//	             · shell      : bash builtins
//	             · coreutils  : GNU coreutils, pure-Go in-process
//	             · classic    : other classic Unix tools, also in-process (jq/awk/sed/…)
//	external   — provisioned + exec'd as separate downloaded binaries
//	agent/ext  — bashy's own agentic features, sectioned by execution venue
//
// From the shell's point of view a coreutils tool IS a builtin (zero fork —
// the Tier-1 in-process thesis), which is why shell + coreutils + classic sit
// under one "builtins" umbrella. The (coreutils | classic) split is the one
// piece of metadata the atlas does not store directly; it is derived from
// membership in the canonical GNU coreutils set (gnuCoreutilsCommands, the same
// list behind `--gnu`). Everything else — the external/agent split (Subclass)
// and the venue partition (Tier) — comes straight from the atlas records.

// venueOrder is the locked six-venue stack (+ account front door) used to
// order the agent/ext section (dhnt docs/execution-tiers.md).
var venueOrder = []string{
	atlas.TierUserland, atlas.TierWorkspace, atlas.TierSandbox,
	atlas.TierSphere, atlas.TierCluster, atlas.TierCloud, atlas.TierAccount,
}

// commandSections is the `bashy commands` surface grouped by execution class.
type commandSections struct {
	Shell     []string            `json:"shell"`      // bash builtins
	Coreutils []string            `json:"coreutils"`  // GNU coreutils, in-process
	Classic   []string            `json:"classic"`    // other classic tools, in-process
	External  []string            `json:"external"`   // downloaded + exec'd
	Agent     map[string][]string `json:"agent"`      // venue -> native agentic verbs
}

// agentToolGroups are the atlas groups whose in-process tools are bashy's own
// agentic features (code intelligence, the graph, orchestration, knowledge,
// net helpers) rather than classic userland commands — so they belong in the
// agent/ext section even though they resolve as in-process `coreutils`-class
// tools, not front-door verbs.
var agentToolGroups = map[string]bool{
	atlas.GroupCodeIntel: true,
	atlas.GroupOrch:      true,
	atlas.GroupKnowledge: true,
	atlas.GroupNet:       true,
}

// classSections partitions the live catalog into the five presentation classes.
// It reuses liveAtlas (Class/Subclass/Group/Tier per command, name-sorted) so
// the grouping stays in lockstep with the atlas the --view/--json paths report.
func classSections(all bool) commandSections {
	gnu := sliceSet(gnuCoreutilsCommands)
	s := commandSections{Agent: map[string][]string{}}
	for _, r := range liveAtlas(all) {
		switch r.Class {
		case "builtin":
			s.Shell = append(s.Shell, r.Name)
		case "coreutils":
			switch {
			case agentToolGroups[r.Group]:
				// bashy agent tools that happen to run in-process (graph,
				// code-intel, foreman, fetch/browser) — group with the verbs.
				s.Agent[r.Tier] = append(s.Agent[r.Tier], r.Name)
			case gnu[r.Name]:
				s.Coreutils = append(s.Coreutils, r.Name)
			default:
				s.Classic = append(s.Classic, r.Name)
			}
		case "verb":
			// Downloaded, exec'd binaries (managed externals + toolchain
			// provisioners) are "not ours, we just run them"; everything else
			// is a native bashy feature, placed by its venue.
			if r.Subclass == atlas.SubclassManagedExternal || r.Subclass == atlas.SubclassProvisioner {
				s.External = append(s.External, r.Name)
			} else {
				s.Agent[r.Tier] = append(s.Agent[r.Tier], r.Name)
			}
		}
	}
	return s
}

// printClassSections renders the five-section surface. In verbose mode each
// described command gets its one-line synopsis; otherwise names are wrapped
// into compact columns.
func printClassSections(w io.Writer, verbose, all bool) {
	s := classSections(all)
	syn := func(n string) string {
		if t := tool.Lookup(n); t != nil && t.Synopsis != "" {
			return t.Synopsis
		}
		return verbSynopsis[n]
	}

	builtinTotal := len(s.Shell) + len(s.Coreutils) + len(s.Classic)
	fmt.Fprintf(w, "builtins — in-process, no fork (%d):\n", builtinTotal)
	printSubSection(w, "shell", s.Shell, verbose, nil) // builtins carry no synopsis in the fork
	printSubSection(w, "coreutils", s.Coreutils, verbose, syn)
	printSubSection(w, "classic", s.Classic, verbose, syn)

	fmt.Fprintln(w)
	fmt.Fprintf(w, "external — provisioned + exec'd downloaded binaries (%d):\n", len(s.External))
	printSubSection(w, "", s.External, verbose, syn)

	fmt.Fprintln(w)
	fmt.Fprintln(w, "bashy agent/ext — native features, by venue:")
	for _, v := range venueOrder {
		printSubSection(w, v, s.Agent[v], verbose, syn)
	}
}

// printSubSection prints one labeled sub-block, skipping empty ones. A non-empty
// label prints "  label (N):" then an indented body; an empty label prints the
// body directly under the parent header.
func printSubSection(w io.Writer, label string, names []string, verbose bool, syn func(string) string) {
	if len(names) == 0 {
		return
	}
	indent := "    "
	if label != "" {
		fmt.Fprintf(w, "  %s (%d):\n", label, len(names))
	} else {
		indent = "  "
	}
	if verbose && syn != nil {
		width := 0
		for _, n := range names {
			if len(n) > width {
				width = len(n)
			}
		}
		for _, n := range names {
			if d := syn(n); d != "" {
				fmt.Fprintf(w, "%s%-*s  %s\n", indent, width, n, d)
			} else {
				fmt.Fprintf(w, "%s%s\n", indent, n)
			}
		}
		return
	}
	wrapNames(w, names, indent, 80)
}

// wrapNames prints names space-separated, wrapped to width, each line prefixed
// with indent.
func wrapNames(w io.Writer, names []string, indent string, width int) {
	line := indent
	for _, n := range names {
		if len(line)+len(n)+1 > width && line != indent {
			fmt.Fprintln(w, strings.TrimRight(line, " "))
			line = indent
		}
		line += n + " "
	}
	if strings.TrimSpace(line) != "" {
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}
}
