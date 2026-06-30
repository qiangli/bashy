// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestCommandsCatalogSources(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	builtins, core, verbs := commandsCatalog()

	// Builtins come from the sh interp (compgen -b set).
	if !slices.Contains(builtins, "cd") || !slices.Contains(builtins, "export") {
		t.Errorf("builtins missing core entries: %v", builtins)
	}
	// The coreutils userland — invisible to compgen, the reason this exists.
	for _, want := range []string{"cat", "ls", "grep", "tree", "yc"} {
		if !slices.Contains(core, want) {
			t.Errorf("coreutils userland missing %q", want)
		}
	}
	// Front-door verbs + the docker→podman shim + the lister itself.
	for _, want := range []string{"weave", "run", "commands", "docker"} {
		if !slices.Contains(verbs, want) {
			t.Errorf("verbs missing %q", want)
		}
	}
	// Each group is sorted.
	for _, g := range [][]string{builtins, core, verbs} {
		if !slices.IsSorted(g) {
			t.Errorf("group not sorted: %v", g)
		}
	}
}

func TestCommandsCatalogAgentModeAddsProvisioners(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	_, _, human := commandsCatalog()
	if slices.Contains(human, "go") {
		t.Error("go should not be shimmed (listed) outside agent mode")
	}
	t.Setenv("BASHY_AGENTIC", "1")
	_, _, agent := commandsCatalog()
	for _, want := range []string{"go", "cmake", "clang"} {
		if !slices.Contains(agent, want) {
			t.Errorf("agent mode should list provisioner %q", want)
		}
	}
}

func TestPrintCommandGroupFormat(t *testing.T) {
	var b bytes.Buffer
	printCommandGroup(&b, "demo", []string{"a", "b", "c"})
	out := b.String()
	if !strings.HasPrefix(out, "demo (3):\n") {
		t.Errorf("missing titled count header: %q", out)
	}
	if !strings.Contains(out, "  a b c") {
		t.Errorf("names should be indented and space-joined: %q", out)
	}
}

func TestVerbSynopsisCoversEveryVerb(t *testing.T) {
	// Every shimmed verb (incl. docker + the agent-mode provisioners) must have
	// a one-line synopsis, so `commands -v` never shows a bare verb.
	all := append([]string{"docker"}, alwaysShimVerbs...)
	all = append(all, agentModeShimVerbs...)
	for _, v := range all {
		if strings.TrimSpace(verbSynopsis[v]) == "" {
			t.Errorf("verb %q has no synopsis in verbSynopsis", v)
		}
	}
}

func TestPrintCommandSynopsesFormat(t *testing.T) {
	var b bytes.Buffer
	syn := func(n string) string {
		if n == "weave" {
			return "the orchestrator"
		}
		return ""
	}
	printCommandSynopses(&b, "verbs", []string{"weave", "x"}, syn)
	out := b.String()
	if !strings.Contains(out, "verbs (2):") {
		t.Errorf("missing header: %q", out)
	}
	if !strings.Contains(out, "weave") || !strings.Contains(out, "the orchestrator") {
		t.Errorf("synopsis line missing: %q", out)
	}
	// A name with no synopsis still prints (bare), not "name  ".
	if !strings.Contains(out, "\n  x\n") {
		t.Errorf("bare name (no synopsis) should still appear: %q", out)
	}
}

func TestPrintCommandGroupWraps(t *testing.T) {
	// Many long names must wrap to multiple indented lines, not one huge line.
	names := make([]string, 40)
	for i := range names {
		names[i] = "longcommandname"
	}
	var b bytes.Buffer
	printCommandGroup(&b, "wrap", names)
	lines := strings.Count(strings.TrimRight(b.String(), "\n"), "\n")
	if lines < 2 {
		t.Errorf("expected wrapped output across multiple lines, got %d newlines", lines)
	}
}
