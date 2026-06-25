// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"testing"

	"mvdan.cc/sh/v3/expand"
)

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
