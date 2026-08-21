// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
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
	link, err := EnsureBash53Fixtures(root)
	if err != nil {
		t.Fatalf("should be a no-op when tests/ present: %v", err)
	}
	if link != filepath.Join(root, "external", "bash-5.3") {
		t.Errorf("unexpected link path %q", link)
	}
}

func TestFetchBash53TestsVerifiesArchiveAndExtractsRequiredTrees(t *testing.T) {
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for name, body := range map[string]string{
		"bash-5.3/tests/run-all":   "fixture",
		"bash-5.3/support/recho.c": "helper",
		"bash-5.3/COPYING":         "not extracted",
	} {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(archive.Bytes())
	want := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive.Bytes())
	}))
	defer srv.Close()

	dst := t.TempDir()
	if err := fetchBash53Tests(srv.URL, dst, want); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"tests/run-all", "support/recho.c"} {
		if _, err := os.Stat(filepath.Join(dst, rel)); err != nil {
			t.Fatalf("required fixture %s: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dst, "COPYING")); !os.IsNotExist(err) {
		t.Fatalf("unexpected non-fixture extraction: %v", err)
	}
	if err := fetchBash53Tests(srv.URL, t.TempDir(), "00"); err == nil {
		t.Fatal("checksum mismatch accepted")
	}
}
