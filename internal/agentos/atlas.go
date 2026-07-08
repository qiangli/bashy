// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// The bashy side of the Command Atlas (docs/command-atlas.md): merges what
// only the embedding shell knows — the builtin name set, shim visibility,
// the declarative registry — with the curated metadata tables in
// coreutils/pkg/atlas into one per-command record set for the
// `bashy commands` atlas views.
package agentos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/qiangli/coreutils/external/registry"
	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/tool"
)

const atlasSchemaVersion = "bashy-atlas-v1"

// atlasRecord is one merged Command Atlas record.
type atlasRecord struct {
	Name     string   `json:"name"`
	Class    string   `json:"class"`
	Subclass string   `json:"subclass,omitempty"`
	Group    string   `json:"group"`
	Tier     string   `json:"tier"`
	Resolver string   `json:"resolver"`
	Caps     []string `json:"caps,omitempty"`
	Synopsis string   `json:"synopsis,omitempty"`
	Hidden   bool     `json:"hidden,omitempty"`
	AliasOf  string   `json:"alias_of,omitempty"`
}

// atlasCatalog builds the merged atlas records for the given live catalog
// (the outputs of commandsCatalog + hiddenVerbsCatalog). Names are unique;
// when a name exists in several sources the shell's resolution order wins
// (builtin > coreutils tool > front-door verb), mirroring dispatch.
func atlasCatalog(builtins, core, verbs, hidden []string) []atlasRecord {
	seen := map[string]bool{}
	var out []atlasRecord
	add := func(r atlasRecord) {
		if seen[r.Name] {
			return
		}
		seen[r.Name] = true
		out = append(out, r)
	}
	for _, n := range builtins {
		add(atlasRecord{
			Name: n, Class: "builtin", Group: atlas.GroupShell,
			Tier: atlas.TierUserland, Resolver: "bash-builtin",
		})
	}
	for _, n := range core {
		r := atlasRecord{Name: n, Class: "coreutils", Resolver: "bashy-in-process"}
		fillFromAtlas(&r)
		if t := tool.Lookup(n); t != nil {
			r.Synopsis = t.Synopsis
		}
		add(r)
	}
	for _, n := range verbs {
		add(verbAtlasRecord(n, false))
	}
	for _, n := range hidden {
		add(verbAtlasRecord(n, true))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// verbAtlasRecord resolves one front-door verb: the curated table first,
// then the declarative registry (whose entries derive group/tier/caps from
// Entry.Tier — new registry CLIs need no atlas edit).
func verbAtlasRecord(name string, hidden bool) atlasRecord {
	r := atlasRecord{
		Name: name, Class: "verb", Resolver: "bashy-front-door",
		Hidden: hidden, Synopsis: verbSynopsis[name],
	}
	if e, ok := atlas.Lookup(name); ok {
		applyEntry(&r, e)
		return r
	}
	if e, ok := registry.Lookup(name); ok {
		applyEntry(&r, atlas.RegistryEntry(e.Tier))
		return r
	}
	// Unknown to both tables: keep it visible rather than dropping it —
	// the coverage test fails on this state so it cannot persist silently.
	r.Group, r.Tier = atlas.GroupPlatform, atlas.TierUserland
	return r
}

func fillFromAtlas(r *atlasRecord) {
	if e, ok := atlas.Lookup(r.Name); ok {
		applyEntry(r, e)
		return
	}
	r.Group, r.Tier = atlas.GroupPlatform, atlas.TierUserland
}

func applyEntry(r *atlasRecord, e atlas.Entry) {
	r.Group, r.Tier, r.Subclass, r.Caps, r.AliasOf = e.Group, e.Tier, e.Subclass, e.Caps, e.AliasOf
}

// liveAtlas assembles the full merged catalog for the current mode
// (agent-mode provisioners included only under $BASHY_AGENTIC, mirroring the
// Preamble), optionally with the hidden compatibility aliases.
func liveAtlas(includeHidden bool) []atlasRecord {
	builtins, core, verbs := commandsCatalog()
	var hidden []string
	if includeHidden {
		hidden = hiddenVerbsCatalog()
	}
	return atlasCatalog(builtins, core, verbs, hidden)
}

// --- the views ---------------------------------------------------------------

// atlasViews are the non-classic --view values.
var atlasViews = []string{"tier", "group", "capabilities"}

// atlasGroupDisplayOrder is the presentation order for the group view:
// classical userland first, then the extended groups.
var atlasGroupDisplayOrder = []string{
	atlas.GroupShell, atlas.GroupFileutils, atlas.GroupTextutils,
	atlas.GroupShellutils, atlas.GroupCodeIntel, atlas.GroupNet,
	atlas.GroupOrch, atlas.GroupKnowledge, atlas.GroupEngines,
	atlas.GroupForge, atlas.GroupToolchains, atlas.GroupStorage,
	atlas.GroupClusterCloud, atlas.GroupPlatform, atlas.GroupAccount,
}

// tierSynopsis mirrors the locked one-liners in dhnt docs/execution-tiers.md.
var tierSynopsis = map[string]string{
	atlas.TierUserland:  "single-node, native",
	atlas.TierWorkspace: "single-node, fs-isolated",
	atlas.TierSandbox:   "single-node, OS-isolated (OCI)",
	atlas.TierSphere:    "multi-node, peer-direct",
	atlas.TierCluster:   "your own many machines, orchestrated",
	atlas.TierCloud:     "multi-provider, hosted",
	atlas.TierAccount:   "the Tessaro front door (pairs a machine for tiers 4-5)",
}

type atlasRequest struct {
	view    string // "", "tier", "group", "capabilities"
	tier    string // filters (ANDed when several are given)
	group   string
	cap     string
	idioms  bool
	full    bool // --atlas: full records
	asJSON  bool
	all     bool // include hidden compatibility aliases
	verbose bool
}

type atlasJSON struct {
	SchemaVersion string            `json:"schema_version"`
	View          string            `json:"view,omitempty"`
	Filter        map[string]string `json:"filter,omitempty"`
	Tiers         []string          `json:"tiers,omitempty"`
	Groups        []string          `json:"groups,omitempty"`
	Capabilities  []string          `json:"capabilities,omitempty"`
	Commands      []atlasRecord     `json:"commands,omitempty"`
	Idioms        []atlas.Idiom     `json:"idioms,omitempty"`
}

// dispatchAtlas renders the Command Atlas views. Unknown vocabulary values
// exit 2 and print the closed vocabulary so an agent self-corrects in one
// round trip.
func dispatchAtlas(req atlasRequest) int {
	if req.view != "" && !containsString(sortedCopy(atlasViews), req.view) {
		fmt.Fprintf(os.Stderr, "commands: unknown view %q (views: classic %s)\n",
			req.view, strings.Join(atlasViews, " "))
		return 2
	}
	if req.tier != "" && !containsString(sortedCopy(atlas.Tiers()), req.tier) {
		fmt.Fprintf(os.Stderr, "commands: unknown tier %q (tiers: %s)\n",
			req.tier, strings.Join(atlas.Tiers(), " "))
		return 2
	}
	if req.group != "" && !containsString(atlas.Groups(), req.group) {
		fmt.Fprintf(os.Stderr, "commands: unknown group %q (groups: %s)\n",
			req.group, strings.Join(atlas.Groups(), " "))
		return 2
	}
	if req.cap != "" && !containsString(atlas.Capabilities(), req.cap) {
		fmt.Fprintf(os.Stderr, "commands: unknown capability %q (capabilities: %s)\n",
			req.cap, strings.Join(atlas.Capabilities(), " "))
		return 2
	}

	if req.idioms {
		if req.asJSON {
			b, _ := json.Marshal(atlasJSON{SchemaVersion: atlasSchemaVersion, Idioms: atlas.Idioms()})
			fmt.Println(string(b))
			return 0
		}
		printIdioms(os.Stdout, atlas.Idioms())
		return 0
	}

	records := liveAtlas(req.all)
	filter := map[string]string{}
	if req.tier != "" {
		filter["tier"] = req.tier
	}
	if req.group != "" {
		filter["group"] = req.group
	}
	if req.cap != "" {
		filter["cap"] = req.cap
	}
	if len(filter) > 0 {
		records = filterAtlas(records, req.tier, req.group, req.cap)
	}

	if req.asJSON {
		out := atlasJSON{
			SchemaVersion: atlasSchemaVersion,
			View:          req.view,
			Tiers:         atlas.Tiers(),
			Groups:        atlas.Groups(),
			Capabilities:  atlas.Capabilities(),
			Commands:      records,
		}
		if len(filter) > 0 {
			out.Filter = filter
		}
		if req.full {
			out.Idioms = atlas.Idioms()
		}
		b, _ := json.Marshal(out)
		fmt.Println(string(b))
		return 0
	}

	switch {
	case len(filter) > 0:
		printAtlasFiltered(os.Stdout, records, filter)
	case req.view == "group":
		printAtlasByKey(os.Stdout, records, atlasGroupDisplayOrder, "", func(r atlasRecord) string { return r.Group })
	case req.view == "capabilities":
		printAtlasCaps(os.Stdout, records)
	case req.full:
		printAtlasRecords(os.Stdout, records)
	default: // "tier"
		printAtlasByKey(os.Stdout, records, atlas.Tiers(), "tier ", func(r atlasRecord) string { return r.Tier })
	}
	return 0
}

func filterAtlas(records []atlasRecord, tier, group, capability string) []atlasRecord {
	var out []atlasRecord
	for _, r := range records {
		if tier != "" && r.Tier != tier {
			continue
		}
		if group != "" && r.Group != group {
			continue
		}
		if capability != "" && !containsString(r.Caps, capability) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// printAtlasByKey renders records bucketed by a key (tier or group) in the
// given order, reusing the classic wrapped-column block.
func printAtlasByKey(w io.Writer, records []atlasRecord, order []string, prefix string, key func(atlasRecord) string) {
	byKey := map[string][]string{}
	for _, r := range records {
		k := key(r)
		byKey[k] = append(byKey[k], r.Name)
	}
	for _, k := range order {
		names := byKey[k]
		if len(names) == 0 {
			continue
		}
		title := prefix + k
		if s := tierSynopsis[k]; prefix != "" && s != "" {
			title += " — " + s
		}
		printCommandGroup(w, title, names)
	}
}

func printAtlasCaps(w io.Writer, records []atlasRecord) {
	byCap := map[string][]string{}
	for _, r := range records {
		for _, c := range r.Caps {
			byCap[c] = append(byCap[c], r.Name)
		}
	}
	for _, c := range atlas.Capabilities() {
		if len(byCap[c]) == 0 {
			continue
		}
		printCommandGroup(w, c, byCap[c])
	}
}

func printAtlasFiltered(w io.Writer, records []atlasRecord, filter map[string]string) {
	var parts []string
	for _, k := range []string{"tier", "group", "cap"} {
		if v := filter[k]; v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	names := make([]string, 0, len(records))
	for _, r := range records {
		names = append(names, r.Name)
	}
	printCommandSynopses(w, "atlas — "+strings.Join(parts, " "), names, func(n string) string {
		for _, r := range records {
			if r.Name == n {
				return r.Synopsis
			}
		}
		return ""
	})
}

func printAtlasRecords(w io.Writer, records []atlasRecord) {
	fmt.Fprintf(w, "command atlas (%d commands; docs/command-atlas.md):\n", len(records))
	width := 0
	for _, r := range records {
		if len(r.Name) > width {
			width = len(r.Name)
		}
	}
	for _, r := range records {
		line := fmt.Sprintf("  %-*s  %s/%s", width, r.Name, r.Tier, r.Group)
		if len(r.Caps) > 0 {
			line += " [" + strings.Join(r.Caps, ",") + "]"
		}
		if r.AliasOf != "" {
			line += " → " + r.AliasOf
		}
		fmt.Fprintln(w, line)
	}
}

func printIdioms(w io.Writer, idioms []atlas.Idiom) {
	fmt.Fprintf(w, "idioms — commands naturally used together (%d):\n", len(idioms))
	for _, id := range idioms {
		fmt.Fprintf(w, "  %s (%s): %s\n", id.ID, strings.Join(id.Commands, " "), id.Pattern)
		if id.Fused != "" {
			fmt.Fprintf(w, "      fused: %s\n", id.Fused)
		}
		fmt.Fprintf(w, "      %s\n", id.Note)
	}
}

// atlasFeatureFields adds the additive atlas keys to a --features report.
func atlasFeatureFields(out map[string]any, name string, class string, hidden bool) {
	var r atlasRecord
	switch class {
	case "builtin":
		r = atlasRecord{Group: atlas.GroupShell, Tier: atlas.TierUserland}
	case "coreutils":
		r = atlasRecord{Name: name}
		fillFromAtlas(&r)
	case "verb":
		r = verbAtlasRecord(name, hidden)
	default:
		return
	}
	out["group"], out["tier"] = r.Group, r.Tier
	if len(r.Caps) > 0 {
		out["caps"] = r.Caps
	}
	if r.Subclass != "" {
		out["subclass"] = r.Subclass
	}
	if r.AliasOf != "" {
		out["alias_of"] = r.AliasOf
	}
}

func sortedCopy(items []string) []string {
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}
