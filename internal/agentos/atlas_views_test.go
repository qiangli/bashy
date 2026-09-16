// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"runtime"
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
	var sb bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(&sb, r)
		done <- err
	}()
	code := dispatchCommands(args)
	w.Close()
	os.Stdout = prev
	if err := <-done; err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return sb.String(), code
}

func TestAtlasViewTierJSON(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--view", "tier", "--os", "any", "--json")
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
	for _, want := range []string{"weave", "sprint", "dag", "loom"} {
		if !slices.Contains(names, want) {
			t.Errorf("workspace tier missing %q (got %v)", want, names)
		}
	}
	// sdlc is curated-hidden (experimental): absent by default, present with --all.
	if slices.Contains(names, "sdlc") {
		t.Errorf("experimental sdlc leaked into the default workspace view")
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
	for _, want := range []string{"weave", "fetch", "ast", "kb"} {
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
	// `registered` (Sprint 179) is the one ADDITIVE key: the operator's ring,
	// always present as a list (empty here — TestMain isolates the ring).
	want := map[string]bool{"schema_version": true, "builtins": true, "coreutils": true, "verbs": true, "registered": true}
	for k := range got {
		if !want[k] {
			t.Errorf("unexpected key %q in default v1 output", k)
		}
	}
	if reg, ok := got["registered"].([]any); !ok || len(reg) != 0 {
		t.Errorf("registered = %v, want an empty list under an isolated ring", got["registered"])
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

// TestAtlasViewPosix: the certification view lists exactly the POSIX-required
// names the catalog provides, grouped by provider, and names what it cannot
// list (`sh`, a Preamble shim) instead of silently dropping it.
func TestAtlasViewPosix(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--view", "posix", "--os", "any", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got atlasJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Filter["posix"] != "true" {
		t.Errorf("filter = %v, want posix=true", got.Filter)
	}
	names := map[string]string{}
	for _, r := range got.Commands {
		if !r.Posix {
			t.Errorf("%s: not POSIX-required but in the posix view", r.Name)
		}
		names[r.Name] = r.Origin
	}
	if len(names) != 115 { // 116 required; `sh` is the Preamble shim, not a record
		t.Errorf("posix view lists %d names, want 115", len(names))
	}
	for name, origin := range map[string]string{"cd": "bash", "cat": "gnu", "awk": "unix", "m4": "external"} {
		if names[name] != origin {
			t.Errorf("%s: origin %q, want %q", name, names[name], origin)
		}
	}
	if _, ok := names["sh"]; ok {
		t.Errorf("sh is a shim, not a catalogued command")
	}
	text, code := captureCommands(t, "--view", "posix", "--os", "any")
	for _, want := range []string{"internal — in the bashy binary", "(105)", "bin-managed — exec'd", "(10)", "not listed (1): sh"} {
		if !strings.Contains(text, want) {
			t.Errorf("posix text view missing %q:\n%s", want, text)
		}
	}
	if code != 0 || !strings.Contains(text, "not listed (1): sh") {
		t.Errorf("text view must name the unlisted sh shim:\n%s", text)
	}
}

// TestAtlasViewExternal: the bin-managed view lists exactly the exec'd names
// and counts the POSIX-required pinned providers as the pure-Go debt.
func TestAtlasViewExternal(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--view", "external", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got atlasJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Filter["origin"] != "external" {
		t.Errorf("filter = %v, want origin=external", got.Filter)
	}
	names := map[string]bool{}
	for _, r := range got.Commands {
		if r.Origin != "external" {
			t.Errorf("%s: origin %q in the external view", r.Name, r.Origin)
		}
		names[r.Name] = true
	}
	wantExternal := []string{"git", "kubectl", "go", "doctl"}
	wantDebt := "pure-Go debt: 10 POSIX-required"
	if runtime.GOOS == "windows" {
		wantDebt = "pure-Go debt: 0 POSIX-required"
	} else {
		wantExternal = append(wantExternal, "m4", "posix-providers")
	}
	for _, want := range wantExternal {
		if !names[want] {
			t.Errorf("external view missing %q", want)
		}
	}
	for _, no := range []string{"awk", "cat", "weave", "cd"} {
		if names[no] {
			t.Errorf("external view must not list %q", no)
		}
	}
	text, code := captureCommands(t, "--view", "external")
	if code != 0 || !strings.Contains(text, wantDebt) {
		t.Errorf("text view must count the POSIX provider debt:\n%s", text)
	}
}

// TestPlatformFilterDefaultsToHost: the default listing is this host's; a
// name the atlas says this OS cannot run is absent from it, present under
// --os any, and still answered by name.
func TestPlatformFilterDefaultsToHost(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--os", "windows", "--view", "origin", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got atlasJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Filter["os"] != "windows" {
		t.Errorf("filter = %v", got.Filter)
	}
	names := map[string]bool{}
	for _, r := range got.Commands {
		names[r.Name] = true
		if !slices.Contains(r.OS, "windows") {
			t.Errorf("%s: os %v leaked through the windows filter", r.Name, r.OS)
		}
	}
	for _, unix := range []string{"mkfifo", "chown", "ps", "m4", "ollama"} {
		if names[unix] {
			t.Errorf("%s listed for windows", unix)
		}
	}
	for _, ok := range []string{"cat", "grep", "weave", "oci", "more"} { // more: partial, still supported
		if !names[ok] {
			t.Errorf("%s missing for windows", ok)
		}
	}
	// --os any lifts the filter; --all implies it.
	for _, args := range [][]string{{"--os", "any", "--view", "origin", "--json"}, {"--all", "--view", "origin", "--json"}} {
		out, _ = captureCommands(t, args...)
		got = atlasJSON{}
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if got.Filter["os"] != "" {
			t.Errorf("%v: os filter still set: %v", args, got.Filter)
		}
		found := false
		for _, r := range got.Commands {
			if r.Name == "ps" {
				found = true
			}
		}
		if !found {
			t.Errorf("%v: ps (linux-only) must be listed when the platform filter is lifted", args)
		}
	}
	// Name lookup is never filtered: ps answers on every host.
	out, code = captureCommands(t, "ps")
	if code != 0 || !strings.Contains(out, "only: linux") && !strings.Contains(out, "only on linux") {
		t.Errorf("ps by name: exit %d\n%s", code, out)
	}
	if _, code = captureCommands(t, "--os", "plan9"); code != 2 {
		t.Errorf("unknown os must exit 2, got %d", code)
	}
}

// TestPortableFilterComposes: --portable narrows any view to full-support
// commands, and --view portable lists them by origin with the rest explained.
func TestPortableFilterComposes(t *testing.T) {
	t.Setenv("BASHY_AGENTIC", "")
	out, code := captureCommands(t, "--view", "posix", "--portable", "--json")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	var got atlasJSON
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Filter["portable"] != "true" || got.Filter["posix"] != "true" {
		t.Errorf("filter = %v", got.Filter)
	}
	for _, r := range got.Commands {
		if !r.Portable || !r.Posix {
			t.Errorf("%s: portable=%v posix=%v in the portable posix view", r.Name, r.Portable, r.Posix)
		}
		if r.Name == "chown" || r.Name == "more" || r.Name == "m4" {
			t.Errorf("%s is not portable", r.Name)
		}
	}
	text, code := captureCommands(t, "--view", "portable")
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	for _, want := range []string{"portable — runs as-is on windows, macOS and linux", "not on every platform", "everywhere, with a documented gap", "mkfifo (darwin,linux)", "more (partial on windows)"} {
		if !strings.Contains(text, want) {
			t.Errorf("portable view missing %q:\n%s", want, text)
		}
	}
}
