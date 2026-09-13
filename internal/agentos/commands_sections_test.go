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

	// Agent/ext by venue — including in-process agent tools (graph,
	// code-intel) that resolve as coreutils-class but belong with the verbs.
	// foreman was a member here; it is now a suppressed internal (Bashy #40),
	// so it no longer resolves as a coreutils-class agent tool.
	agentAt := func(venue, name string) bool { return has(s.Agent[venue], name) }
	for _, name := range []string{"graph", "ast", "chat", "meet", "kb", "whois"} {
		if !agentAt(atlas.TierUserland, name) {
			t.Errorf("agent/userland missing %q", name)
		}
	}
	// POSIX applets in the net/code-intel groups are userland, not agent
	// features — the origin axis files them right (the old group shortcut
	// did not).
	for _, name := range []string{"mail", "mailx", "talk"} {
		if !has(s.Classic, name) {
			t.Errorf("classic (POSIX applet) missing %q", name)
		}
	}
	for _, name := range []string{"lp", "ctags"} {
		if !has(s.External, name) {
			t.Errorf("external (pinned provider) missing %q", name)
		}
	}
	for _, name := range []string{"weave", "sprint", "dag"} {
		if !agentAt(atlas.TierWorkspace, name) {
			t.Errorf("agent/workspace missing %q", name)
		}
	}
	// The taught names are listed; the engines behind them are experimental.
	if !agentAt(atlas.TierSandbox, "sandbox") {
		t.Errorf("agent/sandbox missing %q", "sandbox")
	}
	if !agentAt(atlas.TierSphere, "peer") {
		t.Errorf("agent/sphere missing %q", "peer")
	}
	for _, name := range []string{"podman", "docker", "sphere", "supervise", "run", "tokens", "posix-gate"} {
		if !has(s.Experimental, name) {
			t.Errorf("experimental missing %q", name)
		}
	}
	for _, name := range []string{"skills", "invoke", "issue"} {
		if !has(s.Aliases, name) {
			t.Errorf("aliases missing %q", name)
		}
	}
	// The 1.0.0 core: every core row name is a real, visible, bashy-added
	// command, and core is a subset of agent/* (never of the userland).
	coreN := 0
	for _, row := range s.Core {
		for _, n := range row.Commands {
			coreN++
			found := false
			for _, venue := range venueOrder {
				if agentAt(venue, n) {
					found = true
				}
			}
			if !found && !has(s.Diagnostics, n) {
				t.Errorf("core %q is not a visible bashy-added command", n)
			}
			if has(s.More, n) {
				t.Errorf("core %q also listed under more", n)
			}
		}
	}
	if coreN != 35 {
		t.Errorf("core has %d commands, want 35 (Sprint 167 decision of record)", coreN)
	}
	for _, name := range []string{"inspect", "sandbox", "ollama", "peer", "dks", "login", "tessaro",
		"release", "transpile", "dhnt", "otel", "duration", "tz", "ntp", "sntp", "clip", "ast"} {
		if !has(s.More, name) {
			t.Errorf("more (keep-visible) missing %q", name)
		}
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
