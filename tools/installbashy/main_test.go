// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDefaultInstallDirUsesSharedDhntBin(t *testing.T) {
	t.Setenv("BASHY_INSTALL_DIR", filepath.Join(t.TempDir(), "legacy"))
	want := filepath.Join(t.TempDir(), "dhnt")
	t.Setenv("DHNT_BIN_DIR", want)
	if got := defaultInstallDir(); got != want {
		t.Fatalf("default install dir = %q, want DHNT_BIN_DIR %q", got, want)
	}
	t.Setenv("DHNT_BIN_DIR", "")
	if got := defaultInstallDir(); got != os.Getenv("BASHY_INSTALL_DIR") {
		t.Fatalf("legacy fallback = %q, want %q", got, os.Getenv("BASHY_INSTALL_DIR"))
	}
}

func TestVerifyBashySurfaceRequiresAgentOSVerbs(t *testing.T) {
	complete := append([]string(nil), requiredAgentOSVerbs...)
	var calls [][]string
	run := func(_ string, args ...string) ([]byte, error) {
		calls = append(calls, append([]string(nil), args...))
		switch {
		case reflect.DeepEqual(args, []string{"-c", "-l", "echo ok"}):
			return []byte("ok\n"), nil
		case reflect.DeepEqual(args, []string{"commands", "--json"}):
			return []byte(`{"verbs":["` + strings.Join(complete, `","`) + `"]}`), nil
		default:
			return []byte("Usage: bashy " + args[0]), nil
		}
	}
	if err := verifyBashySurface("/tmp/bashy", run); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 4 {
		t.Fatalf("probe count = %d, want 4", len(calls))
	}

	incomplete := func(_ string, args ...string) ([]byte, error) {
		if reflect.DeepEqual(args, []string{"-c", "-l", "echo ok"}) {
			return []byte("ok\n"), nil
		}
		if reflect.DeepEqual(args, []string{"commands", "--json"}) {
			return []byte(`{"verbs":["commands","weave"]}`), nil
		}
		return nil, nil
	}
	err := verifyBashySurface("/tmp/bashy", incomplete)
	if err == nil || !strings.Contains(err.Error(), "judge") || !strings.Contains(err.Error(), "agents") {
		t.Fatalf("missing AgentOS verbs error = %v", err)
	}
}

func TestInstallExecutableReplacesExistingFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new")
	dst := filepath.Join(dir, "bin", executableName("bashy"))
	if err := os.WriteFile(src, []byte("new binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("stale binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := installExecutable(src, dst); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Fatalf("installed body = %q", got)
	}
}
