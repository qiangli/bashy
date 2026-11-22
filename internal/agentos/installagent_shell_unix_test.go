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
	for _, agent := range []string{"agy", "muse", "codex"} {
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

func TestInstallAgentUsesLauncherIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("BASHY_SHIM_DIR", t.TempDir())
	t.Setenv("BASHY_PREFS", "")
	// os.Executable is this test binary, not the launcher. The same distinction
	// applies when install-agent is reached from a standalone command adapter.
	target := filepath.Join(t.TempDir(), "bashy launcher")
	if err := os.WriteFile(target, []byte("#!/bin/sh\nprintf 'launcher:%s\\n' \"$*\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_SELF", target)
	if code := dispatchInstallAgent([]string{"agy"}); code != 0 {
		t.Fatalf("install-agent returned %d", code)
	}
	for _, name := range shimNames {
		out, err := exec.Command(filepath.Join(shimDir(), name), "-c", "echo ok").CombinedOutput()
		if err != nil || string(out) != "launcher:-c echo ok\n" {
			t.Fatalf("%s shim: %v: %s", name, err, out)
		}
	}
}

func TestInstallAgentRejectsAdapterCacheTargets(t *testing.T) {
	home := t.TempDir()
	xdg := t.TempDir()
	cache := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", xdg)
	t.Setenv("BASHY_ADAPTER_CACHE", cache)
	t.Setenv("BASHY_SHIM_DIR", t.TempDir())
	t.Setenv("BASHY_PREFS", "")
	for _, root := range []string{cache, filepath.Join(xdg, "bashy", "adapters"), filepath.Join(home, ".cache", "bashy", "adapters")} {
		t.Run(root, func(t *testing.T) {
			target := filepath.Join(root, "yoke", "foreman")
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(t.TempDir(), "bashy")
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}
			for _, shell := range []string{target, link} {
				t.Setenv("BASHY_SELF", shell)
				for _, args := range [][]string{{"agy"}, {"agy", "--shell", shell}} {
					if code := dispatchInstallAgent(args); code == 0 {
						t.Fatalf("accepted adapter as shell: %v", args)
					}
				}
				for _, ins := range []agentInstaller{shimInstaller("agy"), codexInstaller(false)} {
					if _, err := ins.install(shell, false); err == nil {
						t.Fatalf("%s accepted adapter target %s", ins.name, shell)
					}
				}
			}
		})
	}
	for _, name := range shimNames {
		if _, err := os.Stat(filepath.Join(shimDir(), name)); !os.IsNotExist(err) {
			t.Fatalf("shim %s written: %v", name, err)
		}
	}
	// A neighboring directory is not inside the cache (avoid prefix matching).
	if err := validateShellShimTarget(cache + "-other/bashy"); err != nil {
		t.Fatal(err)
	}
}
