// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockVulnRunner struct {
	govulnModule ScannerResult
	govulnBinary ScannerResult
	osvScanner   ScannerResult
	grypeBinary  ScannerResult
}

func (m mockVulnRunner) RunGovulncheckModule(_ context.Context, path string) ScannerResult {
	r := m.govulnModule
	r.Target = path
	return r
}

func (m mockVulnRunner) RunGovulncheckBinary(_ context.Context, path string) ScannerResult {
	r := m.govulnBinary
	r.Target = path
	return r
}

func (m mockVulnRunner) RunOsvScanner(_ context.Context, path string) ScannerResult {
	r := m.osvScanner
	r.Target = path
	return r
}

func (m mockVulnRunner) RunGrype(_ context.Context, path string) ScannerResult {
	r := m.grypeBinary
	r.Target = path
	return r
}

func TestClassifyTarget_GoModule(t *testing.T) {
	dir := t.TempDir()
	goMod := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(goMod, []byte("module testmod\n\ngo 1.27\n"), 0644); err != nil {
		t.Fatal(err)
	}

	target, err := classifyTarget(dir)
	if err != nil {
		t.Fatalf("classifyTarget failed: %v", err)
	}
	if target.Type != TargetGoModule {
		t.Errorf("expected TargetGoModule, got %v", target.Type)
	}
	if target.Path != dir {
		t.Errorf("expected path %s, got %s", dir, target.Path)
	}
}

func TestClassifyTarget_Binary(t *testing.T) {
	dir := t.TempDir()
	binPath := filepath.Join(dir, "fake-binary")
	if err := os.WriteFile(binPath, []byte("\x7fELFfake"), 0755); err != nil {
		t.Fatal(err)
	}

	target, err := classifyTarget(binPath)
	if err != nil {
		t.Fatalf("classifyTarget failed: %v", err)
	}
	if target.Type != TargetBinary {
		t.Errorf("expected TargetBinary, got %v", target.Type)
	}
	if target.Path != binPath {
		t.Errorf("expected path %s, got %s", binPath, target.Path)
	}
}

func TestClassifyTarget_DirectoryWithoutGoMod(t *testing.T) {
	dir := t.TempDir()
	_, err := classifyTarget(dir)
	if err == nil {
		t.Fatal("expected error for directory without go.mod, got nil")
	}
	if !strings.Contains(err.Error(), "no go.mod found") {
		t.Errorf("expected error mentioning no go.mod found, got: %v", err)
	}
}

func TestClassifyTarget_NonExistent(t *testing.T) {
	_, err := classifyTarget("/path/that/does/not/exist/anywhere")
	if err == nil {
		t.Fatal("expected error for non-existent path, got nil")
	}
}

func TestClassifyTargets_Self(t *testing.T) {
	targets, err := classifyTargets(true, nil)
	if err != nil {
		t.Fatalf("classifyTargets with self failed: %v", err)
	}
	if len(targets) != 1 {
		t.Fatalf("expected 1 target, got %d", len(targets))
	}
	if targets[0].Type != TargetBinary {
		t.Errorf("expected self target to be TargetBinary, got %v", targets[0].Type)
	}
	if targets[0].Path == "" {
		t.Error("expected non-empty executable path for self target")
	}
}

func TestClassifyTargets_Multi(t *testing.T) {
	dir := t.TempDir()
	modDir := filepath.Join(dir, "mod")
	if err := os.MkdirAll(modDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module m\n"), 0644); err != nil {
		t.Fatal(err)
	}
	binFile := filepath.Join(dir, "app")
	if err := os.WriteFile(binFile, []byte("bin"), 0755); err != nil {
		t.Fatal(err)
	}

	targets, err := classifyTargets(false, []string{modDir, binFile})
	if err != nil {
		t.Fatalf("classifyTargets multi failed: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(targets))
	}
	if targets[0].Type != TargetGoModule || targets[1].Type != TargetBinary {
		t.Errorf("unexpected target types: %v, %v", targets[0].Type, targets[1].Type)
	}
}

func TestEvaluateVulnReport_AllPass(t *testing.T) {
	runner := mockVulnRunner{
		govulnBinary: ScannerResult{Scanner: "govulncheck", Status: "PASS"},
		grypeBinary:  ScannerResult{Scanner: "grype", Status: "PASS"},
	}

	targets := []Target{{Path: "/tmp/app", Type: TargetBinary}}
	report := evaluateVulnReport(context.Background(), runner, targets)

	if !report.Pass {
		t.Errorf("expected report.Pass to be true, got false")
	}
	if len(report.Scanners) != 2 {
		t.Fatalf("expected 2 scanner results, got %d", len(report.Scanners))
	}
	for _, s := range report.Scanners {
		if s.Status != "PASS" {
			t.Errorf("scanner %s status = %q, want PASS", s.Scanner, s.Status)
		}
	}

	var buf bytes.Buffer
	formatVulnReport(&buf, report, false)
	out := buf.String()
	if !strings.Contains(out, "govulncheck: PASS") || !strings.Contains(out, "grype: PASS") {
		t.Errorf("unexpected format output:\n%s", out)
	}
}

func TestEvaluateVulnReport_SingleFail(t *testing.T) {
	runner := mockVulnRunner{
		govulnBinary: ScannerResult{Scanner: "govulncheck", Status: "FAIL", Findings: 3},
		grypeBinary:  ScannerResult{Scanner: "grype", Status: "PASS"},
	}

	targets := []Target{{Path: "/tmp/app", Type: TargetBinary}}
	report := evaluateVulnReport(context.Background(), runner, targets)

	if report.Pass {
		t.Errorf("expected report.Pass to be false when a scanner fails")
	}

	var buf bytes.Buffer
	formatVulnReport(&buf, report, false)
	out := buf.String()
	if !strings.Contains(out, "govulncheck: FAIL (3 findings)") {
		t.Errorf("missing FAIL (3 findings) in output:\n%s", out)
	}
	if !strings.Contains(out, "grype: PASS") {
		t.Errorf("missing grype PASS in output:\n%s", out)
	}
}

func TestEvaluateVulnReport_SingleFindingPlurality(t *testing.T) {
	runner := mockVulnRunner{
		govulnBinary: ScannerResult{Scanner: "govulncheck", Status: "FAIL", Findings: 1},
		grypeBinary:  ScannerResult{Scanner: "grype", Status: "PASS"},
	}

	targets := []Target{{Path: "/tmp/app", Type: TargetBinary}}
	report := evaluateVulnReport(context.Background(), runner, targets)

	var buf bytes.Buffer
	formatVulnReport(&buf, report, false)
	out := buf.String()
	if !strings.Contains(out, "govulncheck: FAIL (1 finding)") {
		t.Errorf("expected '1 finding' (singular), got:\n%s", out)
	}
}

func TestEvaluateVulnReport_SkippedOfflineNeverReportedAsPass(t *testing.T) {
	runner := mockVulnRunner{
		govulnBinary: ScannerResult{Scanner: "govulncheck", Status: "PASS"},
		grypeBinary: ScannerResult{
			Scanner: "grype",
			Status:  "SKIPPED",
			Reason:  "offline, no cached vulnerability database",
		},
	}

	targets := []Target{{Path: "/tmp/app", Type: TargetBinary}}
	report := evaluateVulnReport(context.Background(), runner, targets)

	// Exit 0 only if no scanner FAILed: SKIPPED is not FAIL, so report.Pass must be true!
	if !report.Pass {
		t.Errorf("expected report.Pass to be true when no scanner failed (one PASS, one SKIPPED)")
	}

	// But a SKIPPED scanner is NEVER reported as PASS:
	if report.Scanners[1].Status == "PASS" {
		t.Fatal("SKIPPED scanner must NEVER be reported as PASS")
	}
	if report.Scanners[1].Status != "SKIPPED" {
		t.Fatalf("expected status SKIPPED, got %q", report.Scanners[1].Status)
	}

	var buf bytes.Buffer
	formatVulnReport(&buf, report, false)
	out := buf.String()
	if !strings.Contains(out, "govulncheck: PASS") {
		t.Errorf("missing govulncheck PASS in output:\n%s", out)
	}
	if !strings.Contains(out, "grype: SKIPPED (offline, no cached vulnerability database)") {
		t.Errorf("missing grype SKIPPED in output:\n%s", out)
	}
}

func TestEvaluateVulnReport_ModuleTree(t *testing.T) {
	runner := mockVulnRunner{
		govulnModule: ScannerResult{Scanner: "govulncheck", Status: "PASS"},
		osvScanner:   ScannerResult{Scanner: "osv-scanner", Status: "FAIL", Findings: 5},
	}

	targets := []Target{{Path: "/tmp/mod", Type: TargetGoModule}}
	report := evaluateVulnReport(context.Background(), runner, targets)

	if report.Pass {
		t.Errorf("expected report.Pass to be false")
	}
	if len(report.Scanners) != 2 {
		t.Fatalf("expected 2 scanner results, got %d", len(report.Scanners))
	}
	if report.Scanners[0].Scanner != "govulncheck" || report.Scanners[0].Status != "PASS" {
		t.Errorf("unexpected scanner 0: %+v", report.Scanners[0])
	}
	if report.Scanners[1].Scanner != "osv-scanner" || report.Scanners[1].Status != "FAIL" || report.Scanners[1].Findings != 5 {
		t.Errorf("unexpected scanner 1: %+v", report.Scanners[1])
	}

	var buf bytes.Buffer
	formatVulnReport(&buf, report, false)
	out := buf.String()
	if !strings.Contains(out, "govulncheck: PASS") || !strings.Contains(out, "osv-scanner: FAIL (5 findings)") {
		t.Errorf("unexpected formatted output:\n%s", out)
	}
}

func TestEvaluateVulnReport_JSONEncoding(t *testing.T) {
	runner := mockVulnRunner{
		govulnBinary: ScannerResult{Scanner: "govulncheck", Status: "PASS"},
		grypeBinary: ScannerResult{
			Scanner: "grype",
			Status:  "SKIPPED",
			Reason:  "offline, no cached vulnerability database",
		},
	}

	targets := []Target{{Path: "/tmp/app", Type: TargetBinary}}
	report := evaluateVulnReport(context.Background(), runner, targets)

	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded VulnReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if !decoded.Pass {
		t.Errorf("decoded.Pass = false, want true")
	}
	if len(decoded.Scanners) != 2 {
		t.Fatalf("decoded.Scanners len = %d, want 2", len(decoded.Scanners))
	}
	if decoded.Scanners[0].Status != "PASS" {
		t.Errorf("decoded.Scanners[0].Status = %q, want PASS", decoded.Scanners[0].Status)
	}
	if decoded.Scanners[1].Status != "SKIPPED" {
		t.Errorf("decoded.Scanners[1].Status = %q, want SKIPPED", decoded.Scanners[1].Status)
	}
	if decoded.Scanners[1].Reason != "offline, no cached vulnerability database" {
		t.Errorf("decoded.Scanners[1].Reason = %q", decoded.Scanners[1].Reason)
	}
}
