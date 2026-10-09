//go:build !windows

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// `install-agent codex --yes` must hand chsh the caller's terminal: chsh asks
// for the account password, and with stdin detached it can only fail with a
// PAM authentication error.
func TestCodexYesChshReadsPasswordFromCallersStdin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	shim := filepath.Join(shimDir(), "bash")

	shells := filepath.Join(t.TempDir(), "shells")
	if err := os.WriteFile(shells, []byte(shim+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := etcShellsPath
	etcShellsPath = shells
	t.Cleanup(func() { etcShellsPath = old })

	binDir := t.TempDir()
	marker := filepath.Join(t.TempDir(), "chsh.args")
	script := "#!/bin/sh\nread -r pw || pw=\nif [ \"$pw\" != hunter2 ]; then echo 'chsh: PAM: Authentication failure' >&2; exit 1; fi\necho \"$@\" > " + marker + "\n"
	if err := os.WriteFile(filepath.Join(binDir, "chsh"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString("hunter2\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()
	oldStdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = oldStdin; r.Close() })

	target := filepath.Join(t.TempDir(), "bashy")
	if err := os.WriteFile(target, []byte("#!/bin/sh\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := codexInstaller(true).install(target, true); err != nil {
		t.Fatalf("install-agent codex --yes: %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || strings.TrimSpace(string(got)) != "-s "+shim {
		t.Fatalf("chsh not run with the shim: args %q, err %v", got, err)
	}
}
