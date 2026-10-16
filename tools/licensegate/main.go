// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Command licensegate verifies that bashy's full shipped Go dependency graph
// (which includes yoke, sh, and outpost) plus any vendored or third_party
// directories contains only permissive licenses (MIT, Apache-2.0, BSD, ISC,
// 0BSD, CC0, Unlicense, Zlib).
//
// It fails closed on any unreviewed non-permissive license (GPL, LGPL, AGPL,
// MPL, CPAL, SSPL) or unknown license unless an exact-file reviewed exception
// exists in license-review.json with matching path, SHA-256 digest, and a
// non-empty justification.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// LicenseClass classifies a license as Permissive, NonPermissive, or Unknown.
type LicenseClass int

const (
	LicensePermissive LicenseClass = iota
	LicenseNonPermissive
	LicenseUnknown
)

func (c LicenseClass) String() string {
	switch c {
	case LicensePermissive:
		return "permissive"
	case LicenseNonPermissive:
		return "non-permissive"
	default:
		return "unknown"
	}
}

func (c LicenseClass) MarshalText() ([]byte, error) {
	return []byte(c.String()), nil
}

// ModuleInfo holds module metadata extracted from `go list -deps -json`.
type ModuleInfo struct {
	Path    string
	Version string
	Dir     string
}

// Finding represents a license inspection finding for a specific file.
type Finding struct {
	Module      string       `json:"module"`
	Version     string       `json:"version,omitempty"`
	File        string       `json:"file"`
	Path        string       `json:"path"`
	License     string       `json:"license"`
	Class       LicenseClass `json:"class"`
	SHA256      string       `json:"sha256"`
	Exception   *Exception   `json:"exception,omitempty"`
	IsException bool         `json:"is_exception"`
	Error       string       `json:"error,omitempty"`
}

// Exception represents an exact-file reviewed exception.
type Exception struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Reason string `json:"reason"`
}

// ReviewRegistry holds reviewed exceptions.
type ReviewRegistry struct {
	Exceptions []Exception
}

var (
	licenseFileRegex = regexp.MustCompile(`(?i)^(LICENSE|LICENCE|COPYING)([\.\-].+)?$`)
	spdxRegex        = regexp.MustCompile(`(?i)spdx-license-identifier:\s*([^\r\n]+)`)
)

var permissiveSPDX = map[string]bool{
	"MIT":          true,
	"APACHE-2.0":   true,
	"BSD-2-CLAUSE": true,
	"BSD-3-CLAUSE": true,
	"ISC":          true,
	"0BSD":         true,
	"CC0-1.0":      true,
	"UNLICENSE":    true,
	"ZLIB":         true,
}

var copyleftSPDX = map[string]bool{
	"GPL-1.0":  true,
	"GPL-2.0":  true,
	"GPL-3.0":  true,
	"AGPL-1.0": true,
	"AGPL-3.0": true,
	"LGPL-2.0": true,
	"LGPL-2.1": true,
	"LGPL-3.0": true,
	"MPL-1.1":  true,
	"MPL-2.0":  true,
	"CPAL-1.0": true,
	"SSPL-1.0": true,
}

// ClassifyLicense inspects the license text and returns the detected license identifier
// and license classification.
func ClassifyLicense(content string) (string, LicenseClass) {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return "UNKNOWN", LicenseUnknown
	}

	// 1. Check SPDX-License-Identifier
	if m := spdxRegex.FindStringSubmatch(content); len(m) > 1 {
		tag := strings.Trim(strings.TrimSpace(m[1]), "\"`'*- \t")
		upper := strings.ToUpper(tag)

		if strings.Contains(upper, " OR ") {
			parts := strings.Split(upper, " OR ")
			// If dual-licensed with OR, at least one permissive choice grants permissive rights
			for _, p := range parts {
				p = strings.Trim(strings.TrimSpace(p), "()`'\"* \t")
				if permissiveSPDX[p] {
					return tag, LicensePermissive
				}
			}
		} else if strings.Contains(upper, " AND ") {
			parts := strings.Split(upper, " AND ")
			hasCopyleft := false
			allPermissive := true
			for _, p := range parts {
				p = strings.Trim(strings.TrimSpace(p), "()`'\"* \t")
				if copyleftSPDX[p] {
					hasCopyleft = true
				}
				if !permissiveSPDX[p] {
					allPermissive = false
				}
			}
			if hasCopyleft {
				return tag, LicenseNonPermissive
			}
			if allPermissive {
				return tag, LicensePermissive
			}
		} else {
			clean := strings.Trim(upper, "()`'\"* \t")
			if permissiveSPDX[clean] {
				return tag, LicensePermissive
			}
			if copyleftSPDX[clean] {
				return tag, LicenseNonPermissive
			}
		}
	}

	norm := strings.ToLower(strings.Join(strings.Fields(content), " "))

	// 2. Check copyleft / prohibited licenses
	if strings.Contains(norm, "mozilla public license") {
		if strings.Contains(norm, "version 2.0") || strings.Contains(norm, "v2.0") || strings.Contains(norm, "2.0") {
			return "MPL-2.0", LicenseNonPermissive
		}
		if strings.Contains(norm, "version 1.1") || strings.Contains(norm, "v1.1") {
			return "MPL-1.1", LicenseNonPermissive
		}
		return "MPL", LicenseNonPermissive
	}
	if strings.Contains(norm, "common public attribution license") || strings.Contains(norm, "cpal-1.0") {
		return "CPAL-1.0", LicenseNonPermissive
	}
	if strings.Contains(norm, "server side public license") {
		return "SSPL-1.0", LicenseNonPermissive
	}
	if strings.Contains(norm, "gnu general public license") || strings.Contains(norm, "gnu affero general public license") {
		if strings.Contains(norm, "version 3") || strings.Contains(norm, "v3") {
			return "GPL-3.0", LicenseNonPermissive
		}
		if strings.Contains(norm, "version 2") || strings.Contains(norm, "v2") {
			return "GPL-2.0", LicenseNonPermissive
		}
		return "GPL", LicenseNonPermissive
	}
	if strings.Contains(norm, "gnu lesser general public license") || strings.Contains(norm, "gnu library general public license") {
		if strings.Contains(norm, "version 3") || strings.Contains(norm, "v3") {
			return "LGPL-3.0", LicenseNonPermissive
		}
		if strings.Contains(norm, "version 2.1") || strings.Contains(norm, "v2.1") {
			return "LGPL-2.1", LicenseNonPermissive
		}
		return "LGPL", LicenseNonPermissive
	}

	// 3. Check permissive licenses
	if strings.Contains(norm, "apache license") && (strings.Contains(norm, "version 2.0") || strings.Contains(norm, "v2.0") || strings.Contains(norm, "2.0, january 2004")) {
		return "Apache-2.0", LicensePermissive
	}
	if strings.Contains(norm, "mit license") || (strings.Contains(norm, "permission is hereby granted, free of charge") && strings.Contains(norm, "without restriction")) {
		return "MIT", LicensePermissive
	}
	if strings.Contains(norm, "redistribution and use in source and binary forms, with or without modification, are permitted") {
		if strings.Contains(norm, "neither the name") || strings.Contains(norm, "the names of its contributors may not be used") || strings.Contains(norm, "the name of") {
			return "BSD-3-Clause", LicensePermissive
		}
		return "BSD-2-Clause", LicensePermissive
	}
	if strings.Contains(norm, "zero-clause bsd") || strings.Contains(norm, "0bsd") ||
		(strings.Contains(norm, "permission to use, copy, modify, and/or distribute this software for any purpose with or without fee") &&
			!strings.Contains(norm, "copyright notice and this permission notice appear")) {
		return "0BSD", LicensePermissive
	}
	if strings.Contains(norm, "isc license") || strings.Contains(norm, "permission to use, copy, modify, and distribute this software for any purpose") {
		return "ISC", LicensePermissive
	}
	if strings.Contains(norm, "cc0 1.0 universal") || strings.Contains(norm, "creative commons zero") || strings.Contains(norm, "public domain dedication") {
		return "CC0-1.0", LicensePermissive
	}
	if strings.Contains(norm, "free and unencumbered software released into the public domain") {
		return "Unlicense", LicensePermissive
	}
	if strings.Contains(norm, "this software is provided 'as-is', without any express or implied warranty") && strings.Contains(norm, "in no event will the authors be held liable") {
		return "Zlib", LicensePermissive
	}

	return "UNKNOWN", LicenseUnknown
}

// LoadReviewRegistry loads reviewed exceptions from the specified JSON file.
// Supports multiple schema variations:
// 1. {"exceptions": [{"path": "...", "sha256": "...", "reason": "..."}]}
// 2. [{"path": "...", "sha256": "...", "reason": "..."}]
// 3. {"inputs": {"path": {"sha256": "...", "reason": "..."}}}
// 4. {"path": {"sha256": "...", "reason": "..."}}
func LoadReviewRegistry(path string) (*ReviewRegistry, error) {
	reg := &ReviewRegistry{}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, fmt.Errorf("read review file: %w", err)
	}

	var raw json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse review JSON: %w", err)
	}

	// Try array of exceptions
	var list []Exception
	if err := json.Unmarshal(raw, &list); err == nil {
		reg.Exceptions = list
		return reg, nil
	}

	// Try object with "exceptions" array
	var wrapped struct {
		Exceptions []Exception `json:"exceptions"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Exceptions) > 0 {
		reg.Exceptions = wrapped.Exceptions
		return reg, nil
	}

	// Try object with "inputs" map (Prima / Ollama shape)
	var inputsWrapper struct {
		Inputs map[string]struct {
			SHA256 string `json:"sha256"`
			Reason string `json:"reason"`
			Notice string `json:"notice"`
		} `json:"inputs"`
	}
	if err := json.Unmarshal(raw, &inputsWrapper); err == nil && len(inputsWrapper.Inputs) > 0 {
		for p, val := range inputsWrapper.Inputs {
			reason := val.Reason
			if reason == "" {
				reason = val.Notice
			}
			reg.Exceptions = append(reg.Exceptions, Exception{
				Path:   p,
				SHA256: val.SHA256,
				Reason: reason,
			})
		}
		return reg, nil
	}

	// Try generic map of path -> {sha256, reason}
	var genericMap map[string]struct {
		SHA256 string `json:"sha256"`
		Reason string `json:"reason"`
		Notice string `json:"notice"`
	}
	if err := json.Unmarshal(raw, &genericMap); err == nil {
		for p, val := range genericMap {
			if val.SHA256 == "" {
				continue
			}
			reason := val.Reason
			if reason == "" {
				reason = val.Notice
			}
			reg.Exceptions = append(reg.Exceptions, Exception{
				Path:   p,
				SHA256: val.SHA256,
				Reason: reason,
			})
		}
		return reg, nil
	}

	return reg, nil
}

// MatchException checks if any reviewed exception matches the finding.
// Returns matched exception, whether match occurred, and any verification error (e.g. SHA mismatch).
func (r *ReviewRegistry) MatchException(finding *Finding, repoRoot string) (*Exception, bool, error) {
	if r == nil || len(r.Exceptions) == 0 {
		return nil, false, nil
	}

	findPathNorm := filepath.ToSlash(finding.Path)
	actualSHA := strings.ToLower(finding.SHA256)

	for _, ex := range r.Exceptions {
		exPathNorm := filepath.ToSlash(ex.Path)

		matches := false
		if exPathNorm == findPathNorm {
			matches = true
		} else if strings.HasSuffix(findPathNorm, "/"+exPathNorm) {
			matches = true
		} else if finding.Module != "" {
			modRel := filepath.ToSlash(filepath.Join(finding.Module, finding.File))
			if exPathNorm == modRel {
				matches = true
			}
			if finding.Version != "" {
				modVerRel := filepath.ToSlash(filepath.Join(finding.Module+"@"+finding.Version, finding.File))
				if exPathNorm == modVerRel {
					matches = true
				}
			}
		}

		if !matches && repoRoot != "" {
			if rel, err := filepath.Rel(repoRoot, finding.Path); err == nil {
				if filepath.ToSlash(rel) == exPathNorm {
					matches = true
				}
			}
		}

		if matches {
			expectedSHA := strings.ToLower(strings.TrimSpace(ex.SHA256))
			if expectedSHA != actualSHA {
				return nil, false, fmt.Errorf("tamper detected: exception for %q expected sha256 %s, got %s", ex.Path, expectedSHA, actualSHA)
			}
			if strings.TrimSpace(ex.Reason) == "" {
				return nil, false, fmt.Errorf("invalid exception: exception for %q has empty reason", ex.Path)
			}
			matchCopy := ex
			return &matchCopy, true, nil
		}
	}

	return nil, false, nil
}

// DiscoverShippedModules runs `go list -deps -json` for the given targets and tags,
// returning unique non-standard Go modules.
func DiscoverShippedModules(targets []string, tags string) (map[string]ModuleInfo, error) {
	args := []string{"list", "-deps", "-json"}
	if tags != "" {
		args = append(args, "-tags", tags)
	}
	args = append(args, targets...)

	cmd := exec.Command("go", args...)
	cmd.Env = append(os.Environ(), "GOWORK=off")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("go list failed: %w\nstderr: %s", err, stderr.String())
	}

	dec := json.NewDecoder(&stdout)
	modules := make(map[string]ModuleInfo)

	for {
		var pkg struct {
			ImportPath string `json:"ImportPath"`
			Standard   bool   `json:"Standard"`
			Module     *struct {
				Path    string `json:"Path"`
				Version string `json:"Version"`
				Dir     string `json:"Dir"`
				Replace *struct {
					Path    string `json:"Path"`
					Version string `json:"Version"`
					Dir     string `json:"Dir"`
				} `json:"Replace"`
			} `json:"Module"`
		}

		if err := dec.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode go list JSON: %w", err)
		}

		if pkg.Standard || pkg.Module == nil || pkg.Module.Path == "" {
			continue
		}

		m := pkg.Module
		targetPath := m.Path
		targetVer := m.Version
		targetDir := m.Dir

		if m.Replace != nil {
			if m.Replace.Path != "" {
				targetPath = m.Replace.Path
			}
			if m.Replace.Version != "" {
				targetVer = m.Replace.Version
			}
			if m.Replace.Dir != "" {
				targetDir = m.Replace.Dir
			}
		}

		if _, exists := modules[targetPath]; !exists {
			modules[targetPath] = ModuleInfo{
				Path:    targetPath,
				Version: targetVer,
				Dir:     targetDir,
			}
		}
	}

	return modules, nil
}

// FindLicenseFiles searches a directory for common license file names.
func FindLicenseFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, "-LOGO") || strings.HasSuffix(name, ".png") || strings.HasSuffix(name, ".jpg") {
			continue
		}
		if licenseFileRegex.MatchString(name) {
			matches = append(matches, filepath.Join(dir, name))
		}
	}
	sort.Strings(matches)
	return matches, nil
}

// ScanVendoredDirectories walks repoRoot to locate any `vendor` or `third_party` directories
// and returns all license files found within them.
func ScanVendoredDirectories(repoRoot string) ([]string, error) {
	var licenseFiles []string

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" || name == ".cache" || name == "bin" || name == "dist" {
				return filepath.SkipDir
			}
			if name == "vendor" || name == "third_party" {
				// Walk this vendored subtree for license files
				_ = filepath.WalkDir(path, func(subPath string, subD fs.DirEntry, subErr error) error {
					if subErr != nil || subD.IsDir() {
						return nil
					}
					subName := subD.Name()
					if !strings.HasSuffix(subName, "-LOGO") && !strings.HasSuffix(subName, ".png") && licenseFileRegex.MatchString(subName) {
						licenseFiles = append(licenseFiles, subPath)
					}
					return nil
				})
				return filepath.SkipDir
			}
		}
		return nil
	})

	sort.Strings(licenseFiles)
	return licenseFiles, err
}

// ScanModule inspects a Go module's license files and returns all findings.
func ScanModule(mod ModuleInfo) ([]Finding, error) {
	if mod.Dir == "" {
		return []Finding{{
			Module:  mod.Path,
			Version: mod.Version,
			License: "UNKNOWN",
			Class:   LicenseUnknown,
			Error:   "module directory not resolved on disk",
		}}, nil
	}

	files, err := FindLicenseFiles(mod.Dir)
	if err != nil {
		return nil, fmt.Errorf("read module dir %s: %w", mod.Dir, err)
	}

	if len(files) == 0 {
		return []Finding{{
			Module:  mod.Path,
			Version: mod.Version,
			Path:    mod.Dir,
			License: "UNKNOWN",
			Class:   LicenseUnknown,
			Error:   "no license file found in module root",
		}}, nil
	}

	var findings []Finding
	for _, fpath := range files {
		raw, err := os.ReadFile(fpath)
		if err != nil {
			return nil, fmt.Errorf("read license file %s: %w", fpath, err)
		}

		h := sha256.Sum256(raw)
		shaStr := hex.EncodeToString(h[:])
		lic, class := ClassifyLicense(string(raw))

		findings = append(findings, Finding{
			Module:  mod.Path,
			Version: mod.Version,
			File:    filepath.Base(fpath),
			Path:    fpath,
			License: lic,
			Class:   class,
			SHA256:  shaStr,
		})
	}

	// Also check any internal vendor / third_party directories inside this module
	_ = filepath.WalkDir(mod.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == mod.Dir {
			return nil
		}
		name := d.Name()
		if name == "vendor" || name == "third_party" {
			_ = filepath.WalkDir(path, func(subPath string, subD fs.DirEntry, subErr error) error {
				if subErr != nil || subD.IsDir() {
					return nil
				}
				subName := subD.Name()
				if !strings.HasSuffix(subName, "-LOGO") && !strings.HasSuffix(subName, ".png") && licenseFileRegex.MatchString(subName) {
					raw, err := os.ReadFile(subPath)
					if err == nil {
						h := sha256.Sum256(raw)
						shaStr := hex.EncodeToString(h[:])
						lic, class := ClassifyLicense(string(raw))
						rel, _ := filepath.Rel(mod.Dir, subPath)
						findings = append(findings, Finding{
							Module:  mod.Path,
							Version: mod.Version,
							File:    filepath.ToSlash(rel),
							Path:    subPath,
							License: lic,
							Class:   class,
							SHA256:  shaStr,
						})
					}
				}
				return nil
			})
			return filepath.SkipDir
		}
		return nil
	})

	return findings, nil
}

type GateOptions struct {
	Targets    []string
	Tags       string
	ReviewFile string
	ScanDir    string
	ReportOnly bool
	JSON       bool
	Quiet      bool
}

type GateReport struct {
	TotalModules      int       `json:"total_modules"`
	PermissiveModules int       `json:"permissive_modules"`
	TotalFindings     int       `json:"total_findings"`
	ExceptionsCount   int       `json:"exceptions_count"`
	ViolationsCount   int       `json:"violations_count"`
	Passed            bool      `json:"passed"`
	Violations        []Finding `json:"violations"`
	Exceptions        []Finding `json:"exceptions"`
}

func RunGate(opts GateOptions, stdout, stderr io.Writer) (*GateReport, error) {
	if len(opts.Targets) == 0 {
		opts.Targets = []string{
			"./cmd/bashy",
			"./cmd/bash",
			"./cmd/sh",
			"github.com/qiangli/outpost/cmd/outpost",
		}
	}
	if opts.Tags == "" {
		opts.Tags = "fb_archives"
	}

	repoRoot := opts.ScanDir
	if repoRoot == "" {
		if cwd, err := os.Getwd(); err == nil {
			repoRoot = cwd
		}
	}

	reviewPath := opts.ReviewFile
	if reviewPath == "" {
		cand1 := filepath.Join(repoRoot, "scripts", "license-review.json")
		cand2 := filepath.Join(repoRoot, "license-review.json")
		if _, err := os.Stat(cand1); err == nil {
			reviewPath = cand1
		} else if _, err := os.Stat(cand2); err == nil {
			reviewPath = cand2
		} else {
			reviewPath = cand1
		}
	}

	registry, err := LoadReviewRegistry(reviewPath)
	if err != nil {
		return nil, fmt.Errorf("load review registry: %w", err)
	}

	modules, err := DiscoverShippedModules(opts.Targets, opts.Tags)
	if err != nil {
		return nil, fmt.Errorf("discover shipped modules: %w", err)
	}

	report := &GateReport{
		TotalModules: len(modules),
	}

	var allFindings []Finding
	moduleViolations := make(map[string]bool)

	for _, mod := range modules {
		modFindings, err := ScanModule(mod)
		if err != nil {
			return nil, err
		}
		for _, f := range modFindings {
			allFindings = append(allFindings, f)
			if f.Class != LicensePermissive {
				moduleViolations[mod.Path] = true
			}
		}
	}

	report.PermissiveModules = report.TotalModules - len(moduleViolations)

	// Scan local repo vendored/third_party directories
	vendoredFiles, err := ScanVendoredDirectories(repoRoot)
	if err == nil {
		for _, vf := range vendoredFiles {
			raw, rErr := os.ReadFile(vf)
			if rErr != nil {
				continue
			}
			h := sha256.Sum256(raw)
			shaStr := hex.EncodeToString(h[:])
			lic, class := ClassifyLicense(string(raw))
			rel, _ := filepath.Rel(repoRoot, vf)
			allFindings = append(allFindings, Finding{
				Module:  "vendored",
				File:    filepath.ToSlash(rel),
				Path:    vf,
				License: lic,
				Class:   class,
				SHA256:  shaStr,
			})
		}
	}

	// Filter and evaluate non-permissive or unknown findings against reviewed exceptions
	for i := range allFindings {
		f := &allFindings[i]
		if f.Class == LicensePermissive && f.Error == "" {
			continue
		}

		report.TotalFindings++
		ex, matched, exErr := registry.MatchException(f, repoRoot)
		if exErr != nil {
			f.Error = exErr.Error()
			report.Violations = append(report.Violations, *f)
			continue
		}

		if matched {
			f.IsException = true
			f.Exception = ex
			report.Exceptions = append(report.Exceptions, *f)
		} else {
			report.Violations = append(report.Violations, *f)
		}
	}

	report.ExceptionsCount = len(report.Exceptions)
	report.ViolationsCount = len(report.Violations)
	report.Passed = report.ViolationsCount == 0

	if opts.JSON {
		data, mErr := json.MarshalIndent(report, "", "  ")
		if mErr != nil {
			return nil, fmt.Errorf("marshal JSON report: %w", mErr)
		}
		fmt.Fprintln(stdout, string(data))
		if !report.Passed && !opts.ReportOnly {
			return report, fmt.Errorf("license gate failed with %d violation(s)", report.ViolationsCount)
		}
		return report, nil
	}

	if !opts.Quiet {
		fmt.Fprintln(stdout, "==> Shipped Go Dependency License Gate")
		fmt.Fprintln(stdout, "Targets:")
		for _, t := range opts.Targets {
			fmt.Fprintf(stdout, "  - %s\n", t)
		}
		fmt.Fprintf(stdout, "Build tags: %s\n", opts.Tags)
		fmt.Fprintf(stdout, "Review exceptions file: %s\n", reviewPath)
		fmt.Fprintf(stdout, "Modules scanned: %d (Permissive: %d, Non-permissive/Unknown: %d)\n",
			report.TotalModules, report.PermissiveModules, len(moduleViolations))
		fmt.Fprintf(stdout, "Reviewed exceptions: %d\n", report.ExceptionsCount)

		if report.ExceptionsCount > 0 {
			fmt.Fprintln(stdout, "\nReviewed Exceptions Admitted:")
			for _, ex := range report.Exceptions {
				fmt.Fprintf(stdout, "  [EXEMPT] %s (%s): %s\n    Reason: %s\n    SHA256: %s\n",
					ex.Module, ex.License, ex.File, ex.Exception.Reason, ex.SHA256)
			}
		}

		if report.ViolationsCount > 0 {
			fmt.Fprintf(stdout, "\nFAIL: %d unreviewed non-permissive or unknown license finding(s):\n", report.ViolationsCount)
			for _, v := range report.Violations {
				loc := v.File
				if v.Module != "" {
					loc = fmt.Sprintf("%s: %s", v.Module, v.File)
				}
				desc := v.License
				if v.Error != "" {
					desc = fmt.Sprintf("%s (error: %s)", v.License, v.Error)
				}
				fmt.Fprintf(stdout, "  - %s (%s)\n    Path:   %s\n    SHA256: %s\n",
					loc, desc, v.Path, v.SHA256)
			}
		} else {
			fmt.Fprintf(stdout, "\nPASS: all %d shipped modules and vendored directories permissive or reviewed.\n", report.TotalModules)
		}
	}

	if !report.Passed && !opts.ReportOnly {
		return report, fmt.Errorf("license gate failed with %d unreviewed violation(s)", report.ViolationsCount)
	}

	return report, nil
}

func main() {
	var (
		reviewFile string
		tags       string
		targetsStr string
		scanDir    string
		reportOnly bool
		jsonOutput bool
		quiet      bool
	)

	flag.StringVar(&reviewFile, "review-file", "", "Path to license-review.json")
	flag.StringVar(&tags, "tags", "fb_archives", "Build tags to pass to go list")
	flag.StringVar(&targetsStr, "targets", "", "Comma-separated target list")
	flag.StringVar(&scanDir, "scan-dir", "", "Root directory to scan for vendored/third_party dirs")
	flag.BoolVar(&reportOnly, "report-only", false, "Report findings without failing closed (exit 0)")
	flag.BoolVar(&jsonOutput, "json", false, "Emit JSON report")
	flag.BoolVar(&quiet, "quiet", false, "Emit minimal output")
	flag.Parse()

	var targets []string
	if targetsStr != "" {
		for _, t := range strings.Split(targetsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				targets = append(targets, t)
			}
		}
	}
	if flag.NArg() > 0 {
		targets = append(targets, flag.Args()...)
	}

	opts := GateOptions{
		Targets:    targets,
		Tags:       tags,
		ReviewFile: reviewFile,
		ScanDir:    scanDir,
		ReportOnly: reportOnly,
		JSON:       jsonOutput,
		Quiet:      quiet,
	}

	_, err := RunGate(opts, os.Stdout, os.Stderr)
	if err != nil {
		if !jsonOutput && !quiet {
			fmt.Fprintf(os.Stderr, "licensegate error: %v\n", err)
		}
		os.Exit(1)
	}
}
