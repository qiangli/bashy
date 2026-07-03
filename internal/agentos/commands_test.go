// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
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
	// (code-intel is exposed as flat first-class verbs since the yc flatten.)
	for _, want := range []string{"cat", "ls", "grep", "tree", "list-symbols"} {
		if !slices.Contains(core, want) {
			t.Errorf("coreutils userland missing %q", want)
		}
	}
	// Front-door verbs + the docker→podman shim + the lister itself.
	for _, want := range []string{"weave", "run", "commands", "docker", "self"} {
		if !slices.Contains(verbs, want) {
			t.Errorf("verbs missing %q", want)
		}
	}
	for _, hidden := range hiddenFrontDoorVerbs {
		if slices.Contains(verbs, hidden) {
			t.Errorf("hidden verb %q should not appear in the default catalog", hidden)
		}
	}
	// Each group is sorted.
	for _, g := range [][]string{builtins, core, verbs} {
		if !slices.IsSorted(g) {
			t.Errorf("group not sorted: %v", g)
		}
	}
}

func TestHiddenVerbsCatalog(t *testing.T) {
	hidden := hiddenVerbsCatalog()
	for _, want := range []string{"bootstrap", "upgrade"} {
		if !slices.Contains(hidden, want) {
			t.Errorf("hidden catalog missing %q", want)
		}
	}
	if !slices.IsSorted(hidden) {
		t.Errorf("hidden verbs not sorted: %v", hidden)
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
	all = append(all, hiddenFrontDoorVerbs...)
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

func TestAgenticCommandsMentionsDryRun(t *testing.T) {
	var b bytes.Buffer
	printAgenticCommands(&b)
	out := b.String()
	for _, want := range []string{
		"bashy help dryrun",
		"bashy fetch --json URL",
		"bashy self fetch",
		"BASHY_AGENTIC=1 bashy --dry-run",
		"destroy",
		"truncate",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("agentic commands help missing %q:\n%s", want, out)
		}
	}
}

func TestGNUCoreutilsReportTracksGaps(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	builtins, core, _ := commandsCatalog()
	report := gnuCoreutilsReport(core, builtins)

	if report.Summary.UpstreamCommands == 0 {
		t.Fatal("expected GNU upstream inventory")
	}
	if !slices.Contains(report.BashyNative, "ls") {
		t.Fatalf("expected implemented GNU command ls in native set: %#v", report.BashyNative)
	}
	if !slices.Contains(report.Missing, "timeout") {
		t.Fatalf("expected missing GNU command timeout in missing set: %#v", report.Missing)
	}
	if !slices.Contains(report.CoveredByBuiltins, "printf") || !slices.Contains(report.CoveredByBuiltins, "test") {
		t.Fatalf("expected printf/test to be tracked as bash builtin coverage: %#v", report.CoveredByBuiltins)
	}
	if !gnuGapHas(report.Not100Conformant, "ls") {
		t.Fatalf("expected implemented but uncertified GNU command in not_100_conformant: %#v", report.Not100Conformant)
	}
	if !slices.Contains(report.NonGNUExtras, "grep") || !slices.Contains(report.NonGNUExtras, "list-symbols") {
		t.Fatalf("expected non-GNU bashy extras: %#v", report.NonGNUExtras)
	}
}

func TestCommandFeatureReportGrepKnownGap(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	builtins, core, verbs := commandsCatalog()
	info := commandFeatureReport("grep", builtins, core, verbs, hiddenVerbsCatalog(), gnuCoreutilsReport(core, builtins))
	if info["class"] != "coreutils" || info["available"] != true {
		t.Fatalf("unexpected grep feature report: %#v", info)
	}
	if _, ok := info["known_gaps"]; !ok {
		t.Fatalf("grep report should include known gaps: %#v", info)
	}
}

func TestCommandFeatureReportMissingGNUCoreutil(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	builtins, core, verbs := commandsCatalog()
	info := commandFeatureReport("timeout", builtins, core, verbs, hiddenVerbsCatalog(), gnuCoreutilsReport(core, builtins))
	if info["class"] != "gnu-coreutils-missing" && info["class"] != "coreutils" {
		t.Fatalf("unexpected timeout feature report: %#v", info)
	}
}

func TestCommandsJSONIncludesGNUReport(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	var b bytes.Buffer
	builtins, core, verbs := commandsCatalog()
	report := gnuCoreutilsReport(core, builtins)
	out := map[string]any{
		"schema_version": commandsSchemaVersion,
		"builtins":       builtins,
		"coreutils":      core,
		"verbs":          verbs,
		"gnu_coreutils":  report,
	}
	if err := json.NewEncoder(&b).Encode(out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"not_100_conformant"`) || !strings.Contains(b.String(), `"missing"`) {
		t.Fatalf("json report missing GNU gap fields: %s", b.String())
	}
}

func gnuGapHas(items []gnuCoreutilsGap, name string) bool {
	for _, item := range items {
		if item.Name == name {
			return true
		}
	}
	return false
}

func TestUsageMentionsAgenticDryRun(t *testing.T) {
	out := Usage()
	for _, want := range []string{"--dryrun", "--dry-run", "BASHY_AGENTIC=1", "bashy help dryrun", "bashy commands --gnu", "GNU coreutils parity", "bashy self fetch"} {
		if !strings.Contains(out, want) {
			t.Fatalf("usage missing %q:\n%s", want, out)
		}
	}
}
