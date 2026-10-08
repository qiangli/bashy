//go:build !windows

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestInstallAgentShimPreservesAgentOSEntryPoint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	// A fake target reports argv[0]. Symlinks select the product's plain-shell
	// route as bash/sh, losing AgentOS and its execution log.
	target := filepath.Join(t.TempDir(), "bashy's shell")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nprintf '%s\\n' \"$0\"\nprintf '%s\\n' \"$@\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"agy", "gemini", "copilot", "codex"} {
		t.Run(agent, func(t *testing.T) {
			ins := shimInstaller(agent)
			if agent == "codex" {
				ins = codexInstaller(false)
			}
			if _, err := ins.install(target, false); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(filepath.Join(shimDir(), "bash"), "-c", "a b").CombinedOutput()
			want := target + "\n-c\na b\n"
			if err != nil || string(out) != want {
				t.Fatalf("shim lost target identity/argv: got %q, err %v; want %q", out, err, want)
			}
		})
	}
}
