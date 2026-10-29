// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"

	"github.com/qiangli/yoke/pkg/atlas"
	"sort"
)

// The `bashy commands` default surface (Sprint 167, bashy 1.0.0) is
// featured-first: the first screen is the 35 commands an agent uses every turn,
// then the handful of visible extras, then one line per userland origin with
// a pointer to the view that expands it, then the count of what is hidden.
// 325 names in a wall was the problem; a first screen an agent can read in
// one pass is the fix.
//
//	featured   — the featured commands, in seven rows (fleet · session · work ·
//	             knowledge · comms · human · discovery). A PRESENTATION
//	             grouping — the atlas `group` axis is untouched.
//	more       — visible yoke commands (bashy-added) that are not featured
//	classic    — everything that is not yoke: bash builtins · GNU coreutils ·
//	             classic Unix · bin-managed externals, as counts (`--view
//	             origin` lists them)
//	hidden     — curated experimental commands (`--all` lists them)
//
// The partition below is keyed on the atlas ORIGIN axis — who defined the
// command — not on group heuristics. The previous split filed every
// in-process tool in a `net`/`code-intel` group as an agent feature, which
// put the POSIX `mail`, `mailx`, `talk`, `lp` and `ctags` next to `weave`.
// Origin is stamped per entry in coreutils/pkg/atlas and ratcheted there.

// venueOrder is the locked six-venue stack (+ account front door) used to
// order the agent/ext section (dhnt docs/execution-tiers.md).
var venueOrder = []string{
	atlas.TierUserland, atlas.TierWorkplace, atlas.TierSandbox,
	atlas.TierSphere, atlas.TierCluster, atlas.TierCloud, atlas.TierAccount,
}

// featuredRow is one presentation row of the featured commands.
type featuredRow struct {
	Label    string   `json:"label"`
	Commands []string `json:"commands"`
}

// featuredRows is the bashy featured list: the operator's named list (agent model
// tool skill chat · sprint todo kb · inbox mb meet ping · dag weave · app ask
// browser) plus the commands those are built on. The order within a row is
// deliberate (the noun first, then what acts on it); it is not sorted.
var featuredRows = []featuredRow{
	{"fleet", []string{"agent", "model", "tool", "skill", "person", "whois", "capability"}},
	{"session", []string{"chat", "delegate", "foreman", "coach", "handoff", "resume", "claim"}},
	{"work", []string{"sprint", "todo", "dag", "weave", "gate"}},
	{"knowledge", []string{"kb", "graph", "craft", "secret"}},
	{"comms", []string{"inbox", "mb", "meet", "ping", "notify", "bus", "activity"}},
	{"human", []string{"app", "ask", "browser", "fetch"}},
	{"discovery", []string{"commands"}},
}

var featuredSet = func() map[string]bool {
	m := map[string]bool{}
	for _, row := range featuredRows {
		for _, n := range row.Commands {
			m[n] = true
		}
	}
	return m
}()

// isFeaturedCommand reports whether name is in the bashy featured list.
func isFeaturedCommand(name string) bool { return featuredSet[name] }

// commandSections is the `bashy commands` surface, partitioned. The
// by-how-it-runs fields (shell / coreutils / classic / external / agent) are
// the v1 shape and stay; featured / more / experimental / aliases are the 1.0.0
// first-screen layout on top of them.
type commandSections struct {
	Featured     []featuredRow       `json:"featured"`               // the featured commands, in presentation rows
	More         []string            `json:"more"`                   // visible bashy-added, not featured
	Experimental []string            `json:"experimental,omitempty"` // curated-hidden (only with --all)
	Aliases      []string            `json:"aliases,omitempty"`      // hidden compatibility aliases (only with --all)
	Shell        []string            `json:"shell"`                  // bash builtins
	Coreutils    []string            `json:"coreutils"`              // GNU coreutils, in-process
	Classic      []string            `json:"classic"`                // other classic Unix tools, in-process
	External     []string            `json:"external"`               // downloaded + exec'd
	Registered   []string            `json:"registered,omitempty"`   // the operator's ring (`commands add`)
	Shadowed     map[string]string   `json:"shadowed,omitempty"`     // ring entries this bashy now ships a command for → holder
	Diagnostics  []string            `json:"diagnostics"`            // check/diagnose/verify family
	Agent        map[string][]string `json:"agent"`                  // venue -> bashy-added commands
}

// classSections partitions the live catalog. It reuses liveAtlas so the
// grouping stays in lockstep with the atlas the --view/--json paths report;
// every decision reads a field of the record (Origin, Subclass, Group, Tier,
// Featured, Status, AliasOf), never a name list of its own — except featuredRows,
// which is the one list this file owns.
func classSections(all bool) commandSections {
	goos := runtime.GOOS
	if all {
		goos = "any"
	}
	return classSectionsOn(all, goos)
}

// classSectionsOn is classSections filtered to one platform ("any" = none).
func classSectionsOn(all bool, goos string) commandSections {
	s := commandSections{Agent: map[string][]string{}}
	for _, row := range featuredRows {
		s.Featured = append(s.Featured, featuredRow{row.Label, append([]string(nil), row.Commands...)})
	}
	s.Shadowed = registeredShadowed()
	for _, r := range liveAtlas(all) {
		if goos != "any" && !slices.Contains(r.OS, goos) {
			continue
		}
		// The operator's ring is its own block, decided BEFORE the hidden
		// check: a hidden registered command is still a registered command,
		// not a compatibility alias, and the default branch below would file
		// an unknown origin under "more yoke".
		if r.Origin == atlas.OriginRegistered {
			if !r.Hidden || all {
				s.Registered = append(s.Registered, r.Name)
			}
			continue
		}
		switch {
		case r.Status == statusExperimental:
			s.Experimental = append(s.Experimental, r.Name)
			continue
		case r.Hidden: // compatibility aliases and hidden spellings (status alias)
			s.Aliases = append(s.Aliases, r.Name)
			continue
		}
		switch r.Origin {
		case atlas.OriginBash:
			s.Shell = append(s.Shell, r.Name)
		case atlas.OriginGNU:
			s.Coreutils = append(s.Coreutils, r.Name)
		case atlas.OriginUnix:
			s.Classic = append(s.Classic, r.Name)
		case atlas.OriginExternal:
			s.External = append(s.External, r.Name)
		default: // bashy — the agent/ext partition by venue, plus featured/more on top
			if !r.Featured {
				s.More = append(s.More, r.Name)
			}
			if r.DispatchClass == "verb" && r.Group == atlas.GroupDiagnostics {
				s.Diagnostics = append(s.Diagnostics, r.Name)
			} else {
				s.Agent[r.Tier] = append(s.Agent[r.Tier], r.Name)
			}
		}
	}
	return s
}

// printClassSections renders the first screen. In verbose mode each featured and
// visible-extra command gets its one-line synopsis; otherwise names are
// wrapped into compact columns. `all` appends the hidden sets.
func printClassSections(w io.Writer, verbose, all bool, goos string) {
	s := classSectionsOn(all, goos)
	syn := synopsisOf
	featuredN := 0
	for _, row := range s.Featured {
		featuredN += len(row.Commands)
	}
	userland := len(s.Shell) + len(s.Coreutils) + len(s.Classic) + len(s.External)
	// The hidden count is the EXPERIMENTAL count; hidden spellings of visible
	// commands (podman, docker, sphere) are aliases, and are counted there.
	hiddenN, aliasN := 0, 0
	for _, r := range liveAtlas(true) {
		switch {
		case r.Status == statusExperimental:
			hiddenN++
		case r.Hidden:
			aliasN++
		}
	}
	total := featuredN + len(s.More) + userland + len(s.Registered) + hiddenN + aliasN

	fmt.Fprintf(w, "bashy commands — the 1.0.0 surface: %d featured + %d more yoke commands; `--all` for everything (%d)\n",
		featuredN, len(s.More), total)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "featured — what an agent uses every turn (%d):\n", featuredN)
	if verbose {
		for _, row := range s.Featured {
			printSubSection(w, row.Label, row.Commands, true, syn)
		}
	} else {
		width := 0
		for _, row := range s.Featured {
			if len(row.Label) > width {
				width = len(row.Label)
			}
		}
		for _, row := range s.Featured {
			fmt.Fprintf(w, "  %-*s  %s\n", width, row.Label, strings.Join(row.Commands, " "))
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintf(w, "more yoke commands — visible, not featured (%d):\n", len(s.More))
	printSubSection(w, "", s.More, verbose, syn)

	fmt.Fprintln(w)
	fmt.Fprintf(w, "classic — everything that is not yoke: bash builtins (%d) · GNU coreutils (%d) · classic Unix (%d) · bin-managed externals (%d):\n",
		len(s.Shell), len(s.Coreutils), len(s.Classic), len(s.External))
	fmt.Fprintln(w, "  bashy commands --view origin     every name by who defined it (* = POSIX-required)")
	fmt.Fprintln(w, "  bashy commands --view posix      the 116 POSIX-required utilities: internal (pure Go) vs bin-managed")
	fmt.Fprintln(w, "  bashy commands --view external   what is downloaded + exec'd, and the pure-Go debt")
	fmt.Fprintln(w, "  bashy commands --view tier       by execution venue")

	if goos != "any" {
		if n := unsupportedHere(goos); len(n) > 0 {
			fmt.Fprintf(w, "  not on %s (%d), not listed: %s — `bashy commands --os any` lists everything\n",
				goos, len(n), strings.Join(n, " "))
		}
	}
	fmt.Fprintln(w, "  bashy commands --view portable   what runs as-is on windows · macOS · linux (--portable filters any view)")

	if len(s.Registered) > 0 || len(s.Shadowed) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "registered — yours, added with `bashy commands add`; dispatched like every command above (%d):\n", len(s.Registered))
		printSubSection(w, "", s.Registered, verbose, syn)
		if len(s.Shadowed) > 0 {
			names := make([]string, 0, len(s.Shadowed))
			for n := range s.Shadowed {
				names = append(names, n)
			}
			sort.Strings(names)
			fmt.Fprintf(w, "  shadowed by this bashy, skipped (%d):", len(names))
			for _, n := range names {
				fmt.Fprintf(w, " %s (%s)", n, s.Shadowed[n])
			}
			fmt.Fprintln(w, " — rename with `bashy commands set NAME --set name=…`")
		}
	}

	fmt.Fprintln(w)
	if all {
		fmt.Fprintf(w, "experimental yoke commands — hidden by default: they work, they are not yet proven (%d):\n", len(s.Experimental))
		printSubSection(w, "", s.Experimental, verbose, syn)
		fmt.Fprintf(w, "hidden aliases (%d):\n", len(s.Aliases))
		printSubSection(w, "", s.Aliases, verbose, syn)
	} else {
		fmt.Fprintf(w, "%d experimental yoke commands are hidden — they work, they are not yet proven: bashy commands --all\n", hiddenN)
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
		// Continuation lines of a multi-line synopsis hang under the description
		// column, so a wrapped description stays aligned instead of falling back
		// to column 0.
		pad := strings.Repeat(" ", len(indent)+width+2)
		for _, n := range names {
			d := syn(n)
			if d == "" {
				fmt.Fprintf(w, "%s%s\n", indent, n)
				continue
			}
			if strings.Contains(d, "\n") {
				d = strings.ReplaceAll(d, "\n", "\n"+pad)
			}
			fmt.Fprintf(w, "%s%-*s  %s\n", indent, width, n, d)
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
