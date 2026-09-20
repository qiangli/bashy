// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"testing"

	"github.com/qiangli/yoke/pkg/reduce"
)

// Stage 0 canonicalizes the home directory only where the `$HOME` token
// round-trips through the shell. On Windows the environment's HOME is the
// native spelling while the shell's is MSYS, so `C:\Users\x\s213` would become
// `$HOME\s213` — a path no lookup can use once pasted back (todo 1ec1081071d7,
// the tour's commands/register case). There, nothing is rewritten.
func TestDisplayHomeIsEmptyOnWindows(t *testing.T) {
	cases := []struct {
		goos, home, want string
	}{
		{"linux", "/home/alice", "/home/alice"},
		{"darwin", "/Users/alice", "/Users/alice"},
		{"windows", `C:\Users\alice`, ""},
		{"windows", "/c/Users/alice", ""},
		{"windows", "", ""},
	}
	for _, c := range cases {
		if got := displayHome(c.home, c.goos); got != c.want {
			t.Errorf("displayHome(%q, %s) = %q, want %q", c.home, c.goos, got, c.want)
		}
	}
}

// The failure the todo recorded, replayed through the same canonicalizer the
// reducer calls: with the native Windows home, a PATH list is rewritten; with
// the home displayHome hands the reducer on Windows, it is left alone.
func TestWindowsPathListIsNeverRewrittenToLiteralHome(t *testing.T) {
	in := []byte(`C:\Windows\System32;C:\Windows;C:\Users\alice\s213` + "\n")
	if got := reduce.CanonicalizeHome(in, displayHome(`C:\Users\alice`, "windows")); string(got) != string(in) {
		t.Fatalf("windows PATH display rewritten: %q", got)
	}
}
