// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/reduce"
	"github.com/spf13/cobra"
)

// captureStdout runs fn with os.Stdout replaced by a pipe and returns what it
// wrote. The front doors under test print straight to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	func() {
		defer func() { os.Stdout = old; _ = w.Close() }()
		fn()
	}()
	out := <-done
	_ = r.Close()
	return out
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// decodeEnvelope decodes a single top-level JSON object and asserts its
// schema_version is the versioned name the release bar probes for.
func decodeEnvelope(t *testing.T, out, schema string) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("output is not one JSON object: %v\n%s", err, out)
	}
	if got := env["schema_version"]; got != schema {
		t.Fatalf("schema_version = %v, want %q\n%s", got, schema, out)
	}
	return env
}

func TestFleetListJSONCarriesVersionedEnvelope(t *testing.T) {
	t.Setenv("BASHY_FLEET_DIR", t.TempDir())
	cases := map[string]func() *cobra.Command{
		"tool":  func() *cobra.Command { return fleet.NewToolsCmd() },
		"model": func() *cobra.Command { return fleet.NewModelsCmd() },
		"agent": func() *cobra.Command { return fleet.NewAgentsCmd() },
	}
	for noun, build := range cases {
		for _, argv := range [][]string{{"list", "--json"}, {"--json"}} {
			if noun == "agent" && len(argv) == 1 {
				continue // the bare `agent --json` is bashy's roster (bashy-agents-v1), not the catalog
			}
			cmd := build()
			wrapFleetListEnvelope(cmd, noun, true)
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(io.Discard)
			cmd.SetArgs(argv)
			if err := cmd.Execute(); err != nil {
				t.Fatalf("%s %v: %v", noun, argv, err)
			}
			env := decodeEnvelope(t, out.String(), fleetListSchemaVersion)
			if env["kind"] != noun {
				t.Errorf("%s %v: kind = %v", noun, argv, env["kind"])
			}
			items, ok := env["items"].([]any)
			if !ok || len(items) == 0 {
				t.Errorf("%s %v: items missing or empty: %v", noun, argv, env["items"])
			}
		}
	}
}

func TestFleetListEnvelopeLeavesTextAndObjectsAlone(t *testing.T) {
	t.Setenv("BASHY_FLEET_DIR", t.TempDir())
	cmd := fleet.NewToolsCmd()
	wrapFleetListEnvelope(cmd, "tool", true)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"list"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "NAME ") {
		t.Errorf("text listing changed: %q", firstLine(out.String()))
	}
	// Once the registry emits its own envelope the front door must pass it
	// through byte-for-byte instead of nesting a second one.
	native := []byte("{\n  \"schema_version\": \"bashy-fleet-list-v2\"\n}\n")
	var got bytes.Buffer
	if err := writeFleetListEnvelope(&got, "tool", native); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), native) {
		t.Errorf("object output was rewrapped: %s", got.String())
	}
}

func TestConformListJSONCarriesVersionedEnvelope(t *testing.T) {
	cmd := verifyCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--list", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	env := decodeEnvelope(t, out.String(), conformSchemaVersion)
	suites, _ := env["suites"].([]any)
	if len(suites) != len(suiteRegistry()) {
		t.Fatalf("suites = %d, want %d", len(suites), len(suiteRegistry()))
	}
	first, _ := suites[0].(map[string]any)
	for _, k := range []string{"name", "kind", "summary", "license", "ready"} {
		if _, ok := first[k]; !ok {
			t.Errorf("suite row lacks %q: %v", k, first)
		}
	}
}

func TestInstallAgentStatusJSONCarriesVersionedEnvelope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	// The aider check runs `SHELL -i -c "echo ok"`; the default shell is this
	// process, which under `go test` is the test binary — so name a real
	// shell, as an operator with a relocated bashy would.
	shell := "/bin/sh"
	if runtime.GOOS == "windows" {
		shell = os.Getenv("COMSPEC")
	}
	out := captureStdout(t, func() {
		if code := dispatchInstallAgent([]string{"--json", "--shell", shell}); code != 0 {
			t.Errorf("install-agent --json exited %d", code)
		}
	})
	env := decodeEnvelope(t, out, installAgentSchemaVersion)
	agents, _ := env["agents"].([]any)
	if len(agents) == 0 {
		t.Fatalf("no agents in status envelope: %s", out)
	}
	row, _ := agents[0].(map[string]any)
	for _, k := range []string{"name", "installed", "wired"} {
		if _, ok := row[k]; !ok {
			t.Errorf("status row lacks %q: %v", k, row)
		}
	}
	if env["shell"] == "" {
		t.Errorf("shell is empty: %s", out)
	}
	// The per-agent check keeps the same envelope and reports the failure
	// inside it instead of on stderr, so an agent can act on the verdict.
	out = captureStdout(t, func() {
		_ = dispatchInstallAgent([]string{"claude", "--check", "--json", "--shell", shell})
	})
	env = decodeEnvelope(t, out, installAgentSchemaVersion)
	if env["agent"] != "claude" {
		t.Errorf("check envelope agent = %v", env["agent"])
	}
	if _, ok := env["wired"].(bool); !ok {
		t.Errorf("check envelope lacks a boolean wired: %s", out)
	}
}

func TestMCPToolsJSONCarriesVersionedEnvelope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	out := captureStdout(t, func() {
		if code := dispatchMCP([]string{"tools", "--json"}); code != 0 {
			t.Errorf("mcp tools --json exited %d", code)
		}
	})
	env := decodeEnvelope(t, out, mcpToolsSchemaVersion)
	if env["profile"] != "default" {
		t.Errorf("profile = %v", env["profile"])
	}
	tools, _ := env["tools"].([]any)
	if len(tools) == 0 {
		t.Errorf("default profile serves no tools: %s", out)
	}
	text := captureStdout(t, func() {
		if code := dispatchMCP([]string{"tools"}); code != 0 {
			t.Errorf("mcp tools exited %d", code)
		}
	})
	if strings.HasPrefix(text, "{") || strings.TrimSpace(text) == "" {
		t.Errorf("text listing is wrong: %q", firstLine(text))
	}
	if code := dispatchMCP([]string{"tools", "--tools", ""}); code != 2 {
		t.Errorf("empty --tools exited %d, want 2", code)
	}
}

func TestOutListAndJSONCarryVersionedEnvelope(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	out := captureStdout(t, func() {
		if code := dispatchOut([]string{"--list", "--json"}); code != 0 {
			t.Errorf("out --list --json exited %d", code)
		}
	})
	env := decodeEnvelope(t, out, outSchemaVersion)
	if items, ok := env["artifacts"].([]any); !ok || len(items) != 0 {
		t.Errorf("empty store should list zero artifacts: %s", out)
	}
	store := reduce.NewStore(filepath.Join(home, "exec", "output"))
	digest, err := store.Put([]byte("alpha\nbeta\n"))
	if err != nil {
		t.Fatal(err)
	}
	handle := strings.TrimPrefix(digest, "sha256:")[:8]
	out = captureStdout(t, func() {
		if code := dispatchOut([]string{"--json", handle}); code != 0 {
			t.Errorf("out --json %s exited %d", handle, code)
		}
	})
	env = decodeEnvelope(t, out, outSchemaVersion)
	if env["content"] != "alpha\nbeta\n" || env["digest"] != digest {
		t.Errorf("recovered envelope = %s", out)
	}
	if env["lines"] != float64(2) || env["bytes"] != float64(11) {
		t.Errorf("counts = lines %v bytes %v", env["lines"], env["bytes"])
	}
	out = captureStdout(t, func() {
		if code := dispatchOut([]string{"--list", "--json"}); code != 0 {
			t.Errorf("out --list --json exited %d", code)
		}
	})
	env = decodeEnvelope(t, out, outSchemaVersion)
	if items, _ := env["artifacts"].([]any); len(items) != 1 {
		t.Errorf("store with one blob lists %d artifacts: %s", len(items), out)
	}
	// Plain recovery is unchanged: raw bytes, no envelope.
	out = captureStdout(t, func() { _ = dispatchOut([]string{handle}) })
	if out != "alpha\nbeta\n" {
		t.Errorf("plain out = %q", out)
	}
}

func TestOllamaStatusJSONCarriesVersionedEnvelope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("OLLAMA_HOST", "")
	out := captureStdout(t, func() {
		if code := runOllamaStatus([]string{"--json"}); code != 0 {
			t.Errorf("ollama status --json exited %d", code)
		}
	})
	env := decodeEnvelope(t, out, ollamaStatusSchemaVersion)
	if env["engine"] != "ollama" || env["build"] == "" {
		t.Errorf("status envelope = %s", out)
	}
	if _, ok := env["available"].(bool); !ok {
		t.Errorf("available is not a bool: %s", out)
	}
	if code := runOllamaStatus([]string{"extra"}); code != 2 {
		t.Errorf("unexpected argument exited %d, want 2", code)
	}
}

func TestReleaseBarInventoryDeclaresProbesForModelAgentAndInspectionVerbs(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", "release-bar-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Commands []struct {
			Command   string   `json:"command"`
			Section   string   `json:"section"`
			Probe     []string `json:"json_probe"`
			Consumer  string   `json:"consumer"`
			Reference string   `json:"consumer_ref"`
		} `json:"commands"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	slice := map[string]bool{"models": true, "tools": true, "agents": true, "model": true, "tool": true, "agent": true,
		"chat": true, "verify": true, "conform": true, "llm": true, "ollama": true, "mcp": true, "install-agent": true, "out": true}
	seen := map[string]bool{}
	for _, row := range manifest.Commands {
		if !slice[row.Command] {
			continue
		}
		seen[row.Command] = true
		if len(row.Probe) == 0 || !strings.Contains(strings.Join(row.Probe, " "), "--json") {
			t.Errorf("%s: no --json probe declared", row.Command)
		}
		if (row.Consumer == "") != (row.Reference == "") {
			t.Errorf("%s: consumer and consumer_ref must be set together", row.Command)
		}
	}
	for name := range slice {
		if !seen[name] {
			t.Errorf("%s: missing from the release inventory", name)
		}
	}
}
