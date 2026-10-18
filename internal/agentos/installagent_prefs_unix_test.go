//go:build !windows

package agentos

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const prefsFixture = `{"budget_bytes":2048,"view":"json","defaults":{"*":{"api_token":"sk-LEAK","limit":3}}}`

func TestInstallAgentProjectsPrefsToShim(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_PREFS", prefsFixture)
	target := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nprintf '%s' \"$BASHY_PREFS\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := shimInstaller("gemini").install(target, false); err != nil {
		t.Fatal(err)
	}
	// Agent process starts without BASHY_PREFS; the shim must supply it.
	cmd := exec.Command(filepath.Join(shimDir(), "bash"))
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(out))
	}
	got := string(out)
	if !strings.Contains(got, `"budget_bytes":2048`) || !strings.Contains(got, `"limit":3`) {
		t.Fatalf("prefs not projected: %q", got)
	}
	if strings.Contains(got, "sk-LEAK") || strings.Contains(got, "api_token") {
		t.Fatalf("secret leaked: %q", got)
	}
}

func TestInstallAgentNoPrefsLeavesShimCanonical(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_PREFS", "")
	target := filepath.Join(t.TempDir(), "fake")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := shimInstaller("gemini").install(target, false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(shimDir(), "bash"))
	if strings.Contains(string(b), "BASHY_PREFS") {
		t.Fatalf("unexpected projection: %s", b)
	}
}

func TestInstallAgentClaudePrefsProjection(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_PREFS", prefsFixture)
	ins := claudeInstaller()
	if _, err := ins.install("/bin/sh", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(claudeSettingsPath(false))
	if !strings.Contains(string(b), "BASHY_PREFS") || strings.Contains(string(b), "sk-LEAK") {
		t.Fatalf("settings: %s", b)
	}
	if _, err := ins.uninstall(false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(claudeSettingsPath(false))
	if strings.Contains(string(b), "BASHY_PREFS") {
		t.Fatalf("not removed: %s", b)
	}
}

func TestInstallAgentMalformedPrefsFailsClosed(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_PREFS", `{"budget`)
	if _, err := claudeInstaller().install("/bin/sh", false); err == nil || !strings.Contains(err.Error(), "BASHY_PREFS") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(claudeSettingsPath(false)); err == nil {
		t.Fatal("settings written despite malformed prefs")
	}
}
