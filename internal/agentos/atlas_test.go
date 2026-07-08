// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"testing"

	"github.com/qiangli/coreutils/pkg/atlas"
)

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
	byName := map[string]atlasRecord{}
	for _, r := range records {
		byName[r.Name] = r
		if !groups[r.Group] {
			t.Errorf("%s: group %q not in vocabulary", r.Name, r.Group)
		}
		if !tiers[r.Tier] {
			t.Errorf("%s: tier %q not in vocabulary", r.Name, r.Tier)
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
	if r := byName["docker"]; r.AliasOf != "podman" || r.Tier != atlas.TierSandbox {
		t.Errorf("docker = %+v, want alias_of podman, tier sandbox", r)
	}
	if r := byName["doctl"]; r.Tier != atlas.TierCloud || r.Subclass != atlas.SubclassManagedExternal {
		t.Errorf("doctl = %+v, want registry-derived cloud/managed-external", r)
	}
	if r := byName["upgrade"]; !r.Hidden || r.AliasOf != "self" {
		t.Errorf("upgrade = %+v, want hidden alias of self", r)
	}
	if r := byName["go"]; r.Subclass != atlas.SubclassProvisioner {
		t.Errorf("go = %+v, want subclass provisioner", r)
	}
}

// Without agent mode the provisioners are not shimmed and must be absent —
// the atlas mirrors the Preamble, not a superset of it.
func TestAtlasRespectsAgentMode(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	for _, r := range liveAtlas(false) {
		if r.Name == "cargo" {
			t.Fatalf("agent-mode provisioner %q present without $BASHY_AGENTIC", r.Name)
		}
		if r.Hidden {
			t.Fatalf("hidden verb %q present without includeHidden", r.Name)
		}
	}
}
