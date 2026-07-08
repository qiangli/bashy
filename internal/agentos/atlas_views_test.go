// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

// captureCommands runs dispatchCommands with stdout captured.
func captureCommands(t *testing.T, args ...string) (string, int) {
	t.Helper()
	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	code := dispatchCommands(args)
	w.Close()
	os.Stdout = prev
	var sb strings.Builder
	buf := make([]byte, 64<<10)
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String(), code
}

func TestAtlasViewTierJSON(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--view", "tier", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got atlasJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.SchemaVersion != atlasSchemaVersion {
		t.Errorf("schema_version = %q, want %q", got.SchemaVersion, atlasSchemaVersion)
	}
	if got.View != "tier" {
		t.Errorf("view = %q", got.View)
	}
	if len(got.Commands) == 0 {
		t.Fatal("no commands")
	}
	// The merged record set must cover the classic catalog exactly (unique
	// names, builtin-first precedence).
	builtins, core, verbs := commandsCatalog()
	unique := map[string]bool{}
	for _, set := range [][]string{builtins, core, verbs} {
		for _, n := range set {
			unique[n] = true
		}
	}
	if len(got.Commands) != len(unique) {
		t.Errorf("commands = %d records, want %d unique catalog names", len(got.Commands), len(unique))
	}
	if got.Idioms != nil {
		t.Errorf("tier view should not embed idioms")
	}
}

func TestAtlasFilterTier(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--tier", "workspace", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got atlasJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Filter["tier"] != "workspace" {
		t.Errorf("filter = %v", got.Filter)
	}
	var names []string
	for _, r := range got.Commands {
		if r.Tier != "workspace" {
			t.Errorf("%s: tier %q leaked through the workspace filter", r.Name, r.Tier)
		}
		names = append(names, r.Name)
	}
	for _, want := range []string{"weave", "sprint", "dag", "sdlc", "loom"} {
		if !slices.Contains(names, want) {
			t.Errorf("workspace tier missing %q (got %v)", want, names)
		}
	}
}

func TestAtlasFilterCap(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--cap", "json", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got atlasJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var names []string
	for _, r := range got.Commands {
		names = append(names, r.Name)
	}
	for _, want := range []string{"weave", "fetch", "tokens", "kb"} {
		if !slices.Contains(names, want) {
			t.Errorf("cap=json missing %q", want)
		}
	}
	if slices.Contains(names, "rm") {
		t.Errorf("rm has no json cap but passed the filter")
	}
}

func TestAtlasUnknownVocabularyExits2(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	for _, args := range [][]string{
		{"--tier", "bogus"},
		{"--group", "bogus"},
		{"--cap", "bogus"},
		{"--view", "bogus"},
	} {
		if _, code := captureCommands(t, args...); code != 2 {
			t.Errorf("%v: exit = %d, want 2", args, code)
		}
	}
}

func TestAtlasIdiomsJSON(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--idioms", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got atlasJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Idioms) < 10 {
		t.Errorf("idioms = %d, want >= 10", len(got.Idioms))
	}
	if got.Commands != nil {
		t.Errorf("--idioms should not embed the command records")
	}
}

func TestAtlasFullIncludesIdioms(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--atlas", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got atlasJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(got.Commands) == 0 || len(got.Idioms) == 0 {
		t.Errorf("--atlas: commands=%d idioms=%d, want both non-empty", len(got.Commands), len(got.Idioms))
	}
	if len(got.Tiers) == 0 || len(got.Groups) == 0 || len(got.Capabilities) == 0 {
		t.Errorf("--atlas: vocabularies missing")
	}
}

// The back-compat promise: the default JSON output keeps exactly the
// bashy-commands-v1 shape — atlas work must never leak keys into it.
func TestCommandsDefaultJSONUnchanged(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["schema_version"] != commandsSchemaVersion {
		t.Errorf("schema_version = %v, want %q", got["schema_version"], commandsSchemaVersion)
	}
	want := map[string]bool{"schema_version": true, "builtins": true, "coreutils": true, "verbs": true}
	for k := range got {
		if !want[k] {
			t.Errorf("unexpected key %q in default v1 output", k)
		}
	}
}

// --view classic is an explicit alias for the default output.
func TestAtlasViewClassicAliasesDefault(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	def, code1 := captureCommands(t)
	cls, code2 := captureCommands(t, "--view", "classic")
	if code1 != 0 || code2 != 0 {
		t.Fatalf("exit = %d/%d", code1, code2)
	}
	if def != cls {
		t.Errorf("--view classic output differs from default")
	}
}

func TestAtlasTextTierView(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--view", "tier", "--plain")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"tier userland", "tier workspace", "tier sandbox"} {
		if !strings.Contains(out, want) {
			t.Errorf("tier view missing %q header", want)
		}
	}
}

func TestFeaturesReportGainsAtlasKeys(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "grep", "--features")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got["group"] != "textutils" || got["tier"] != "userland" {
		t.Errorf("grep features: group=%v tier=%v", got["group"], got["tier"])
	}
	// Legacy keys unchanged.
	if got["class"] != "coreutils" || got["resolver"] != "bashy-in-process" {
		t.Errorf("legacy keys changed: %v", got)
	}
}
