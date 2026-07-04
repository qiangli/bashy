// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSuiteRegistryPrecisionLadder pins the load-bearing distinction: the four
// suites are named for the precise claim each earns, and the LICENSE posture
// (public-fetch vs user-supplied) is correct per suite — this is what keeps the
// legal boundary (never auto-fetch a licensed suite) honest.
func TestSuiteRegistryPrecisionLadder(t *testing.T) {
	byName := map[string]suiteSpec{}
	for _, s := range suiteRegistry() {
		byName[s.Name] = s
	}
	for name, wantKind := range map[string]string{
		"compat": "compatibility", "conformance": "conformance",
		"compliance": "certification", "benchmark": "benchmark",
	} {
		s, ok := byName[name]
		if !ok {
			t.Fatalf("missing suite %q", name)
		}
		if s.Kind != wantKind {
			t.Errorf("%s kind = %q, want %q", name, s.Kind, wantKind)
		}
	}
	// Public suites carry a fetch URL and are fetch-at-runtime.
	for _, name := range []string{"compat", "conformance"} {
		s := byName[name]
		if s.License != licensePublicFetch || s.FetchURL == "" {
			t.Errorf("%s should be public-fetch with a URL: %+v", name, s)
		}
	}
	// The official POSIX suite is LICENSED: user-supplied, NO auto-fetch URL, stub.
	c := byName["compliance"]
	if c.License != licenseUserSupplied {
		t.Errorf("compliance must be user-supplied (licensed), got %v", c.License)
	}
	if c.FetchURL != "" {
		t.Errorf("compliance must NOT carry an auto-fetch URL (licensed suite): %q", c.FetchURL)
	}
	if c.Ready {
		t.Error("compliance must be a stub until a licensed suite is wired")
	}
}

func TestVerifyComplianceStubExitCodes(t *testing.T) {
	if code := runVerifyCompliance("/tmp", nil); code != 0 {
		t.Errorf("compliance stub (no args) exit = %d, want 0", code)
	}
	if code := runVerifyCompliance("/tmp", []string{"--suite", "/nonexistent-xyz"}); code != 2 {
		t.Errorf("compliance --suite <missing> exit = %d, want 2", code)
	}
}

func TestEnsureBash53FixturesNoOpWhenPresent(t *testing.T) {
	root := t.TempDir()
	// A pre-existing tests/ dir must short-circuit before any network fetch.
	if err := os.MkdirAll(filepath.Join(root, "external", "bash-5.3", "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	link, err := ensureBash53Fixtures(root)
	if err != nil {
		t.Fatalf("should be a no-op when tests/ present: %v", err)
	}
	if link != filepath.Join(root, "external", "bash-5.3") {
		t.Errorf("unexpected link path %q", link)
	}
}
