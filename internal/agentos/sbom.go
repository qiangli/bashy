// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	outgit "github.com/qiangli/yoke/git"
)

// LicenseClass classifies a module's license.
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

func (c *LicenseClass) UnmarshalText(text []byte) error {
	switch string(text) {
	case "permissive":
		*c = LicensePermissive
	case "non-permissive":
		*c = LicenseNonPermissive
	default:
		*c = LicenseUnknown
	}
	return nil
}

// SPDX 2.3 schema models

type SPDXDocument struct {
	SPDXVersion       string             `json:"spdxVersion"`
	DataLicense       string             `json:"dataLicense"`
	SPDXID            string             `json:"SPDXID"`
	Name              string             `json:"name"`
	DocumentNamespace string             `json:"documentNamespace"`
	CreationInfo      SPDXCreationInfo   `json:"creationInfo"`
	Packages          []SPDXPackage      `json:"packages"`
	Relationships     []SPDXRelationship `json:"relationships"`
	Comment           string             `json:"comment,omitempty"`
}

type SPDXCreationInfo struct {
	Created  string   `json:"created"`
	Creators []string `json:"creators"`
	Comment  string   `json:"comment,omitempty"`
}

type SPDXPackage struct {
	SPDXID                string            `json:"SPDXID"`
	Name                  string            `json:"name"`
	VersionInfo           string            `json:"versionInfo"`
	DownloadLocation      string            `json:"downloadLocation"`
	FilesAnalyzed         bool              `json:"filesAnalyzed"`
	LicenseConcluded      string            `json:"licenseConcluded"`
	LicenseDeclared       string            `json:"licenseDeclared"`
	LicenseClassification LicenseClass      `json:"licenseClassification,omitempty"`
	Checksums             []SPDXChecksum    `json:"checksums,omitempty"`
	ExternalRefs          []SPDXExternalRef `json:"externalRefs,omitempty"`
	Comment               string            `json:"comment,omitempty"`
}

type SPDXChecksum struct {
	Algorithm     string `json:"algorithm"`
	ChecksumValue string `json:"checksumValue"`
}

type SPDXExternalRef struct {
	ReferenceCategory string `json:"referenceCategory"`
	ReferenceType     string `json:"referenceType"`
	ReferenceLocator  string `json:"referenceLocator"`
}

type SPDXRelationship struct {
	SPDXElementID      string `json:"spdxElementId"`
	RelationshipType   string `json:"relationshipType"`
	RelatedSPDXElement string `json:"relatedSpdxElement"`
	Comment            string `json:"comment,omitempty"`
}

// SBOMOptions configures SBOM generation.
type SBOMOptions struct {
	TargetOS   string
	TargetArch string
	Commit     string
	BuildTags  string
	Gate       bool
}

// ClassifyLicense inspects license text and returns the normalized SPDX identifier
// and license classification.
func ClassifyLicense(content string) (string, LicenseClass) {
	if strings.TrimSpace(content) == "" {
		return "UNKNOWN", LicenseUnknown
	}

	// Check SPDX-License-Identifier header
	spdxRe := regexp.MustCompile(`(?i)spdx-license-identifier:\s*([a-zA-Z0-9\.\-]+)`)
	if m := spdxRe.FindStringSubmatch(content); len(m) > 1 {
		spdx := m[1]
		upper := strings.ToUpper(spdx)
		switch upper {
		case "MIT", "APACHE-2.0", "BSD-2-CLAUSE", "BSD-3-CLAUSE", "ISC", "0BSD", "CC0-1.0", "UNLICENSE", "ZLIB":
			return spdx, LicensePermissive
		case "GPL-1.0", "GPL-2.0", "GPL-3.0", "AGPL-1.0", "AGPL-3.0", "LGPL-2.0", "LGPL-2.1", "LGPL-3.0", "MPL-1.1", "MPL-2.0", "SSPL-1.0":
			return spdx, LicenseNonPermissive
		}
	}

	norm := strings.ToLower(strings.Join(strings.Fields(content), " "))

	// Check non-permissive (copyleft) licenses first
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
	if strings.Contains(norm, "mozilla public license") && (strings.Contains(norm, "2.0") || strings.Contains(norm, "v. 2.0") || strings.Contains(norm, "v2.0")) {
		return "MPL-2.0", LicenseNonPermissive
	}
	if strings.Contains(norm, "server side public license") {
		return "SSPL-1.0", LicenseNonPermissive
	}

	// Permissive licenses
	if strings.Contains(norm, "apache license") && (strings.Contains(norm, "version 2.0") || strings.Contains(norm, "v2.0") || strings.Contains(norm, "2.0, january 2004")) {
		return "Apache-2.0", LicensePermissive
	}
	if strings.Contains(norm, "mit license") || (strings.Contains(norm, "permission is hereby granted, free of charge, to any person obtaining a copy") && strings.Contains(norm, "without restriction")) {
		return "MIT", LicensePermissive
	}
	if strings.Contains(norm, "redistribution and use in source and binary forms, with or without modification, are permitted") {
		if strings.Contains(norm, "neither the name") || strings.Contains(norm, "the names of its contributors may not be used") || strings.Contains(norm, "the name of") {
			return "BSD-3-Clause", LicensePermissive
		}
		return "BSD-2-Clause", LicensePermissive
	}
	if strings.Contains(norm, "zero-clause bsd") || strings.Contains(norm, "0bsd") ||
		(strings.Contains(norm, "permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted") &&
			!strings.Contains(norm, "copyright notice and this permission notice appear")) {
		return "0BSD", LicensePermissive
	}
	if strings.Contains(norm, "isc license") || strings.Contains(norm, "permission to use, copy, modify, and distribute this software for any purpose") || strings.Contains(norm, "permission to use, copy, modify, and/or distribute this software for any purpose") {
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

var firstPartyLicenses = map[string]string{
	"github.com/qiangli/bashy":                "MIT",
	"github.com/bashsharp/bashsharp":          "BSD-3-Clause",
	"github.com/qiangli/coreutils":            "MIT",
	"github.com/qiangli/yoke":                 "MIT",
	"github.com/qiangli/yoke/pkg/llmgw":       "MIT",
	"github.com/qiangli/ycode":                "MIT",
	"github.com/qiangli/ycode/examples/genie": "MIT",
	"mvdan.cc/sh/v3":                          "BSD-3-Clause",
	"github.com/qiangli/outpost":              "MIT",
	"github.com/ergochat/readline":            "MIT",
	"github.com/filebrowser/filebrowser/v2":   "Apache-2.0",
	"github.com/odvcencio/gotreesitter":       "MIT",
	"github.com/benhoyt/goawk":                "MIT",
	"github.com/qiangli/gfy":                  "MIT",
	"github.com/dhnt/dhnt":                    "Apache-2.0",
}

// FindLicense searches a directory for common license file names and returns
// the file path and content.
func FindLicense(dir string) (string, []byte) {
	if dir == "" {
		return "", nil
	}
	names := []string{
		"LICENSE", "LICENSE.txt", "LICENSE.md", "LICENCE", "LICENCE.txt", "LICENCE.md",
		"LICENSE.MIT", "LICENSE.BSD", "LICENSE.APACHE", "COPYING", "COPYING.txt", "COPYING.md",
	}
	for _, n := range names {
		p := filepath.Join(dir, n)
		if data, err := os.ReadFile(p); err == nil && len(data) > 0 {
			return p, data
		}
	}
	return "", nil
}

func encodeModPath(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// findModuleLicenseDir attempts to locate the directory holding the module's files.
func findModuleLicenseDir(modPath, modVer string, localRoots []string) string {
	gomodcache := os.Getenv("GOMODCACHE")
	if gomodcache == "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			if home, err := os.UserHomeDir(); err == nil {
				gopath = filepath.Join(home, "go")
			}
		}
		if gopath != "" {
			gomodcache = filepath.Join(gopath, "pkg", "mod")
		}
	}

	// 1. Try Go module cache
	if gomodcache != "" && modVer != "" && modVer != "(devel)" {
		enc := encodeModPath(modPath)
		dir := filepath.Join(gomodcache, enc+"@"+modVer)
		if fi, err := os.Stat(dir); err == nil && fi.IsDir() {
			return dir
		}
	}

	// 2. Try local roots (umbrella root, sibling submodules)
	for _, root := range localRoots {
		if root == "" {
			continue
		}
		var candidate string
		switch {
		case modPath == "mvdan.cc/sh/v3":
			candidate = filepath.Join(root, "sh")
		case modPath == "github.com/benhoyt/goawk":
			candidate = filepath.Join(root, "coreutils", "third_party", "goawk")
		case modPath == "github.com/ergochat/readline":
			candidate = filepath.Join(root, "readline")
		case modPath == "github.com/filebrowser/filebrowser/v2":
			candidate = filepath.Join(root, "filebrowser")
		case modPath == "github.com/odvcencio/gotreesitter":
			candidate = filepath.Join(root, "gotreesitter")
		case strings.Contains(modPath, "ycode/examples/genie"):
			candidate = filepath.Join(root, "ycode")
		case strings.Contains(modPath, "yoke/pkg/llmgw"):
			candidate = filepath.Join(root, "yoke")
		case strings.HasPrefix(modPath, "github.com/qiangli/"):
			sub := strings.TrimPrefix(modPath, "github.com/qiangli/")
			candidate = filepath.Join(root, sub)
		case strings.HasPrefix(modPath, "github.com/bashsharp/"):
			sub := strings.TrimPrefix(modPath, "github.com/bashsharp/")
			candidate = filepath.Join(root, sub)
		case strings.HasPrefix(modPath, "github.com/dhnt/"):
			sub := strings.TrimPrefix(modPath, "github.com/dhnt/")
			candidate = filepath.Join(root, sub)
		default:
			base := filepath.Base(modPath)
			candidate = filepath.Join(root, base)
		}
		if fi, err := os.Stat(candidate); err == nil && fi.IsDir() {
			return candidate
		}
	}

	return ""
}

// sanitizeSPDXID creates a valid SPDX ID from a module or package name.
func sanitizeSPDXID(name string) string {
	var b strings.Builder
	b.WriteString("SPDXRef-Package-")
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
			b.WriteRune(r)
		} else {
			b.WriteRune('-')
		}
	}
	return b.String()
}

// loadGoModPins reads go.mod from the current or specified directory to resolve
// pinned versions for first-party or replaced dependencies.
func loadGoModPins(dir string) map[string]string {
	pins := make(map[string]string)
	modPath := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(modPath)
	if err != nil {
		// Try parent
		modPath = filepath.Join(dir, "..", "go.mod")
		data, err = os.ReadFile(modPath)
		if err != nil {
			return pins
		}
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "require ") || strings.HasPrefix(line, "\t") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				pkg := parts[0]
				if pkg == "require" && len(parts) >= 3 {
					pkg = parts[1]
					pins[pkg] = parts[2]
				} else if len(parts) >= 2 && !strings.HasPrefix(pkg, "//") {
					pins[pkg] = parts[1]
				}
			}
		}
		if strings.HasPrefix(line, "replace ") {
			parts := strings.Fields(line)
			// e.g. replace A => B v1.2.3
			for i, p := range parts {
				if p == "=>" && i > 1 && i+2 < len(parts) {
					orig := parts[1]
					repVer := parts[i+2]
					pins[orig] = repVer
				}
			}
		}
	}
	return pins
}

// GenerateSBOM extracts Go build info from the specified binary and writes an SPDX 2.3 document.
func GenerateSBOM(binaryPath string, opts SBOMOptions) (*SPDXDocument, error) {
	f, err := os.Open(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("open binary: %w", err)
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return nil, fmt.Errorf("hash binary: %w", err)
	}
	binaryDigest := hex.EncodeToString(hasher.Sum(nil))

	bi, err := buildinfo.ReadFile(binaryPath)
	if err != nil {
		return nil, fmt.Errorf("read Go buildinfo: %w", err)
	}

	// Extract build settings
	settings := make(map[string]string)
	for _, s := range bi.Settings {
		settings[s.Key] = s.Value
	}

	targetOS := opts.TargetOS
	if targetOS == "" {
		targetOS = settings["GOOS"]
	}
	targetArch := opts.TargetArch
	if targetArch == "" {
		targetArch = settings["GOARCH"]
	}

	commit := opts.Commit
	if commit == "" {
		commit = settings["vcs.revision"]
	}
	if commit == "" {
		// Fallback to git through the one door (sprint 252 S252.6):
		// bare rev-parse stays on the host binary; failure keeps the
		// old silent-empty shape.
		if out, err := outgit.RunChecked(context.Background(), "", []string{"rev-parse", "HEAD"}); err == nil {
			commit = strings.TrimSpace(out)
		}
	}

	buildTags := opts.BuildTags
	if buildTags == "" {
		buildTags = settings["-tags"]
	}

	createdTime := settings["vcs.time"]
	if createdTime == "" {
		createdTime = time.Now().UTC().Format(time.RFC3339)
	}

	// Local roots for discovering first-party and sibling module licenses
	cwd, _ := os.Getwd()
	var localRoots []string
	addRoot := func(p string) {
		if abs, err := filepath.Abs(p); err == nil {
			localRoots = append(localRoots, abs)
		}
	}
	for curr := cwd; curr != "" && curr != filepath.Dir(curr); curr = filepath.Dir(curr) {
		addRoot(curr)
		if _, err := os.Stat(filepath.Join(curr, "go.work")); err == nil {
			addRoot(curr)
			break
		}
	}
	if binDir := filepath.Dir(binaryPath); binDir != "" {
		for curr := binDir; curr != "" && curr != filepath.Dir(curr); curr = filepath.Dir(curr) {
			addRoot(curr)
			if _, err := os.Stat(filepath.Join(curr, "go.work")); err == nil {
				addRoot(curr)
				break
			}
		}
	}

	goModPins := loadGoModPins(cwd)

	docName := fmt.Sprintf("bashy-%s-%s", targetOS, targetArch)
	doc := &SPDXDocument{
		SPDXVersion:       "SPDX-2.3",
		DataLicense:       "CC0-1.0",
		SPDXID:            "SPDXRef-DOCUMENT",
		Name:              docName,
		DocumentNamespace: fmt.Sprintf("https://github.com/qiangli/bashy/spdx/%s-%s", docName, binaryDigest[:16]),
		CreationInfo: SPDXCreationInfo{
			Created:  createdTime,
			Creators: []string{"Tool: bashy-release-sbom-1.0", "Organization: bashy"},
			Comment:  fmt.Sprintf("target: %s/%s; commit: %s; goVersion: %s; tags: %s; digest: %s", targetOS, targetArch, commit, bi.GoVersion, buildTags, binaryDigest),
		},
	}

	rootPkgID := "SPDXRef-Package-bashy"
	rootPkg := SPDXPackage{
		SPDXID:                rootPkgID,
		Name:                  "bashy",
		VersionInfo:           bi.Main.Version,
		DownloadLocation:      "NOASSERTION",
		FilesAnalyzed:         false,
		LicenseConcluded:      "MIT",
		LicenseDeclared:       "MIT",
		LicenseClassification: LicensePermissive,
		Checksums: []SPDXChecksum{
			{Algorithm: "SHA256", ChecksumValue: binaryDigest},
		},
		ExternalRefs: []SPDXExternalRef{
			{
				ReferenceCategory: "PACKAGE-MANAGER",
				ReferenceType:     "purl",
				ReferenceLocator:  "pkg:golang/github.com/qiangli/bashy@" + bi.Main.Version,
			},
		},
		Comment: fmt.Sprintf("commit=%s; goVersion=%s; tags=%s; os=%s; arch=%s", commit, bi.GoVersion, buildTags, targetOS, targetArch),
	}
	doc.Packages = append(doc.Packages, rootPkg)
	doc.Relationships = append(doc.Relationships, SPDXRelationship{
		SPDXElementID:      "SPDXRef-DOCUMENT",
		RelationshipType:   "DESCRIBES",
		RelatedSPDXElement: rootPkgID,
	})

	// Process dependencies
	seen := make(map[string]bool)
	seen["github.com/qiangli/bashy"] = true

	// Also ensure first-party modules from bi.Main or local modules are accounted
	deps := append([]*debug.Module(nil), bi.Deps...)

	for _, dep := range deps {
		targetDep := dep
		if dep.Replace != nil {
			targetDep = dep.Replace
		}
		if seen[targetDep.Path] {
			continue
		}
		seen[targetDep.Path] = true

		ver := targetDep.Version
		if ver == "" || ver == "(devel)" {
			if pin, ok := goModPins[targetDep.Path]; ok {
				ver = pin
			} else if commit != "" && strings.HasPrefix(targetDep.Path, "github.com/qiangli/") {
				ver = commit
			}
		}

		// Locate license
		dir := findModuleLicenseDir(targetDep.Path, targetDep.Version, localRoots)
		_, licBytes := FindLicense(dir)

		licID, licClass := ClassifyLicense(string(licBytes))
		if licClass == LicenseUnknown {
			if fpLic, ok := firstPartyLicenses[targetDep.Path]; ok {
				licID = fpLic
				licClass = LicensePermissive
			}
		}

		pkgID := sanitizeSPDXID(targetDep.Path)
		spdxPkg := SPDXPackage{
			SPDXID:                pkgID,
			Name:                  targetDep.Path,
			VersionInfo:           ver,
			DownloadLocation:      "NOASSERTION",
			FilesAnalyzed:         false,
			LicenseConcluded:      licID,
			LicenseDeclared:       licID,
			LicenseClassification: licClass,
			ExternalRefs: []SPDXExternalRef{
				{
					ReferenceCategory: "PACKAGE-MANAGER",
					ReferenceType:     "purl",
					ReferenceLocator:  fmt.Sprintf("pkg:golang/%s@%s", targetDep.Path, ver),
				},
			},
		}

		if targetDep.Sum != "" {
			spdxPkg.Checksums = append(spdxPkg.Checksums, SPDXChecksum{
				Algorithm:     "SHA256",
				ChecksumValue: targetDep.Sum,
			})
		}

		doc.Packages = append(doc.Packages, spdxPkg)
		doc.Relationships = append(doc.Relationships, SPDXRelationship{
			SPDXElementID:      rootPkgID,
			RelationshipType:   "DEPENDS_ON",
			RelatedSPDXElement: pkgID,
		})
	}

	// Sort packages deterministically (root package first, then by name)
	if len(doc.Packages) > 1 {
		sort.Slice(doc.Packages[1:], func(i, j int) bool {
			return doc.Packages[1+i].Name < doc.Packages[1+j].Name
		})
	}
	sort.Slice(doc.Relationships[1:], func(i, j int) bool {
		return doc.Relationships[1+i].RelatedSPDXElement < doc.Relationships[1+j].RelatedSPDXElement
	})

	if opts.Gate {
		if err := ValidateLicenseGate(doc); err != nil {
			return doc, err
		}
	}

	return doc, nil
}

// ValidateLicenseGate fails closed if any module in the document has a non-permissive
// or unknown license.
func ValidateLicenseGate(doc *SPDXDocument) error {
	var violations []string
	for _, pkg := range doc.Packages {
		if pkg.LicenseClassification != LicensePermissive {
			violations = append(violations, fmt.Sprintf("%s: %s (license: %s)", pkg.Name, pkg.LicenseClassification, pkg.LicenseConcluded))
		}
	}
	if len(violations) > 0 {
		return fmt.Errorf("license gate failed with %d non-permissive or unknown module(s):\n  - %s",
			len(violations), strings.Join(violations, "\n  - "))
	}
	return nil
}

func releaseSbomCmd() *cobra.Command {
	var (
		binaryPath string
		distDir    string
		outputPath string
		targetOS   string
		targetArch string
		commit     string
		buildTags  string
		gate       bool
		checkOnly  bool
	)

	cmd := &cobra.Command{
		Use:   "sbom [flags] [binary]",
		Short: "Generate an SPDX 2.3 JSON SBOM from Go buildinfo and enforce permissive licenses",
		Long: `bashy release sbom writes an SPDX 2.3 JSON SBOM per release OS/arch from the
binary's Go build info (go version -m) and embedded first-party modules.

It classifies every module's license (permissive / non-permissive / unknown) using
its LICENSE file and gates execution: failing closed on any non-permissive or
unknown entry.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				binaryPath = args[0]
			}
			if binaryPath == "" && distDir == "" {
				// Default to looking for bin/bashy, or the running executable
				if _, err := os.Stat("bin/bashy"); err == nil {
					binaryPath = "bin/bashy"
				} else if _, err := os.Stat("bashy"); err == nil {
					binaryPath = "bashy"
				} else {
					self, err := os.Executable()
					if err == nil {
						binaryPath = self
					}
				}
			}

			// If dist directory is specified, scan for binaries
			if distDir != "" {
				return runDistSBOM(cmd.OutOrStdout(), cmd.ErrOrStderr(), distDir, outputPath, gate)
			}

			if binaryPath == "" {
				return errors.New("no binary specified; pass a path to the binary or use --dist")
			}

			opts := SBOMOptions{
				TargetOS:   targetOS,
				TargetArch: targetArch,
				Commit:     commit,
				BuildTags:  buildTags,
				Gate:       gate,
			}

			doc, err := GenerateSBOM(binaryPath, opts)
			if err != nil && doc == nil {
				return err
			}

			if checkOnly {
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "release sbom: PASS — all %d modules permissive\n", len(doc.Packages))
				return nil
			}

			data, marshalErr := json.MarshalIndent(doc, "", "  ")
			if marshalErr != nil {
				return fmt.Errorf("marshal SPDX JSON: %w", marshalErr)
			}

			if outputPath != "" && outputPath != "-" {
				if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(outputPath, append(data, '\n'), 0o644); err != nil {
					return fmt.Errorf("write SBOM: %w", err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "release sbom: wrote SPDX 2.3 SBOM (%d packages) to %s\n", len(doc.Packages), outputPath)
			} else {
				cmd.OutOrStdout().Write(append(data, '\n'))
			}

			if err != nil {
				return err
			}
			return nil
		},
	}

	f := cmd.Flags()
	f.StringVar(&binaryPath, "binary", "", "Path to the binary to inspect")
	f.StringVarP(&outputPath, "output", "o", "", "Output file path for SPDX JSON (default: stdout)")
	f.StringVar(&distDir, "dist", "", "Directory containing built release binaries to scan and gate")
	f.StringVar(&targetOS, "os", "", "Target OS override")
	f.StringVar(&targetArch, "arch", "", "Target Arch override")
	f.StringVar(&commit, "commit", "", "Commit SHA override")
	f.StringVar(&buildTags, "tags", "", "Build tags override")
	f.BoolVar(&gate, "gate", true, "Fail closed on any non-permissive or unknown license")
	f.BoolVar(&checkOnly, "check-only", false, "Only validate licenses and gate without writing SBOM")

	return cmd
}

func runDistSBOM(stdout, stderr io.Writer, distDir, outDir string, gate bool) error {
	entries, err := os.ReadDir(distDir)
	if err != nil {
		return fmt.Errorf("read dist dir: %w", err)
	}

	var binaries []string
	for _, e := range entries {
		if e.IsDir() {
			subEntries, _ := os.ReadDir(filepath.Join(distDir, e.Name()))
			for _, se := range subEntries {
				if !se.IsDir() && (strings.HasPrefix(se.Name(), "bashy") || strings.HasPrefix(se.Name(), "bash")) && !strings.HasSuffix(se.Name(), ".spdx.json") && !strings.HasSuffix(se.Name(), ".sha256") {
					binaries = append(binaries, filepath.Join(distDir, e.Name(), se.Name()))
				}
			}
		} else if strings.HasPrefix(e.Name(), "bashy") && !strings.HasSuffix(e.Name(), ".tar.gz") && !strings.HasSuffix(e.Name(), ".zip") && !strings.HasSuffix(e.Name(), ".spdx.json") && !strings.HasSuffix(e.Name(), ".sha256") {
			binaries = append(binaries, filepath.Join(distDir, e.Name()))
		}
	}

	if len(binaries) == 0 {
		return fmt.Errorf("no release binaries found in %s", distDir)
	}

	if outDir == "" {
		outDir = filepath.Join(distDir, "sbom")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	for _, bin := range binaries {
		doc, err := GenerateSBOM(bin, SBOMOptions{Gate: gate})
		if err != nil {
			return fmt.Errorf("%s: %w", bin, err)
		}
		outPath := filepath.Join(outDir, doc.Name+".spdx.json")
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(outPath, append(data, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(stderr, "release sbom: generated %s (%d packages)\n", outPath, len(doc.Packages))
	}
	return nil
}
