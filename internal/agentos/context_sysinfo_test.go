// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"slices"
	"strings"
	"testing"
)

func TestCollectEnvironmentRedaction(t *testing.T) {
	t.Setenv("HOME", "/home/x")            // allowlisted -> shown with value
	t.Setenv("FAKE_API_KEY", "sekret")     // sensitive-shaped -> name-only in redacted
	t.Setenv("RANDOM_UNLISTED_VAR", "val") // not allowlisted -> name-only in redacted
	shown, redacted := collectEnvironment()
	if shown["HOME"] != "/home/x" {
		t.Errorf("HOME (allowlisted) should show real value, got %q", shown["HOME"])
	}
	if _, ok := shown["FAKE_API_KEY"]; ok {
		t.Error("a secret must never appear in the shown map")
	}
	names := strings.Split(redacted, ",")
	for _, want := range []string{"FAKE_API_KEY", "RANDOM_UNLISTED_VAR"} {
		if !slices.Contains(names, want) {
			t.Errorf("redacted list should name %q; got %q", want, redacted)
		}
	}
	if slices.Contains(names, "HOME") {
		t.Errorf("HOME (shown) must not be in the redacted names; got %q", redacted)
	}
	// The whole point: no per-var "<redacted>" padding — just names.
	if strings.Contains(redacted, "<redacted>") {
		t.Errorf("redacted list should be bare names, not padded values: %q", redacted)
	}
}

func TestEnvValueSafe(t *testing.T) {
	// Allowlisted, non-sensitive → shown; everything secret-shaped → hidden,
	// even names that resemble an allowlisted one.
	for name, want := range map[string]bool{
		"PATH": true, "HOME": true, "USER": true, "LANG": true, "TERM": true,
		"OPENAI_API_KEY": false, "AWS_SECRET_ACCESS_KEY": false, "MY_TOKEN": false,
		"NPM_TOKEN": false, "SOME_PASSWORD": false, "UNLISTED": false,
	} {
		if got := envValueSafe(name); got != want {
			t.Errorf("envValueSafe(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestCollectSystem(t *testing.T) {
	s := collectSystem()
	if s.Sysname == "" {
		t.Error("sysname should be set (uname, or GOOS fallback)")
	}
	if s.Machine == "" {
		t.Error("machine should be set (uname -m, or GOARCH fallback)")
	}
	if s.UID != os.Getuid() {
		t.Errorf("uid = %d, want %d", s.UID, os.Getuid())
	}
	if s.Home == "" {
		t.Error("home should be set")
	}
}
