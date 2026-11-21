package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/bashy/custom/internal/toolshortcut"
)

func TestAgentCLIDynamicRingAndDeps(t *testing.T) {
	adapter.AssertStandaloneDeps(t, "agent-cli", "github.com/qiangli/bashy/custom/internal/toolshortcut")

	dir := t.TempDir()
	yaml := "name: customsteer\nkind: cli\ndisplay: Custom Steer\ncli:\n  binary: customsteer-bin\n  launch:\n    exec: customsteer-bin -p {prompt}\n    steer_exec: customsteer-bin\n"
	if err := os.WriteFile(filepath.Join(dir, "customsteer.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_TOOLS_PATH", dir)

	var out, errBuf bytes.Buffer
	if code := toolshortcut.DispatchToolShortcut("customsteer", []string{"--dry-run"}, nil, &out, &errBuf); code != 0 || !strings.Contains(out.String(), "session: customsteer") {
		t.Fatalf("customsteer --dry-run: code=%d out=%q err=%q", code, out.String(), errBuf.String())
	}

	if runtime.GOOS != "windows" {
		fakeDir := t.TempDir()
		fake := filepath.Join(fakeDir, "customsteer-bin")
		if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 5\n"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PATH", fakeDir)
		out.Reset()
		errBuf.Reset()
		if code := toolshortcut.DispatchToolShortcut("customsteer", []string{"--", "--version"}, nil, &out, &errBuf); code != 5 {
			t.Fatalf("customsteer -- --version: code=%d want 5 (err=%q)", code, errBuf.String())
		}
	}
}
