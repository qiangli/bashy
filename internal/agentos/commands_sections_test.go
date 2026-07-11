// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"slices"
	"testing"

	"github.com/qiangli/coreutils/pkg/atlas"
)

// TestClassSectionsTaxonomy pins the by-how-it-runs grouping the default
// `bashy commands` surface renders: the builtins umbrella (shell / coreutils /
// classic), the exec'd externals, and the native agent features by venue.
func TestClassSectionsTaxonomy(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "1") // include agent-mode provisioners in External
	s := classSections(true)

	has := func(set []string, name string) bool { return slices.Contains(set, name) }

	// Builtins umbrella: shell builtins, then GNU coreutils vs classic tools.
	for _, b := range []string{"cd", "export", "echo"} { // echo: builtin shadows the tool
		if !has(s.Shell, b) {
			t.Errorf("shell builtins missing %q", b)
		}
	}
	for _, c := range []string{"cat", "cp", "ls", "wc", "sort", "mkdir"} {
		if !has(s.Coreutils, c) {
			t.Errorf("coreutils (GNU) missing %q", c)
		}
	}
	for _, c := range []string{"grep", "sed", "awk", "jq", "tree", "find", "xargs"} {
		if !has(s.Classic, c) {
			t.Errorf("classic tools missing %q", c)
		}
	}

	// External = downloaded + exec'd (managed externals + provisioners).
	for _, e := range []string{"gh", "kubectl", "helm", "loom"} {
		if !has(s.External, e) {
			t.Errorf("external missing %q", e)
		}
	}
	if !has(s.External, "go") { // toolchain provisioner (agent mode)
		t.Errorf("external missing provisioner %q", "go")
	}

	// Agent/ext by venue — including in-process agent tools (graph, code-intel,
	// foreman) that resolve as coreutils-class but belong with the verbs.
	agentAt := func(venue, name string) bool { return has(s.Agent[venue], name) }
	for _, name := range []string{"graph", "list-symbols", "foreman", "chat", "meet", "kb", "skills"} {
		if !agentAt(atlas.TierUserland, name) {
			t.Errorf("agent/userland missing %q", name)
		}
	}
	for _, name := range []string{"weave", "sprint", "dag"} {
		if !agentAt(atlas.TierWorkspace, name) {
			t.Errorf("agent/workspace missing %q", name)
		}
	}
	if !agentAt(atlas.TierSandbox, "podman") {
		t.Errorf("agent/sandbox missing %q", "podman")
	}
	if !agentAt(atlas.TierSphere, "sphere") {
		t.Errorf("agent/sphere missing %q", "sphere")
	}

	// A managed external must NOT leak into the agent section, and an agent
	// feature must NOT leak into external — the (d)/(e) line is load-bearing.
	for _, venue := range venueOrder {
		for _, n := range s.Agent[venue] {
			if has(s.External, n) {
				t.Errorf("%q appears in both agent/%s and external", n, venue)
			}
		}
	}

	// Every name lands in exactly one section (disjoint partition).
	seen := map[string]int{}
	for _, set := range [][]string{s.Shell, s.Coreutils, s.Classic, s.External} {
		for _, n := range set {
			seen[n]++
		}
	}
	for _, names := range s.Agent {
		for _, n := range names {
			seen[n]++
		}
	}
	for n, c := range seen {
		if c != 1 {
			t.Errorf("%q appears in %d sections, want 1", n, c)
		}
	}
}
