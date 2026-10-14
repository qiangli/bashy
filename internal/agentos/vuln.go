// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/yoke/external/govulncheck"
	"github.com/qiangli/yoke/external/grype"
	"github.com/qiangli/yoke/external/osvscanner"
	"github.com/qiangli/yoke/pkg/binmgr"
)

// TargetType classifies the kind of artifact to scan.
type TargetType int

const (
	TargetGoModule TargetType = iota
	TargetBinary
)

func (t TargetType) String() string {
	switch t {
	case TargetGoModule:
		return "module"
	case TargetBinary:
		return "binary"
	default:
		return "unknown"
	}
}

// Target represents an artifact path and its classified type.
type Target struct {
	Path string     `json:"path"`
	Type TargetType `json:"type"`
}

// ScannerResult records the outcome of a single scanner on a target.
type ScannerResult struct {
	Scanner  string `json:"scanner"`
	Target   string `json:"target,omitempty"`
	Status   string `json:"status"` // "PASS", "FAIL", "SKIPPED"
	Findings int    `json:"findings,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// VulnReport aggregates all scanner results for a vuln run.
type VulnReport struct {
	Scanners []ScannerResult `json:"scanners"`
	Pass     bool            `json:"pass"`
}

// VulnScannerRunner abstracts scanner executions so unit tests can inject fake scanners.
type VulnScannerRunner interface {
	RunGovulncheckModule(ctx context.Context, path string) ScannerResult
	RunGovulncheckBinary(ctx context.Context, path string) ScannerResult
	RunOsvScanner(ctx context.Context, path string) ScannerResult
	RunGrype(ctx context.Context, path string) ScannerResult
}

type realVulnRunner struct{}

func resolveGovulncheckExecutable(ctx context.Context) (string, error) {
	if p := govulncheck.CachedBinary(""); p != "" {
		return p, nil
	}
	if base, err := binmgr.CacheDir(); err == nil {
		fallback := filepath.Join(base, "govulncheck", govulncheck.DefaultVersion, binmgr.BinaryName("govulncheck"))
		if binmgr.IsExecutable(fallback) {
			return fallback, nil
		}
	}
	return govulncheck.Ensure(ctx, "")
}

func isNetworkUnavailable(msg string) bool {
	m := strings.ToLower(msg)
	return strings.Contains(m, "dial tcp") ||
		strings.Contains(m, "no such host") ||
		strings.Contains(m, "network is unreachable") ||
		strings.Contains(m, "connection refused") ||
		strings.Contains(m, "i/o timeout")
}

type govulnFindingTraceFrame struct {
	Module   string `json:"module"`
	Package  string `json:"package"`
	Function string `json:"function"`
}

type govulnFindingMsg struct {
	OSV   string                    `json:"osv"`
	Trace []govulnFindingTraceFrame `json:"trace"`
}

type govulnStreamMsg struct {
	Finding *govulnFindingMsg `json:"finding,omitempty"`
}

func parseGovulncheckOutput(stdout []byte, runErr error, stderrStr string) (status string, findings int, reason string) {
	dec := json.NewDecoder(bytes.NewReader(stdout))
	calledOSVs := make(map[string]bool)
	allOSVs := make(map[string]bool)

	for dec.More() {
		var msg govulnStreamMsg
		if err := dec.Decode(&msg); err != nil {
			break
		}
		if msg.Finding != nil {
			allOSVs[msg.Finding.OSV] = true
			isCalled := false
			for _, frame := range msg.Finding.Trace {
				if frame.Function != "" {
					isCalled = true
					break
				}
			}
			if isCalled {
				calledOSVs[msg.Finding.OSV] = true
			}
		}
	}

	if len(calledOSVs) > 0 {
		return "FAIL", len(calledOSVs), ""
	}

	if runErr == nil {
		return "PASS", 0, ""
	}

	if len(allOSVs) > 0 {
		return "FAIL", len(allOSVs), ""
	}

	if binmgr.Offline() && isNetworkUnavailable(stderrStr) {
		return "SKIPPED", 0, "offline: network unavailable"
	}

	errMsg := strings.TrimSpace(stderrStr)
	if errMsg == "" {
		errMsg = runErr.Error()
	}
	return "FAIL", 0, errMsg
}

func (realVulnRunner) RunGovulncheckModule(ctx context.Context, path string) ScannerResult {
	bin, err := resolveGovulncheckExecutable(ctx)
	if err != nil {
		if binmgr.Offline() {
			return ScannerResult{
				Scanner: "govulncheck",
				Target:  path,
				Status:  "SKIPPED",
				Reason:  "offline, binary not cached",
			}
		}
		return ScannerResult{
			Scanner: "govulncheck",
			Target:  path,
			Status:  "FAIL",
			Reason:  err.Error(),
		}
	}

	cmd := binmgr.Command(ctx, bin, "-json", "./...")
	cmd.Dir = path
	cmd.Env = govulncheck.ChildEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	status, findings, reason := parseGovulncheckOutput(stdout.Bytes(), runErr, stderr.String())
	return ScannerResult{
		Scanner:  "govulncheck",
		Target:   path,
		Status:   status,
		Findings: findings,
		Reason:   reason,
	}
}

func (realVulnRunner) RunGovulncheckBinary(ctx context.Context, path string) ScannerResult {
	bin, err := resolveGovulncheckExecutable(ctx)
	if err != nil {
		if binmgr.Offline() {
			return ScannerResult{
				Scanner: "govulncheck",
				Target:  path,
				Status:  "SKIPPED",
				Reason:  "offline, binary not cached",
			}
		}
		return ScannerResult{
			Scanner: "govulncheck",
			Target:  path,
			Status:  "FAIL",
			Reason:  err.Error(),
		}
	}

	cmd := binmgr.Command(ctx, bin, "-mode=binary", "-json", path)
	cmd.Env = govulncheck.ChildEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	status, findings, reason := parseGovulncheckOutput(stdout.Bytes(), runErr, stderr.String())
	return ScannerResult{
		Scanner:  "govulncheck",
		Target:   path,
		Status:   status,
		Findings: findings,
		Reason:   reason,
	}
}

type osvScanOutput struct {
	Results []struct {
		Packages []struct {
			Vulnerabilities []struct {
				ID string `json:"id"`
			} `json:"vulnerabilities"`
		} `json:"packages"`
	} `json:"results"`
}

func (realVulnRunner) RunOsvScanner(ctx context.Context, path string) ScannerResult {
	bin, err := osvscanner.Ensure(ctx, "")
	if err != nil {
		if binmgr.Offline() {
			return ScannerResult{
				Scanner: "osv-scanner",
				Target:  path,
				Status:  "SKIPPED",
				Reason:  "offline, binary not cached",
			}
		}
		return ScannerResult{
			Scanner: "osv-scanner",
			Target:  path,
			Status:  "FAIL",
			Reason:  err.Error(),
		}
	}

	cmd := binmgr.Command(ctx, bin, "scan", "source", "--format", "json", "-r", path)
	cmd.Env = osvscanner.ChildEnv(os.Environ())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	var doc osvScanOutput
	if err := json.Unmarshal(stdout.Bytes(), &doc); err == nil {
		uniqueVulns := make(map[string]bool)
		for _, res := range doc.Results {
			for _, pkg := range res.Packages {
				for _, v := range pkg.Vulnerabilities {
					if v.ID != "" {
						uniqueVulns[v.ID] = true
					}
				}
			}
		}
		if len(uniqueVulns) > 0 {
			return ScannerResult{
				Scanner:  "osv-scanner",
				Target:   path,
				Status:   "FAIL",
				Findings: len(uniqueVulns),
			}
		}
		if runErr == nil {
			return ScannerResult{
				Scanner:  "osv-scanner",
				Target:   path,
				Status:   "PASS",
				Findings: 0,
			}
		}
	}

	if binmgr.Offline() && isNetworkUnavailable(stderr.String()) {
		return ScannerResult{
			Scanner: "osv-scanner",
			Target:  path,
			Status:  "SKIPPED",
			Reason:  "offline: network unavailable",
		}
	}

	errMsg := strings.TrimSpace(stderr.String())
	if errMsg == "" && runErr != nil {
		errMsg = runErr.Error()
	}
	return ScannerResult{
		Scanner: "osv-scanner",
		Target:  path,
		Status:  "FAIL",
		Reason:  errMsg,
	}
}

type grypeScanOutput struct {
	Matches []struct {
		Vulnerability struct {
			ID string `json:"id"`
		} `json:"vulnerability"`
	} `json:"matches"`
}

func (realVulnRunner) RunGrype(ctx context.Context, path string) ScannerResult {
	bin, err := grype.Ensure(ctx, "")
	if err != nil {
		if binmgr.Offline() {
			return ScannerResult{
				Scanner: "grype",
				Target:  path,
				Status:  "SKIPPED",
				Reason:  "offline, binary not cached",
			}
		}
		return ScannerResult{
			Scanner: "grype",
			Target:  path,
			Status:  "FAIL",
			Reason:  err.Error(),
		}
	}

	dbDir, err := grype.EnsureDB(ctx, bin)
	if err != nil {
		if errors.Is(err, grype.ErrOfflineNoDB) {
			return ScannerResult{
				Scanner: "grype",
				Target:  path,
				Status:  "SKIPPED",
				Reason:  "offline, no cached vulnerability database",
			}
		}
		return ScannerResult{
			Scanner: "grype",
			Target:  path,
			Status:  "FAIL",
			Reason:  err.Error(),
		}
	}

	cmd := binmgr.Command(ctx, bin, "-o", "json", path)
	cmd.Env = grype.DBEnv(dbDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	var doc grypeScanOutput
	if err := json.Unmarshal(stdout.Bytes(), &doc); err == nil {
		if len(doc.Matches) > 0 {
			return ScannerResult{
				Scanner:  "grype",
				Target:   path,
				Status:   "FAIL",
				Findings: len(doc.Matches),
			}
		}
		if runErr == nil {
			return ScannerResult{
				Scanner:  "grype",
				Target:   path,
				Status:   "PASS",
				Findings: 0,
			}
		}
	}

	errMsg := strings.TrimSpace(stderr.String())
	if errMsg == "" && runErr != nil {
		errMsg = runErr.Error()
	}
	return ScannerResult{
		Scanner: "grype",
		Target:  path,
		Status:  "FAIL",
		Reason:  errMsg,
	}
}

func classifyTarget(p string) (Target, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return Target{}, fmt.Errorf("stat %s: %w", p, err)
	}
	if fi.IsDir() {
		goMod := filepath.Join(p, "go.mod")
		if _, err := os.Stat(goMod); err != nil {
			return Target{}, fmt.Errorf("%s: directory is not a Go module (no go.mod found)", p)
		}
		return Target{Path: p, Type: TargetGoModule}, nil
	}
	return Target{Path: p, Type: TargetBinary}, nil
}

func selfTarget() (Target, error) {
	exe, err := os.Executable()
	if err != nil {
		return Target{}, fmt.Errorf("resolve self executable: %w", err)
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return Target{Path: exe, Type: TargetBinary}, nil
}

func classifyTargets(self bool, paths []string) ([]Target, error) {
	var targets []Target
	if self {
		t, err := selfTarget()
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	for _, p := range paths {
		t, err := classifyTarget(p)
		if err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		if _, err := os.Stat("go.mod"); err == nil {
			targets = append(targets, Target{Path: ".", Type: TargetGoModule})
		} else {
			return nil, errors.New("specify a target path or --self")
		}
	}
	return targets, nil
}

func evaluateVulnReport(ctx context.Context, runner VulnScannerRunner, targets []Target) VulnReport {
	var report VulnReport
	report.Pass = true

	for _, target := range targets {
		switch target.Type {
		case TargetGoModule:
			rGovuln := runner.RunGovulncheckModule(ctx, target.Path)
			rOsv := runner.RunOsvScanner(ctx, target.Path)
			report.Scanners = append(report.Scanners, rGovuln, rOsv)
		case TargetBinary:
			rGovuln := runner.RunGovulncheckBinary(ctx, target.Path)
			rGrype := runner.RunGrype(ctx, target.Path)
			report.Scanners = append(report.Scanners, rGovuln, rGrype)
		}
	}

	for _, s := range report.Scanners {
		if s.Status == "FAIL" {
			report.Pass = false
			break
		}
	}
	return report
}

func formatVulnReport(w io.Writer, report VulnReport, multiTarget bool) {
	for _, res := range report.Scanners {
		prefix := res.Scanner
		if multiTarget && res.Target != "" {
			prefix = fmt.Sprintf("%s (%s)", res.Scanner, res.Target)
		}
		var line string
		switch res.Status {
		case "PASS":
			line = fmt.Sprintf("%s: PASS", prefix)
		case "FAIL":
			findingStr := fmt.Sprintf("%d findings", res.Findings)
			if res.Findings == 1 {
				findingStr = "1 finding"
			}
			if res.Findings == 0 && res.Reason != "" {
				findingStr = res.Reason
			}
			line = fmt.Sprintf("%s: FAIL (%s)", prefix, findingStr)
		case "SKIPPED":
			reason := res.Reason
			reason = strings.TrimPrefix(reason, "skipped: ")
			line = fmt.Sprintf("%s: SKIPPED (%s)", prefix, reason)
		default:
			line = fmt.Sprintf("%s: %s", prefix, res.Status)
		}
		fmt.Fprintln(w, line)
	}
}

func dispatchVuln(args []string) int {
	var self bool
	var asJSON bool
	var paths []string

	for _, a := range args {
		switch {
		case a == "--self":
			self = true
		case a == "--json" || a == "--json=true":
			asJSON = true
		case a == "--json=false":
			asJSON = false
		case a == "-h" || a == "--help":
			fmt.Println("usage: vuln [--self] [--json] [PATH...]")
			fmt.Println("Check for known vulnerabilities using pinned govulncheck, osv-scanner, and grype.")
			fmt.Println("")
			fmt.Println("Flags:")
			fmt.Println("  --self   scan the running bashy executable as a binary")
			fmt.Println("  --json   emit results as JSON")
			fmt.Println("  -h, --help  show help")
			return 0
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(os.Stderr, "vuln: unknown option %q\n", a)
			return 2
		default:
			paths = append(paths, a)
		}
	}

	targets, err := classifyTargets(self, paths)
	if err != nil {
		fmt.Fprintf(os.Stderr, "vuln: %v\n", err)
		return 2
	}

	report := evaluateVulnReport(context.Background(), realVulnRunner{}, targets)

	if asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(report); err != nil {
			fmt.Fprintf(os.Stderr, "vuln: encode json: %v\n", err)
			return 1
		}
	} else {
		formatVulnReport(os.Stdout, report, len(targets) > 1)
	}

	if !report.Pass {
		return 1
	}
	return 0
}
