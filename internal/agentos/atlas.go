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
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/qiangli/coreutils/external/registry"
	"github.com/qiangli/coreutils/pkg/atlas"
	"github.com/qiangli/coreutils/pkg/webconsole"
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
	Stage    string   `json:"sdlc"` // SDLC stage: plan|code|test|deploy|cross
	Resolver string   `json:"resolver"`
	Caps     []string `json:"caps,omitempty"`
	Effects  []string `json:"effects,omitempty"`
	Synopsis string   `json:"synopsis,omitempty"`

	// Web is the browser surface this command declares, if any. It is what
	// `bashy web-console` discovers instead of carrying a hardcoded tile list,
	// and what `commands --view web` renders. omitempty keeps every record
	// without one byte-identical to the v1 shape.
	Web     *atlas.WebSurface `json:"web_ui,omitempty"`
	Hidden  bool              `json:"hidden,omitempty"`
	AliasOf string            `json:"alias_of,omitempty"`

	// Origin is the provenance axis (Sprint 167): bash | gnu | unix | external
	// | bashy — WHO defined the command, as opposed to Class, which says how
	// it resolves. Posix tags the 116 POSIX-required names across origins.
	// Core marks the bashy 1.0.0 core; Status is "experimental" on a
	// curated-hidden command, absent on a hidden alias, so a reader can tell
	// "hidden because unproven" from "hidden because it is a second spelling".
	Origin string `json:"origin,omitempty"`
	Posix  bool   `json:"posix,omitempty"`
	Core   bool   `json:"core,omitempty"`
	Status string `json:"status,omitempty"`

	// Platform support (Sprint 167): OS = where the command is supported,
	// Partial = supported OSes with a documented gap, Portable = full on all
	// three. Default listings are filtered to runtime.GOOS (a listing that
	// names mkfifo on Windows advertises a command that will fail); --os any
	// lifts the filter, --portable keeps only the cross-platform set.
	OS       []string `json:"os,omitempty"`
	Partial  []string `json:"partial,omitempty"`
	Portable bool     `json:"portable,omitempty"`
}

// statusExperimental is the Status of a curated-hidden command that is
// hidden because it is unproven; statusAlias is the Status of a curated-hidden
// name that is hidden only because it is a second SPELLING of a visible
// canonical command (podman/docker → oci, sphere → peer). The label matches
// the reason, so a reader is never told that a vendor spelling is "not yet
// proven" when its engine is on the first screen.
const (
	statusExperimental = "experimental"
	statusAlias        = "alias"
)

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
			Tier: atlas.TierUserland, Stage: atlas.StageCross,
			Resolver: "bash-builtin",
			// The shell owns its builtins; the atlas tables never see them, so
			// the origin is stamped here. `[`, `printf`, `kill` … are also GNU
			// coreutils programs, but the builtin shadows the tool: the shell
			// is who answers, so the shell is the origin.
			Origin: atlas.OriginBash, Posix: atlas.IsPosixRequired(n),
			OS: atlas.OSes(), Portable: true,
		})
	}
	for _, n := range core {
		add(toolAtlasRecord(n, false))
	}
	for _, n := range verbs {
		add(verbAtlasRecord(n, false))
	}
	for _, n := range hidden {
		// `--all` adds two kinds of hidden name: compatibility aliases (verbs)
		// and curated experimental commands, which may be in-process tools.
		if tool.Lookup(n) != nil {
			add(toolAtlasRecord(n, true))
			continue
		}
		add(verbAtlasRecord(n, true))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// bashyOwnedVerbAtlas classifies front-door verbs implemented here rather than
// in coreutils, so they carry a real classification instead of falling into the
// deliberately-empty unknown branch below.
var bashyOwnedVerbAtlas = map[string]atlas.Entry{
	"resource": {
		Stage: atlas.StageCross, Group: atlas.GroupDiagnostics, Tier: atlas.TierUserland,
		Caps: []string{atlas.CapJSON, atlas.CapReadOnly}, Effects: []string{atlas.EffRead},
	},
	"resources": {
		Stage: atlas.StageCross, Group: atlas.GroupDiagnostics, Tier: atlas.TierUserland,
		Caps: []string{atlas.CapJSON, atlas.CapReadOnly}, Effects: []string{atlas.EffRead}, AliasOf: "resource",
	},
	"out": {
		Stage: atlas.StageCross, Group: atlas.GroupDiagnostics, Tier: atlas.TierUserland,
		Caps: []string{atlas.CapReadOnly}, Effects: []string{atlas.EffRead},
	},
	"transpile": {
		Stage: atlas.StageCode, Group: atlas.GroupToolchains, Tier: atlas.TierUserland,
		Effects: []string{atlas.EffRead, atlas.EffWrite},
	},
	"full": {
		Stage: atlas.StageCross, Group: atlas.GroupShellutils, Tier: atlas.TierUserland,
		Caps: []string{atlas.CapSpawnsProcesses}, Effects: []string{atlas.EffExec},
	},
	// dhnt reads pipeline/run/binding JSON and writes JSON or Workflow YAML to
	// stdout. No network, no mutation — the local-first contract/compiler, not
	// a transport.
	"dhnt": {
		Stage: atlas.StageTest,
		Group: atlas.GroupPlatform,
		Tier:  atlas.TierUserland,
		Caps:  []string{atlas.CapJSON, atlas.CapReadOnly},
	},
	// release turns a .goreleaser.yaml into named, checksummed artifacts. It
	// serves DEPLOY: no other verb owns what bytes leave this machine or under
	// what name. Tier is workspace — it reads and writes one project tree and
	// nothing outside it. Group is toolchains: the T0 stages are the build and
	// packaging side of the surface, next to the go/cmake/clang provisioners it
	// drives (the closed group vocabulary, which lives in coreutils, has no
	// "build" member and this slice does not change it).
	//
	// Effects are exactly what the T0 slice does: read the tree, write dist/,
	// exec the Go toolchain. NOT `net` — `release --snapshot` is local-first,
	// and publishing (which would earn `net`) is not implemented here.
	"release": {
		Stage:   atlas.StageDeploy,
		Group:   atlas.GroupToolchains,
		Tier:    atlas.TierWorkspace,
		Caps:    []string{atlas.CapJSON, atlas.CapSpawnsProcesses},
		Effects: []string{atlas.EffExec, atlas.EffRead, atlas.EffWrite},
	},
	"inbox": {
		Stage:   atlas.StageCross,
		Group:   atlas.GroupOrch,
		Tier:    atlas.TierUserland,
		Caps:    []string{atlas.CapJSON},
		Effects: []string{atlas.EffRead, atlas.EffWrite},
	},
	"notify": {
		Stage:   atlas.StageCross,
		Group:   atlas.GroupOrch,
		Tier:    atlas.TierUserland,
		Caps:    []string{atlas.CapJSON},
		Effects: []string{atlas.EffWrite},
	},
	// activity is the control surface over the shared activity-event contract.
	// Effects are read+write and NOT `net`: the whole delivery path is the
	// local bus and the local session control socket, which is what keeps the
	// SDLC loop inside the local-first guarantee pkg/atlas ratchets.
	"activity": {
		Stage:   atlas.StageCross,
		Group:   atlas.GroupOrch,
		Tier:    atlas.TierUserland,
		Caps:    []string{atlas.CapJSON},
		Effects: []string{atlas.EffRead, atlas.EffWrite},
	},
}

// toolAtlasRecord resolves one in-process (coreutils-class) tool.
func toolAtlasRecord(name string, hidden bool) atlasRecord {
	r := atlasRecord{Name: name, Class: "coreutils", Resolver: "bashy-in-process", Hidden: hidden}
	fillFromAtlas(&r)
	if t := tool.Lookup(name); t != nil {
		r.Synopsis = t.Synopsis
	}
	stampSurface(&r)
	return r
}

// stampSurface sets the 1.0.0-surface fields a record carries on top of its
// atlas classification: Core for the named core, Status for a curated
// experimental command. Aliases (AliasOf set) get neither: an alias is a
// spelling, and its target already says what it is.
func stampSurface(r *atlasRecord) {
	if r.AliasOf == "" && isCoreCommand(r.Name) {
		r.Core = true
	}
	if isCuratedHidden(r.Name) {
		r.Status = statusExperimental
		if r.AliasOf != "" && !isCuratedHidden(r.AliasOf) {
			r.Status = statusAlias
		}
	}
}

// verbAtlasRecord resolves one front-door verb: the curated table first,
// then the declarative registry (whose entries derive group/tier/caps from
// Entry.Tier — new registry CLIs need no atlas edit).
func verbAtlasRecord(name string, hidden bool) (r atlasRecord) {
	r = atlasRecord{
		Name: name, Class: "verb", Resolver: "bashy-front-door",
		Hidden: hidden, Synopsis: verbSynopsis[name],
	}
	defer stampSurface(&r) // named result: the stamp lands on what is returned
	// `resources` is also the historical GNU-shaped in-shell applet. At the
	// bashy front door it is now the hidden plural alias of `resource`, so the
	// front-door classification must win over the applet's shared atlas row.
	if name == "resources" {
		applyEntry(&r, bashyOwnedVerbAtlas[name])
		r.Origin = atlas.OriginBashy
		r.OS, r.Portable = atlas.OSes(), true
		return r
	}
	if e, ok := atlas.Lookup(name); ok {
		applyEntry(&r, e)
		return r
	}
	if e, ok := registry.Lookup(name); ok {
		applyEntry(&r, atlas.RegistryEntry(e.Tier))
		// Registry CLIs are outside the atlas tables; their platform support
		// is declared by name in the same table as every other external.
		if d, ok := atlas.ExternalPlatforms(name); ok {
			r.OS, r.Portable = d.OS, len(d.OS) == len(atlas.OSes())
		}
		return r
	}
	// Verbs bashy owns outright, which the shared coreutils atlas has no
	// reason to know about. Consulted LAST so it can never shadow a curated
	// entry: if coreutils later classifies one of these, that wins and this
	// row becomes dead weight rather than a silent override.
	if e, ok := bashyOwnedVerbAtlas[name]; ok {
		applyEntry(&r, e)
		if r.Origin == "" {
			r.Origin = atlas.OriginBashy // bashy-owned by definition
		}
		if len(r.OS) == 0 {
			r.OS, r.Portable = atlas.OSes(), true
		}
		return r
	}
	// Unknown to both tables. Keep it VISIBLE, but do not invent a
	// classification for it.
	//
	// This branch used to assign GroupPlatform/TierUserland with a comment
	// claiming "the coverage test fails on this state so it cannot persist
	// silently". It did not: those are *valid* vocabulary values, and the
	// coverage test only checked that group/tier were in-vocabulary — so an
	// unclassified verb sailed through wearing a fabricated classification.
	// `fanout` shipped that way and nobody could see it. The fallback defeated
	// the very test it invoked as its justification.
	//
	// Leaving these empty makes the state observable, and the bashy-side
	// coverage ratchet now fails on it by name.
	r.Group, r.Tier, r.Stage = "", "", ""
	return r
}

func fillFromAtlas(r *atlasRecord) {
	if e, ok := atlas.Lookup(r.Name); ok {
		applyEntry(r, e)
		return
	}
	// Shell builtins are deliberately absent from the atlas — the embedding
	// shell owns that set (see the atlas package doc) — so this fallback is
	// legitimate here, unlike the verb path. A builtin serves every stage.
	// A registered tool the shared atlas does not list (foreman, registered by
	// bashy) is bashy's own.
	r.Group, r.Tier, r.Stage = atlas.GroupPlatform, atlas.TierUserland, atlas.StageCross
	r.Origin, r.Posix = atlas.OriginBashy, atlas.IsPosixRequired(r.Name)
	r.OS, r.Portable = atlas.OSes(), true
}

func applyEntry(r *atlasRecord, e atlas.Entry) {
	r.Group, r.Tier, r.Subclass, r.Caps, r.AliasOf = e.Group, e.Tier, e.Subclass, e.Caps, e.AliasOf
	r.Stage = e.Stage
	r.Effects = e.Effects
	r.Web = e.Web
	r.Origin, r.Posix = e.Origin, e.Posix
	r.OS, r.Partial, r.Portable = e.OS, e.Partial, e.Portable()
}

// liveAtlas assembles the full merged catalog for the bashy front door,
// optionally with the hidden compatibility aliases. Toolchain provisioners are
// always listed here because `bashy go`, `bashy clang`, etc. are callable even
// when the Preamble leaves bare `go`/`clang` to the user's PATH outside agent
// mode.
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
var atlasViews = []string{"tier", "group", "sdlc", "capabilities", "effects", "web", "origin", "posix", "external", "portable"}

// atlasGroupDisplayOrder is the presentation order for the group view:
// classical userland first, then the extended groups.
var atlasGroupDisplayOrder = []string{
	atlas.GroupShell, atlas.GroupFileutils, atlas.GroupTextutils,
	atlas.GroupShellutils, atlas.GroupCodeIntel, atlas.GroupNet,
	atlas.GroupOrch, atlas.GroupKnowledge, atlas.GroupEngines,
	atlas.GroupForge, atlas.GroupToolchains, atlas.GroupStorage,
	atlas.GroupClusterCloud, atlas.GroupPlatform, atlas.GroupDiagnostics,
	atlas.GroupAccount,
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
	view     string // "", "tier", "group", "sdlc", "capabilities", "effects", "web", "origin"
	tier     string // filters (ANDed when several are given)
	group    string
	cap      string
	effect   string
	idioms   bool
	full     bool // --atlas: full records
	asJSON   bool
	all      bool // include hidden compatibility aliases
	verbose  bool
	os       string // platform filter: a GOOS, or "any"; "" = this host (runtime.GOOS)
	portable bool   // only commands with full support on every platform
}

type atlasJSON struct {
	SchemaVersion   string            `json:"schema_version"`
	View            string            `json:"view,omitempty"`
	Filter          map[string]string `json:"filter,omitempty"`
	Tiers           []string          `json:"tiers,omitempty"`
	Groups          []string          `json:"groups,omitempty"`
	Capabilities    []string          `json:"capabilities,omitempty"`
	SecurityEffects []string          `json:"security_effects,omitempty"`
	Origins         []string          `json:"origins,omitempty"`
	Commands        []atlasRecord     `json:"commands,omitempty"`
	Idioms          []atlas.Idiom     `json:"idioms,omitempty"`
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
	if req.effect != "" && !containsString(atlas.Effects(), req.effect) {
		fmt.Fprintf(os.Stderr, "commands: unknown effect %q (effects: %s)\n",
			req.effect, strings.Join(atlas.Effects(), " "))
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
	if req.effect != "" {
		filter["effect"] = req.effect
	}
	query := len(filter) > 0
	if query {
		records = filterAtlas(records, req.tier, req.group, req.cap, req.effect)
	}
	// Platform: default = this host. `--all` means everything, so it lifts the
	// platform filter too unless --os was given explicitly.
	osFilter := req.os
	if osFilter == "" {
		osFilter = runtime.GOOS
		if req.all {
			osFilter = "any"
		}
	}
	if osFilter != "any" {
		records = filterOS(records, osFilter)
		filter["os"] = osFilter
	}
	if req.portable || req.view == "portable" {
		records = filterPortable(records)
		filter["portable"] = "true"
	}
	if req.view == "external" {
		// Likewise a filter: only what bashy exec's rather than links.
		records = filterOrigin(records, atlas.OriginExternal)
		filter["origin"] = atlas.OriginExternal
	}
	if req.view == "posix" {
		// The posix view IS a filter: only the POSIX-required names, in text
		// and in JSON alike, so `--view posix --json` is the machine answer to
		// "which of the 116 does this bashy provide, and how".
		records = filterPosix(records)
		filter["posix"] = "true"
	}

	if req.asJSON {
		out := atlasJSON{
			SchemaVersion:   atlasSchemaVersion,
			View:            req.view,
			Tiers:           atlas.Tiers(),
			Groups:          atlas.Groups(),
			Capabilities:    atlas.Capabilities(),
			SecurityEffects: atlas.Effects(),
			Origins:         atlas.Origins(),
			Commands:        records,
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
	case req.view == "posix":
		printAtlasPosix(os.Stdout, records)
	case req.view == "external":
		printAtlasExternal(os.Stdout, records)
	case req.view == "portable":
		printAtlasPortable(os.Stdout, records, liveAtlas(req.all))
	case query:
		// tier/group/cap/effect are QUERIES and render as a flat list; the
		// platform filters (os, portable) narrow the records but leave the
		// requested view's shape alone, so `--view origin --portable` is still
		// the origin view.
		printAtlasFiltered(os.Stdout, records, filter)
	case req.view == "group":
		printAtlasByKey(os.Stdout, records, atlasGroupDisplayOrder, "", func(r atlasRecord) string { return r.Group })
	case req.view == "capabilities":
		printAtlasCaps(os.Stdout, records)
	case req.view == "effects":
		printAtlasEffects(os.Stdout, records)
	case req.view == "web":
		printAtlasWeb(os.Stdout, records)
	case req.view == "origin":
		printAtlasOrigin(os.Stdout, records)
	case req.view == "sdlc":
		// The spine: plan → code → test → deploy (+ cross). Reading this view is
		// how you SEE the shape of the surface — which is how the Code stage was
		// found to carry six overlapping verbs while the Test stage carried none.
		printAtlasByKey(os.Stdout, records, atlas.Stages(), "sdlc ", func(r atlasRecord) string { return r.Stage })
	case req.full:
		printAtlasRecords(os.Stdout, records)
	default: // "tier"
		printAtlasByKey(os.Stdout, records, atlas.Tiers(), "tier ", func(r atlasRecord) string { return r.Tier })
	}
	return 0
}

func filterAtlas(records []atlasRecord, tier, group, capability, effect string) []atlasRecord {
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
		if effect != "" && !containsString(r.Effects, effect) {
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

// printAtlasEffects buckets commands by security effect, in the closed-vocab
// order, so an operator can see at a glance which commands can destroy data,
// touch credentials, reach another host, and so on.
func printAtlasEffects(w io.Writer, records []atlasRecord) {
	byEff := map[string][]string{}
	for _, r := range records {
		for _, e := range r.Effects {
			byEff[e] = append(byEff[e], r.Name)
		}
	}
	for _, e := range atlas.Effects() {
		if len(byEff[e]) == 0 {
			continue
		}
		printCommandGroup(w, e, byEff[e])
	}
}

// printAtlasOrigin is the provenance view: one block per origin, in the
// closed order (shell → GNU → classic Unix → exec'd externals → added by
// bashy), each name marked `*` when it is one of the 116 POSIX-required
// utilities and `~` when it is a curated experimental command. It answers the
// question the class split cannot: not "how does this resolve" but "who
// defined it" — the question a reader asks before trusting a name in a script
// that must also run under stock bash.
func printAtlasOrigin(w io.Writer, records []atlasRecord) {
	byOrigin := map[string][]string{}
	posix, experimental := 0, 0
	for _, r := range records {
		name := r.Name
		if r.Posix {
			name += "*"
			posix++
		}
		if r.Status == statusExperimental {
			name += "~"
			experimental++
		}
		byOrigin[r.Origin] = append(byOrigin[r.Origin], name)
	}
	fmt.Fprintf(w, "origin — who defined each command (%d; * = POSIX-required, %d", len(records), posix)
	if experimental > 0 {
		fmt.Fprintf(w, "; ~ = experimental, %d", experimental)
	}
	fmt.Fprintln(w, "):")
	for _, o := range atlas.Origins() {
		names := byOrigin[o]
		if len(names) == 0 {
			continue
		}
		// The block name is the origin value, then its label; for the yoke
		// origin the label already carries the value ("yoke — added by bashy"),
		// so print the label alone rather than "bashy — yoke — added by bashy".
		title := o + " — " + atlas.OriginLabel(o)
		if o == atlas.OriginBashy {
			title = atlas.OriginLabel(o)
		}
		fmt.Fprintf(w, "  %s (%d):\n", title, len(names))
		wrapNames(w, names, "    ", 80)
	}
	if n := len(byOrigin[""]); n > 0 {
		fmt.Fprintf(w, "  (unclassified: %d — a bug; every record must carry an origin)\n", n)
	}
}

func filterOS(records []atlasRecord, goos string) []atlasRecord {
	var out []atlasRecord
	for _, r := range records {
		if slices.Contains(r.OS, goos) {
			out = append(out, r)
		}
	}
	return out
}

func filterPortable(records []atlasRecord) []atlasRecord {
	var out []atlasRecord
	for _, r := range records {
		if r.Portable {
			out = append(out, r)
		}
	}
	return out
}

// printAtlasPortable is the cross-platform view: the commands a script can
// use AS-IS on windows, macOS and linux (full support, no documented gap),
// by origin — and, from the unfiltered catalog, the ones that are not, each
// with its reason, so "why isn't X here" is answered on the same screen.
func printAtlasPortable(w io.Writer, portable, all []atlasRecord) {
	byOrigin := map[string][]string{}
	for _, r := range portable {
		byOrigin[r.Origin] = append(byOrigin[r.Origin], r.Name)
	}
	fmt.Fprintf(w, "portable — runs as-is on windows, macOS and linux (%d of %d):\n", len(portable), len(all))
	for _, o := range atlas.Origins() {
		names := byOrigin[o]
		if len(names) == 0 {
			continue
		}
		title := o + " — " + atlas.OriginLabel(o)
		if o == atlas.OriginBashy {
			title = atlas.OriginLabel(o)
		}
		fmt.Fprintf(w, "  %s (%d):\n", title, len(names))
		wrapNames(w, names, "    ", 80)
	}
	var missing, partial []string
	for _, r := range all {
		switch {
		case r.Portable:
		case len(r.OS) < len(atlas.OSes()):
			missing = append(missing, r.Name+" ("+strings.Join(r.OS, ",")+")")
		default:
			partial = append(partial, r.Name+" (partial on "+strings.Join(r.Partial, ",")+")")
		}
	}
	if len(missing) > 0 {
		fmt.Fprintf(w, "  not on every platform (%d) — where it IS supported:\n", len(missing))
		wrapNames(w, missing, "    ", 80)
	}
	if len(partial) > 0 {
		fmt.Fprintf(w, "  everywhere, with a documented gap (%d) — `bashy commands NAME` names it:\n", len(partial))
		wrapNames(w, partial, "    ", 80)
	}
}

func filterOrigin(records []atlasRecord, origin string) []atlasRecord {
	var out []atlasRecord
	for _, r := range records {
		if r.Origin == origin {
			out = append(out, r)
		}
	}
	return out
}

// printAtlasExternal is the bin-managed view: everything bashy downloads and
// exec's instead of linking, by kind — the pinned POSIX providers (the
// pure-Go DEBT: each is a POSIX-required utility not yet implemented in Go),
// the managed externals (wrapped tools with their own release train), and
// the toolchain provisioners. `*` marks POSIX-required. The view exists so
// "what is still not pure Go" is one command, not a grep.
func printAtlasExternal(w io.Writer, records []atlasRecord) {
	kinds := []struct{ key, title string }{
		{"provider", "pinned POSIX providers — built locally from pinned upstream source; the pure-Go debt"},
		{"managed-external", "managed externals — wrapped tools with their own release train (binmgr)"},
		{"provisioner", "toolchain provisioners — version-pinned language/build toolchains"},
	}
	byKind := map[string][]string{}
	posixDebt := 0
	for _, r := range records {
		name := r.Name
		if r.Posix {
			name += "*"
		}
		if r.Status == statusExperimental {
			name += "~"
		}
		kind := r.Subclass
		if kind == "" {
			kind = atlas.SubclassManagedExternal // registry-derived CLIs carry it; defensive
		}
		// The provider block is EXACTLY the POSIX-required names served by a
		// pinned provider — the debt list, nothing else. `why` (the witr
		// wrapper) and `posix-providers` (the provisioner in front of the ten)
		// are managed externals like any other wrapped tool.
		if r.Class == "coreutils" && r.Posix && kind == atlas.SubclassManagedExternal {
			kind = "provider"
			posixDebt++
		} else if r.Class == "coreutils" {
			kind = atlas.SubclassManagedExternal
		}
		byKind[kind] = append(byKind[kind], name)
	}
	fmt.Fprintf(w, "external — bin-managed: downloaded + exec'd, never linked (%d; * = POSIX-required):\n", len(records))
	for _, k := range kinds {
		names := byKind[k.key]
		if len(names) == 0 {
			continue
		}
		fmt.Fprintf(w, "  %s (%d):\n", k.title, len(names))
		wrapNames(w, names, "    ", 80)
	}
	fmt.Fprintf(w, "  pure-Go debt: %d POSIX-required utilities still come from a pinned provider —\n", posixDebt)
	fmt.Fprintln(w, "    `bashy posix-providers` provisions and inspects them; each one implemented in Go leaves this list.")
}

func filterPosix(records []atlasRecord) []atlasRecord {
	var out []atlasRecord
	for _, r := range records {
		if r.Posix {
			out = append(out, r)
		}
	}
	return out
}

// printAtlasPosix is the certification view: the 116 POSIX-required utility
// names (docs/posix-required-commands.tsv, what `posix-gate` certifies),
// grouped by WHO PROVIDES each one in this bashy — the shell, the GNU
// reimplementation, the classic-Unix reimplementation, or a pinned external
// provider — with any name the listing cannot show named explicitly rather
// than silently missing. A name marked `~` is provided by a curated
// experimental command.
func printAtlasPosix(w io.Writer, records []atlasRecord) {
	byOrigin := map[string][]string{}
	seen := map[string]bool{}
	internal, managed := 0, 0
	for _, r := range records {
		name := r.Name
		if r.Status == statusExperimental {
			name += "~"
		}
		byOrigin[r.Origin] = append(byOrigin[r.Origin], name)
		seen[r.Name] = true
		if r.Origin == atlas.OriginExternal {
			managed++
		} else {
			internal++
		}
	}
	required := atlas.PosixRequired()
	var missing, notHere []string
	for _, n := range required {
		if seen[n] {
			continue
		}
		if e, ok := atlas.Lookup(n); ok && len(e.OS) > 0 {
			// Catalogued, but filtered out by the platform filter.
			notHere = append(notHere, n+" ("+strings.Join(e.OS, ",")+")")
			continue
		}
		missing = append(missing, n)
	}
	fmt.Fprintf(w, "posix — the %d POSIX-required utilities, by who provides each one here (%d listed):\n",
		len(required), len(records))
	// Two top-level groups, labeled so the split is unmistakable to an agent
	// and a human alike: what runs INSIDE the bashy binary (pure Go, zero
	// fork) versus what is BIN-MANAGED (a locally built pinned upstream that
	// bashy exec's). The second group is the pure-Go debt.
	fmt.Fprintf(w, "  internal — in the bashy binary, pure Go, no fork (%d):\n", internal)
	for _, o := range []string{atlas.OriginBash, atlas.OriginGNU, atlas.OriginUnix} {
		names := byOrigin[o]
		if len(names) == 0 {
			continue
		}
		fmt.Fprintf(w, "    %s — %s (%d):\n", o, atlas.OriginLabel(o), len(names))
		wrapNames(w, names, "      ", 80)
	}
	fmt.Fprintf(w, "  bin-managed — exec'd from a locally built pinned upstream; the pure-Go debt (%d):\n", managed)
	if names := byOrigin[atlas.OriginExternal]; len(names) > 0 {
		wrapNames(w, names, "      ", 80)
	}
	if len(notHere) > 0 {
		fmt.Fprintf(w, "  not on this platform (%d) — where it IS supported: %s (`--os any` lists all)\n", len(notHere), strings.Join(notHere, " "))
	}
	if len(missing) > 0 {
		fmt.Fprintf(w, "  not listed (%d): %s\n", len(missing), strings.Join(missing, " "))
		fmt.Fprintln(w, "    `sh` is the Preamble's `sh() { bashy --posix; }` shim, not a catalogued command;")
		fmt.Fprintln(w, "    anything else here is a gap — `bashy posix-gate spec` is the certified projection.")
	}
}

func printAtlasFiltered(w io.Writer, records []atlasRecord, filter map[string]string) {
	var parts []string
	for _, k := range []string{"tier", "group", "cap", "effect", "os", "portable"} {
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
	fmt.Fprintf(w, "command atlas (%d commands):\n", len(records))
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
		if len(r.Effects) > 0 {
			line += " {" + strings.Join(r.Effects, ",") + "}"
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
		r = atlasRecord{Group: atlas.GroupShell, Tier: atlas.TierUserland,
			Origin: atlas.OriginBash, Posix: atlas.IsPosixRequired(name)}
	case "coreutils":
		r = toolAtlasRecord(name, hidden)
	case "verb":
		r = verbAtlasRecord(name, hidden)
	default:
		return
	}
	out["group"], out["tier"] = r.Group, r.Tier
	if len(r.Caps) > 0 {
		out["caps"] = r.Caps
	}
	if len(r.Effects) > 0 {
		out["effects"] = r.Effects
	}
	if r.Subclass != "" {
		out["subclass"] = r.Subclass
	}
	if r.AliasOf != "" {
		out["alias_of"] = r.AliasOf
	}
	if r.Origin != "" {
		out["origin"] = r.Origin
	}
	if r.Posix {
		out["posix"] = true
	}
	if r.Core {
		out["core"] = true
	}
	if r.Status != "" {
		out["status"] = r.Status
	}
	if use := taughtNameFor(name); use != "" {
		out["use"] = use
	}
	if len(r.OS) > 0 {
		out["os"] = r.OS
	}
	if len(r.Partial) > 0 {
		out["partial"] = r.Partial
	}
	if r.Portable {
		out["portable"] = true
	}
	if !slices.Contains(r.OS, runtime.GOOS) && len(r.OS) > 0 {
		out["unsupported_here"] = runtime.GOOS
	}
}

// taughtNameFor returns the visible spelling of a curated-hidden command that
// hides behind a taught alias, or "" when there is none.
func taughtNameFor(name string) string {
	switch name {
	case "podman", "docker":
		return "oci"
	case "sphere":
		return "peer"
	}
	return ""
}

func sortedCopy(items []string) []string {
	out := append([]string(nil), items...)
	sort.Strings(out)
	return out
}

// printAtlasWeb renders the browser surfaces: which commands serve a web UI,
// where the console mounts them, and the command that starts the ones it does
// not own.
//
// This is deliberately a VIEW rather than a verb. `bashy commands` already
// merges the atlas and already has --view, and the atlas's own ratchet asks any
// new verb which SDLC stage it serves that nothing else already does — "list the
// web surfaces" has no answer to that. `bashy web-console` renders the same data
// as tiles; one source, two renderers.
func printAtlasWeb(w io.Writer, records []atlasRecord) {
	// Render the launcher's discovered panels, not only atlas declarations.
	// Terminal and Files belong to the launcher itself and deliberately have no
	// pretend `bashy terminal` / `bashy files` verbs. Conversely, Apps is the
	// launcher rather than one of the apps it launches, so Discover omits it.
	panels := webconsole.Discover()
	if len(panels) == 0 {
		fmt.Fprintln(w, "no web apps are available")
		return
	}

	// A panel's public name is its label (Sprint), while COMMAND is the canonical
	// Bashy verb that owns it (sprint). Keep both fields because labels are for
	// people and commands are stable machine tokens.
	commandByMount := make(map[string]string, len(records))
	for _, r := range records {
		if r.Web != nil && r.Web.Mode != atlas.WebSelf {
			commandByMount[r.Web.Mount] = r.Name
		}
	}
	sort.Slice(panels, func(i, j int) bool { return panels[i].Label < panels[j].Label })

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "APP\tCOMMAND\tMODE\tPORT\tPATH\tSTART")
	for _, p := range panels {
		mount := strings.Trim(p.Path, "/")
		command := commandByMount[mount]
		if command == "" {
			command = "-"
		}
		port := ""
		if p.Port != 0 {
			port = strconv.Itoa(p.Port)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			p.Label, command, p.Mode, port, p.Path, p.StartHint())
	}
	_ = tw.Flush()
	fmt.Fprintln(w, "\nOpen them together with:  bashy app")
}
