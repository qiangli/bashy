// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build e2e

package agentos

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestE2EInspectActionsJSON drives the built binary: `bashy inspect actions
// --json` parses as a bashy-inspect-v1 envelope and carries at least one row
// of each family something fills today — a command (the atlas), an agent (one
// binding seeded into a scratch fleet store with `agent add`), and a skill (the
// embedded ring). Hermetic: every store is a t.TempDir().
func TestE2EInspectActionsJSON(t *testing.T) {
	bin := bashyBinary(t)
	var env []string
	for _, key := range []string{
		"BASHY_HOME", "BASHY_FLEET_DIR", "BASHY_SKILLS_DIR", "BASHY_TOOLS_DIR",
		"BASHY_MODELS_DIR", "BASHY_AGENTS_DIR",
	} {
		env = append(env, key+"="+t.TempDir())
	}
	env = append(env, "BASHY_SKILLS_PATH=")

	stdout, stderr, code := runBashyStdEnv(bin, env, "agent", "add", "b4-probe", "--set", "tool=codex", "--set", "model=gpt5.6-sol", "--ephemeral")
	if code != 0 {
		t.Fatalf("agent add (exit %d):\n%s%s", code, stdout, stderr)
	}

	stdout, stderr, code = runBashyStdEnv(bin, env, "inspect", "actions", "--json")
	if code != 0 {
		t.Fatalf("inspect actions --json (exit %d):\n%s%s", code, stdout, stderr)
	}
	var doc struct {
		SchemaVersion string             `json:"schema_version"`
		Aspect        string             `json:"aspect"`
		Rows          []inspectActionRow `json:"rows"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not one JSON document: %v\n%s", err, stdout)
	}
	if doc.SchemaVersion != inspectSchemaVersion || doc.Aspect != "actions" {
		t.Fatalf("envelope = %s/%s", doc.SchemaVersion, doc.Aspect)
	}
	seen := map[string]inspectActionRow{}
	for _, r := range doc.Rows {
		seen[r.Kind+"/"+r.Name] = r
	}
	if r, ok := seen["command/bashy run"]; !ok || r.Identity != "verb:run" || r.Executor != "verb" {
		t.Errorf("no command row for bashy run: %+v", r)
	}
	if r, ok := seen["agent/b4-probe"]; !ok || r.Identity != "codex:gpt5.6-sol" || r.Latitude != "judge" {
		t.Errorf("the seeded agent did not project: %+v (fleet store not read?)", r)
	}
	if r, ok := seen["skill/conductor"]; !ok || r.Contract != "dhnt" || !strings.HasPrefix(r.Identity, "h") {
		t.Errorf("no embedded skill row for conductor: %+v", r)
	}
	// Generic half only: nothing in the document names a store or a home.
	for _, kv := range env {
		if v := strings.SplitN(kv, "=", 2)[1]; v != "" && strings.Contains(stdout, v) {
			t.Errorf("inspect actions --json leaks a store path (%s)", strings.SplitN(kv, "=", 2)[0])
		}
	}
	if home := os.Getenv("HOME"); home != "" && strings.Contains(stdout, home) {
		t.Error("inspect actions --json leaks the home directory")
	}

	// --kind filters to one family; an unknown kind is refused with the vocabulary.
	stdout, _, code = runBashyStdEnv(bin, env, "inspect", "actions", "--json", "--kind", "skill")
	if code != 0 || strings.Contains(stdout, `"kind":"command"`) || !strings.Contains(stdout, `"kind":"skill"`) {
		t.Errorf("--kind skill (exit %d):\n%s", code, stdout)
	}
	_, stderr, code = runBashyStdEnv(bin, env, "inspect", "actions", "--kind", "nonsense")
	if code != 2 || !strings.Contains(stderr, "command script agent skill") {
		t.Errorf("--kind nonsense exited %d without the vocabulary:\n%s", code, stderr)
	}

	// The first-hop record carries the four counts, script stated as 0.
	stdout, stderr, code = runBashyStdEnv(bin, env, "inspect", "context", "--json")
	if code != 0 {
		t.Fatalf("inspect context --json (exit %d):\n%s%s", code, stdout, stderr)
	}
	var ctx struct {
		Actions map[string]int `json:"actions"`
	}
	if err := json.Unmarshal([]byte(stdout), &ctx); err != nil {
		t.Fatalf("context is not JSON: %v", err)
	}
	for _, k := range []string{"command", "script", "agent", "skill"} {
		if _, ok := ctx.Actions[k]; !ok {
			t.Errorf("context.actions lacks %q", k)
		}
	}
	if ctx.Actions["command"] == 0 || ctx.Actions["agent"] == 0 || ctx.Actions["skill"] == 0 || ctx.Actions["script"] != 0 {
		t.Errorf("context.actions = %v", ctx.Actions)
	}
}
