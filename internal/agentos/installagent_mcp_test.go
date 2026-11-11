// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The --dry-run entries are the contract other docs and scripts quote: pin
// them exactly. All three were verified against the clients themselves
// (claude/codex --help; codex writing to a scratch CODEX_HOME; the opencode
// config schema's McpLocalConfig). The server command is the running
// binary's absolute path, never the bare name a fresh PATH may not resolve.

// fakeSelfExe stands in for the running bashy in the pinned entries.
const fakeSelfExe = "/opt/bashy/bin/bashy"

func withFakeSelfExe(t *testing.T) {
	t.Helper()
	orig := mcpSelfExecutable
	mcpSelfExecutable = func() (string, error) { return fakeSelfExe, nil }
	t.Cleanup(func() { mcpSelfExecutable = orig })
}

func TestInstallAgentMCPServerCommand(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "bashy-real")
	if err := os.WriteFile(real, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "bashy")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	orig := mcpSelfExecutable
	t.Cleanup(func() { mcpSelfExecutable = orig })
	mcpSelfExecutable = func() (string, error) { return link, nil }
	want, _ := filepath.EvalSymlinks(real)
	if got := mcpServerCommand(); got != want {
		t.Fatalf("mcpServerCommand() = %q, want %q (symlinks resolved)", got, want)
	}
	mcpSelfExecutable = func() (string, error) { return "", os.ErrNotExist }
	if got := mcpServerCommand(); got != "bashy" {
		t.Fatalf("fallback = %q, want bare name", got)
	}
}

func TestInstallAgentMCPDryRunClaude(t *testing.T) {
	withFakeSelfExe(t)
	w, ok := mcpWriters()["claude"]
	if !ok {
		t.Fatal("no MCP config writer for claude")
	}
	if got := w.entry(false); got != "claude mcp add bashy -- "+fakeSelfExe+" mcp serve\n" {
		t.Fatalf("claude entry = %q", got)
	}
	if got := w.entry(true); got != "claude mcp add -s project bashy -- "+fakeSelfExe+" mcp serve\n" {
		t.Fatalf("claude project entry = %q", got)
	}
}

func TestInstallAgentMCPDryRunCodex(t *testing.T) {
	withFakeSelfExe(t)
	w, ok := mcpWriters()["codex"]
	if !ok {
		t.Fatal("no MCP config writer for codex")
	}
	want := "[mcp_servers.bashy]\ncommand = \"" + fakeSelfExe + "\"\nargs = [\"mcp\", \"serve\"]\n"
	if got := w.entry(false); got != want {
		t.Fatalf("codex entry = %q, want %q", got, want)
	}
}

func TestInstallAgentMCPDryRunOpencode(t *testing.T) {
	withFakeSelfExe(t)
	w, ok := mcpWriters()["opencode"]
	if !ok {
		t.Fatal("no MCP config writer for opencode")
	}
	got := w.entry(false)
	for _, want := range []string{`"mcp"`, `"bashy"`, `"type": "local"`, `"` + fakeSelfExe + `",`, `"mcp",`, `"serve"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("opencode entry = %q, missing %s", got, want)
		}
	}
}

func TestInstallAgentMCPUnsupported(t *testing.T) {
	for _, name := range []string{"aider", "gemini", "copilot", "agy", "antigravity"} {
		if _, ok := mcpWriters()[name]; ok {
			t.Fatalf("%s unexpectedly has an MCP config writer", name)
		}
	}
	if got := dispatchInstallAgentMCP("aider", false, false, false, false, false); got != 1 {
		t.Fatalf("dispatchInstallAgentMCP(aider) = %d, want 1", got)
	}
}

// Install and uninstall round-trip through a scratch HOME without touching
// the user's real agent configs. Other tables in config.toml must survive.
func TestInstallAgentMCPCodexFileRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	w := mcpWriters()["codex"]
	path := codexMCPConfigPath()
	if dir := filepath.Dir(path); !strings.HasSuffix(dir, filepath.Join(".codex")) {
		t.Fatalf("codexMCPConfigPath = %q, want $HOME/.codex/config.toml", path)
	}
	seed := "[model]\nname = \"o3\"\n"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := w.install(false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), seed) {
		t.Fatalf("install clobbered existing config: %q", data)
	}
	if !strings.Contains(string(data), w.entry(false)) {
		t.Fatalf("install missing entry: %q", data)
	}
	// Installing twice keeps exactly one copy.
	if _, err := w.install(false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if n := strings.Count(string(data), "[mcp_servers.bashy]"); n != 1 {
		t.Fatalf("double install wrote %d sections: %q", n, data)
	}
	if _, err := w.uninstall(false); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if strings.Contains(string(data), "[mcp_servers.bashy]") {
		t.Fatalf("uninstall left the section: %q", data)
	}
	if !strings.Contains(string(data), seed) {
		t.Fatalf("uninstall clobbered existing config: %q", data)
	}
}

func TestInstallAgentMCPOpencodeMergeRoundTrip(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	w := mcpWriters()["opencode"]
	if _, err := w.install(false); err != nil {
		t.Fatal(err)
	}
	m, err := readJSONFile(opencodeConfigPath(false))
	if err != nil {
		t.Fatal(err)
	}
	servers, _ := m["mcp"].(map[string]any)
	entry, _ := servers["bashy"].(map[string]any)
	if entry["type"] != "local" {
		t.Fatalf("mcp.bashy = %v", servers["bashy"])
	}
	// A sibling server entry survives install and uninstall.
	m["mcp"].(map[string]any)["other"] = map[string]any{"type": "local", "command": []string{"other"}}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(opencodeConfigPath(false), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := w.install(false); err != nil {
		t.Fatal(err)
	}
	if _, err := w.uninstall(false); err != nil {
		t.Fatal(err)
	}
	m, err = readJSONFile(opencodeConfigPath(false))
	if err != nil {
		t.Fatal(err)
	}
	servers, _ = m["mcp"].(map[string]any)
	if _, ok := servers["bashy"]; ok {
		t.Fatalf("uninstall left mcp.bashy: %v", servers)
	}
	if servers["other"] == nil {
		t.Fatalf("uninstall removed sibling entry: %v", servers)
	}
}
