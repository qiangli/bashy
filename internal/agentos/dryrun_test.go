// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"path/filepath"
	"testing"

	"mvdan.cc/sh/v3/expand"
)

func TestAnalyzeDestroy(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "build", "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "build", "a.o"), []byte("aaaa"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "build", "sub", "b.o"), []byte("bb"), 0o644)

	if d := analyzeDestroy("rm", []string{"rm", "-rf", "build"}, dir); d == nil ||
		d.Files != 2 || d.Bytes != 6 || !d.Recursive {
		t.Fatalf("rm -rf build: %+v", d)
	}
	if d := analyzeDestroy("rm", []string{"rm", "build"}, dir); d != nil {
		t.Errorf("rm on a dir without -r destroys nothing, got %+v", d)
	}
	if d := analyzeDestroy("rm", []string{"rm", filepath.Join(dir, "build", "a.o")}, dir); d == nil || d.Files != 1 {
		t.Errorf("rm single file: %+v", d)
	}
	if analyzeDestroy("rm", []string{"rm", "-rf", "ghost"}, dir) != nil {
		t.Error("rm of a missing path destroys nothing")
	}
	if analyzeDestroy("ls", []string{"ls", "-la"}, dir) != nil {
		t.Error("non-rm command is not destructive")
	}
}

func TestShQuote(t *testing.T) {
	cases := map[string]string{
		"hello":   "hello",
		"a/b-c.d": "a/b-c.d",
		"":        "''",
		"a b":     "'a b'",
		"it's":    `'it'\''s'`,
	}
	for in, want := range cases {
		if got := shQuote(in); got != want {
			t.Errorf("shQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveCmd(t *testing.T) {
	env := expand.ListEnviron("PATH=/nonexistent-dir")
	// A coreutils builtin resolves in-process (cmds/all is blank-imported).
	if got, ok := resolveCmd("cat", env); !ok || got != "coreutils:cat" {
		t.Errorf("resolveCmd(cat) = %q,%v", got, ok)
	}
	// A command found nowhere is reported missing.
	if _, ok := resolveCmd("totally-missing-tool-xyz", env); ok {
		t.Error("missing tool reported available")
	}
}
