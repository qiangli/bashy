// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build e2e

// End-to-end for `bashy release`: drive the REAL binary the way an operator or
// an agent does, and confirm one invocation lands the archives, the checksum
// manifest and the bashy-release-v1 ledger on disk.
//
//	go test -tags e2e -run TestE2ERelease ./internal/agentos/
package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const e2eReleaseConfig = `
project_name: demo
builds:
  - id: demo
    main: ./cmd/demo
    binary: demo
    targets: [linux_amd64, darwin_arm64]
archives:
  - id: demo
    formats: [tar.gz]
    name_template: '{{ .ProjectName }}-{{ .Os }}-{{ .Arch }}'
checksum:
  name_template: SHA256SUMS
`

// TestE2EReleaseSnapshot packages already-built binaries (--skip-build), so the
// gate measures the release path itself on every OS rather than a cross-compile
// matrix. The naming is the one the fleet-upgrade contract consumes BY NAME.
func TestE2EReleaseSnapshot(t *testing.T) {
	bin := bashyBinary(t)

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".goreleaser.yaml"), []byte(e2eReleaseConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"dist/demo_linux_amd64/demo", "dist/demo_darwin_arm64/demo"} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("binary bytes for "+rel+"\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	stdout, stderr, code := runBashyStd(bin, "release", "--snapshot",
		"--dir", dir, "--version", "0.1.0", "--skip-build", "--json")
	if code != 0 {
		t.Fatalf("`bashy release --snapshot` exited %d:\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if s := unsupportedSignal(stdout + stderr); s != "" {
		t.Fatalf("release did not dispatch (%q): %s", s, firstLineOf(stdout+stderr))
	}

	var ledger struct {
		Schema    string `json:"schema_version"`
		Version   string `json:"version"`
		Snapshot  bool   `json:"snapshot"`
		Artifacts []struct {
			Name   string `json:"name"`
			Type   string `json:"type"`
			SHA256 string `json:"sha256"`
			Size   int64  `json:"size"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout), &ledger); err != nil {
		t.Fatalf("stdout is not a %s ledger: %v\n%s", "bashy-release-v1", err, stdout)
	}
	if ledger.Schema != "bashy-release-v1" || ledger.Version != "0.1.0" || !ledger.Snapshot {
		t.Errorf("ledger = %+v", ledger)
	}

	names := map[string]bool{}
	for _, a := range ledger.Artifacts {
		names[a.Name] = true
		if a.SHA256 == "" || a.Size == 0 {
			t.Errorf("artifact %s has no digest/size", a.Name)
		}
		if _, err := os.Stat(filepath.Join(dir, "dist", a.Name)); err != nil {
			t.Errorf("ledger names %s but it is not on disk: %v", a.Name, err)
		}
	}
	for _, want := range []string{"demo-linux-amd64.tar.gz", "demo-darwin-arm64.tar.gz", "SHA256SUMS"} {
		if !names[want] {
			t.Errorf("ledger is missing %s (got %v)", want, names)
		}
	}

	// The ledger is also written into the output directory, so a later stage
	// (smoke, publish, the fleet envelope) can read it without re-running.
	b, err := os.ReadFile(filepath.Join(dir, "dist", "release-ledger.json"))
	if err != nil {
		t.Fatalf("ledger file: %v", err)
	}
	if !strings.Contains(string(b), "bashy-release-v1") {
		t.Errorf("ledger file is not the schema-tagged ledger:\n%s", b)
	}
}

// A config declaring an unimplemented stage must fail the real binary loudly,
// naming the stage — not produce a partial set of assets and exit 0.
func TestE2EReleaseRefusesUnimplementedStage(t *testing.T) {
	bin := bashyBinary(t)
	dir := t.TempDir()
	cfg := e2eReleaseConfig + "\nannounce:\n  slack:\n    enabled: true\n"
	if err := os.WriteFile(filepath.Join(dir, ".goreleaser.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runBashyStd(bin, "release", "--snapshot", "--dir", dir, "--version", "0.1.0", "--skip-build")
	if code == 0 {
		t.Fatalf("a config with announce: must fail:\n%s", stdout)
	}
	if !strings.Contains(stderr, "announce") {
		t.Errorf("stderr must name the refused stage:\n%s", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist")); err == nil {
		t.Error("a refused config must not have produced an output directory")
	}
}
