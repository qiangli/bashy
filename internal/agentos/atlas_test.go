// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"slices"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/atlas"
)

func TestWebViewListsTheActualAppsByPublicName(t *testing.T) {
	var out bytes.Buffer
	printAtlasWeb(&out, liveAtlas(false))
	text := out.String()

	// These are the six apps the launcher actually exposes. Terminal and Files
	// are launcher-owned, so a view built only from atlas declarations silently
	// omitted them.
	for _, want := range []string{
		"Files", "Inbox", "Meet", "Messages", "Sprint", "Terminal",
	} {
		if !strings.Contains(text, "\n"+want+" ") {
			t.Errorf("web view is missing app %q:\n%s", want, text)
		}
	}

	// Apps is the launcher, not one of its own apps. Public app names and their
	// owning commands must agree.
	if strings.Contains(text, "\nApps ") {
		t.Errorf("web view lists the launcher as its own app:\n%s", text)
	}
	if !strings.Contains(text, "\nSprint") || !strings.Contains(text, "sprint") {
		t.Errorf("Sprint app does not identify its sprint command:\n%s", text)
	}
	for label, command := range map[string]string{"Meet": "meet", "Sprint": "sprint"} {
		found := false
		for _, line := range strings.Split(text, "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == label {
				found = true
				if fields[1] != command {
					t.Errorf("%s COMMAND = %q, want %q:\n%s", label, fields[1], command, text)
				}
			}
		}
		if !found {
			t.Errorf("web view has no %s row:\n%s", label, text)
		}
	}
}

// TestAtlasCoversEveryCommand is the bashy-side coverage ratchet: every name
// the live catalog reports — builtins, tools, always/agent-mode verbs,
// registry CLIs, hidden aliases — must resolve to a group and tier from the
// closed vocabularies. A new verb without an atlas entry fails here by name.
func TestAtlasCoversEveryCommand(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1") // include the agent-mode provisioners

	groups := map[string]bool{}
	for _, g := range atlas.Groups() {
		groups[g] = true
	}
	tiers := map[string]bool{}
	for _, tr := range atlas.Tiers() {
		tiers[tr] = true
	}
	caps := map[string]bool{}
	for _, c := range atlas.Capabilities() {
		caps[c] = true
	}

	records := liveAtlas(true)
	if len(records) == 0 {
		t.Fatal("empty atlas catalog")
	}
	stages := map[string]bool{}
	for _, s := range atlas.Stages() {
		stages[s] = true
	}

	byName := map[string]atlasRecord{}
	for _, r := range records {
		byName[r.Name] = r
		// An empty classification means the name resolved in NEITHER the atlas
		// nor the registry — i.e. a front-door command was shipped without an
		// atlas entry. This used to be undetectable: verbAtlasRecord filled in
		// GroupPlatform/TierUserland, which are *valid* values, so the checks
		// below passed and the omission was invisible. `fanout` lived here.
		if r.Group == "" || r.Tier == "" {
			t.Errorf("%s: NO ATLAS ENTRY — add it to coreutils/pkg/atlas "+
				"(and answer: which SDLC stage does it serve that nothing else does?)", r.Name)
			continue
		}
		if !groups[r.Group] {
			t.Errorf("%s: group %q not in vocabulary", r.Name, r.Group)
		}
		if !tiers[r.Tier] {
			t.Errorf("%s: tier %q not in vocabulary", r.Name, r.Tier)
		}
		if !stages[r.Stage] {
			t.Errorf("%s: sdlc stage %q not in vocabulary %v", r.Name, r.Stage, atlas.Stages())
		}
		for _, c := range r.Caps {
			if !caps[c] {
				t.Errorf("%s: cap %q not in vocabulary", r.Name, c)
			}
		}
		switch r.Class {
		case "builtin", "coreutils", "verb":
		default:
			t.Errorf("%s: unexpected class %q", r.Name, r.Class)
		}
	}
	for _, retired := range []string{"board", "relay"} {
		if _, ok := byName[retired]; ok {
			t.Errorf("retired top-level command %q remains in the live catalog", retired)
		}
	}

	// Every catalog source must be present (unique-by-name, dispatch
	// precedence: builtin wins echo/false/pwd/true; tool wins foreman).
	builtins, core, verbs := commandsCatalog()
	for _, set := range [][]string{builtins, core, verbs, hiddenVerbsCatalog()} {
		for _, n := range set {
			if _, ok := byName[n]; !ok {
				t.Errorf("catalog name %q missing from atlas records", n)
			}
		}
	}

	// Spot invariants.
	if r := byName["echo"]; r.Class != "builtin" || r.Group != atlas.GroupShell {
		t.Errorf("echo = %+v, want builtin/shell (builtin shadows the tool)", r)
	}
	if r := byName["grep"]; r.Class != "coreutils" || r.Group != atlas.GroupTextutils ||
		r.Tier != atlas.TierUserland || r.Resolver != "bashy-in-process" {
		t.Errorf("grep = %+v, want coreutils/textutils/userland", r)
	}
	if r := byName["weave"]; r.Tier != atlas.TierWorkspace {
		t.Errorf("weave tier = %q, want workspace", r.Tier)
	}
	if r := byName["docker"]; r.AliasOf != "oci" || r.Tier != atlas.TierSandbox {
		t.Errorf("docker = %+v, want alias_of oci, tier sandbox", r)
	}
	// `sandbox` is the tier-3 name, so the vocabulary and the verb surface agree.
	// Pinned alongside docker because both are the same alias and a half-applied
	// alias is the regression that once produced "docker: No such file or
	// directory" for a verb `bashy commands` was advertising.
	if r := byName["sandbox"]; r.AliasOf != "oci" || r.Tier != atlas.TierSandbox {
		t.Errorf("sandbox = %+v, want alias_of oci, tier sandbox", r)
	}
	if r := byName["dks"]; r.Stage != atlas.StageDeploy ||
		r.Group != atlas.GroupClusterCloud || r.Tier != atlas.TierCluster {
		t.Errorf("dks = %+v, want deploy/cluster-cloud/cluster", r)
	}
	if r := byName["doctl"]; r.Tier != atlas.TierCloud || r.Subclass != atlas.SubclassManagedExternal {
		t.Errorf("doctl = %+v, want registry-derived cloud/managed-external", r)
	}
	if r := byName["upgrade"]; !r.Hidden || r.AliasOf != "self" {
		t.Errorf("upgrade = %+v, want hidden alias of self", r)
	}
	if r := byName["chat"]; r.Hidden || r.AliasOf != "" {
		t.Errorf("chat = %+v, want visible canonical verb", r)
	}
	if r := byName["invoke"]; !r.Hidden || r.AliasOf != "chat" {
		t.Errorf("invoke = %+v, want hidden alias of chat", r)
	}
	if r := byName["messages"]; !r.Hidden || r.AliasOf != "mb" {
		t.Errorf("messages = %+v, want hidden alias of mb", r)
	} else if mb := byName["mb"]; !slices.Equal(r.Caps, mb.Caps) || !slices.Equal(r.Effects, mb.Effects) {
		t.Errorf("messages metadata = caps %v effects %v, want mb parity caps %v effects %v",
			r.Caps, r.Effects, mb.Caps, mb.Effects)
	}
	// Nouns are singular: the registry/catalog verbs are visible under the
	// singular and their plurals are hidden aliases with identical metadata.
	for plural, singular := range map[string]string{"agents": "agent", "models": "model", "tools": "tool",
		"people": "person", "skills": "skill", "secrets": "secret", "apps": "app", "issue": "todo", "resources": "resource"} {
		s, p := byName[singular], byName[plural]
		if s.Hidden || s.AliasOf != "" {
			t.Errorf("%s = %+v, want visible canonical verb", singular, s)
		}
		if !p.Hidden || p.AliasOf != singular {
			t.Errorf("%s = %+v, want hidden alias of %s", plural, p, singular)
		} else if !slices.Equal(p.Caps, s.Caps) || !slices.Equal(p.Effects, s.Effects) || p.Stage != s.Stage || p.Tier != s.Tier {
			t.Errorf("%s metadata = %+v, want parity with %s %+v", plural, p, singular, s)
		}
		if p.Web != nil {
			t.Errorf("%s declares a web surface; only the canonical %s may", plural, singular)
		}
	}
	if r := byName["ping"]; !slices.Contains(r.Caps, atlas.CapSpawnsProcesses) ||
		!slices.Contains(r.Effects, atlas.EffNet) || !slices.Contains(r.Effects, atlas.EffExec) ||
		!slices.Contains(r.Effects, atlas.EffPersist) {
		t.Errorf("ping = %+v, want hybrid board + system-ICMP metadata", r)
	}
	if r := byName["meet"]; !slices.Contains(r.Effects, atlas.EffRead) ||
		!slices.Contains(r.Effects, atlas.EffWrite) || !slices.Contains(r.Effects, atlas.EffPersist) ||
		!slices.Contains(r.Effects, atlas.EffNet) || !slices.Contains(r.Effects, atlas.EffExec) ||
		!slices.Contains(r.Effects, atlas.EffSpend) {
		t.Errorf("meet = %+v, want durable conversation + inference metadata", r)
	}
	if r := byName["go"]; r.Subclass != atlas.SubclassProvisioner {
		t.Errorf("go = %+v, want subclass provisioner", r)
	}
}

// Stability is a release promise rather than a visibility marker. Every verb
// must carry its declared yoke or bashy-owned tier, and a curated-hidden verb
// may never contradict the experimental marker that keeps it out of teaching.
func TestAtlasVerbStability(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1")
	for _, r := range liveAtlas(true) {
		if r.Class == "verb" && r.Stability == "" {
			t.Errorf("%s: no declared release stability", r.Name)
		}
		if isCuratedHidden(r.Name) && r.Stability != atlas.StabilityExperimental {
			t.Errorf("%s: curated-hidden stability = %q, want %q", r.Name, r.Stability, atlas.StabilityExperimental)
		}
	}
}

// Without includeHidden, hidden compatibility aliases must be absent. Toolchain
// provisioners remain present because the atlas describes the callable bashy
// front door, not only the bare-name Preamble shims.
func TestAtlasHidesHiddenAliasesByDefault(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	seenCargo := false
	for _, r := range liveAtlas(false) {
		if r.Name == "cargo" {
			seenCargo = true
		}
		if r.Hidden {
			t.Fatalf("hidden verb %q present without includeHidden", r.Name)
		}
	}
	if !seenCargo {
		t.Fatal("cargo provisioner missing from default atlas")
	}
}

// TestForemanAtlasEntryIsComplete is the Part 4a slice of the v1.0 release
// bar: foreman carries a named conductor consumer, so the bar requires a
// complete atlas entry (group, tier, sdlc, effects, os — the same predicate
// scripts/release-bar.py uses). The shared yoke atlas deliberately
// suppresses foreman (Bashy #40), so bashy classifies it outright in
// bashyOwnedVerbAtlas; the fallback platform/userland row without effects
// is not a complete entry.
func TestForemanAtlasEntryIsComplete(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1")
	var found *atlasRecord
	for _, r := range liveAtlas(true) {
		if r.Name == "foreman" {
			r := r
			found = &r
		}
	}
	if found == nil {
		t.Fatal("foreman missing from the live atlas catalog")
	}
	if found.Group == "" || found.Tier == "" || found.Stage == "" ||
		len(found.Effects) == 0 || len(found.OS) == 0 {
		t.Errorf("foreman has no complete atlas entry: group=%q tier=%q sdlc=%q effects=%v os=%v",
			found.Group, found.Tier, found.Stage, found.Effects, found.OS)
	}
}
