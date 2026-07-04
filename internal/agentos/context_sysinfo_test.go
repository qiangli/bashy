// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"testing"
)

func TestCollectEnvironmentRedaction(t *testing.T) {
	t.Setenv("HOME", "/home/x")            // allowlisted -> real value
	t.Setenv("FAKE_API_KEY", "sekret")     // sensitive-shaped -> redacted
	t.Setenv("RANDOM_UNLISTED_VAR", "val") // not allowlisted -> redacted
	t.Setenv("EMPTY_ONE", "")              // empty -> present, empty
	env := collectEnvironment()
	if env["HOME"] != "/home/x" {
		t.Errorf("HOME (allowlisted) should show real value, got %q", env["HOME"])
	}
	if env["FAKE_API_KEY"] != "<redacted>" {
		t.Errorf("secret-shaped var must be redacted, got %q", env["FAKE_API_KEY"])
	}
	if env["RANDOM_UNLISTED_VAR"] != "<redacted>" {
		t.Errorf("unlisted var must be redacted (default-deny), got %q", env["RANDOM_UNLISTED_VAR"])
	}
	if v, ok := env["EMPTY_ONE"]; !ok || v != "" {
		t.Errorf("empty var should be present with empty value, got %q ok=%v", v, ok)
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
