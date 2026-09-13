// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/kb"
)

func isolateKBCommandTest(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("BASHY_KB_DIR", filepath.Join(root, "host-kb"))
	t.Setenv("BASHY_HOME", filepath.Join(root, "home"))
	t.Setenv("BASHY_SKILLS_DIR", filepath.Join(root, "skills"))
	t.Setenv("YCODE_DATA_DIR", filepath.Join(root, "agent-data"))
	if err := os.MkdirAll(filepath.Join(root, "host-kb"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestKBContextRefusesSingleStoreFlags(t *testing.T) {
	for _, flag := range []string{"--dir", "--repo", "--user", "--base-dir"} {
		t.Run(strings.TrimPrefix(flag, "--"), func(t *testing.T) {
			root := isolateKBCommandTest(t)
			t.Chdir(root)
			cmd := kb.NewKBCmd()
			cmd.AddCommand(newKBContextCmd())
			args := []string{"context", "--for", "task", flag}
			if flag == "--dir" || flag == "--base-dir" {
				args = append(args, root)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "selects ONE kb store") {
				t.Fatalf("%s error = %v, want explicit single-store refusal", flag, err)
			}
		})
	}
}

func TestKBContextSuppressesSingleStoreScopeHeader(t *testing.T) {
	root := isolateKBCommandTest(t)
	t.Chdir(root)
	cmd := kb.NewKBCmd()
	cmd.AddCommand(newKBContextCmd())
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{"context", "--for", "missing topic", "--rings", "host", "--forms", "note", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr.String(), "kb [") {
		t.Fatalf("context leaked kb single-store scope header: %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), `"context_version": 1`) {
		t.Fatalf("context output missing frozen envelope version: %s", stdout.String())
	}
}
