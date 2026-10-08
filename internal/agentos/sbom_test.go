// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifyLicense(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		wantID    string
		wantClass LicenseClass
	}{
		{
			name:      "MIT License standard",
			text:      "MIT License\n\nCopyright (c) 2025 Alice\n\nPermission is hereby granted, free of charge, to any person obtaining a copy...",
			wantID:    "MIT",
			wantClass: LicensePermissive,
		},
		{
			name:      "Apache-2.0 License standard",
			text:      "                                 Apache License\n                           Version 2.0, January 2004\n                        http://www.apache.org/licenses/",
			wantID:    "Apache-2.0",
			wantClass: LicensePermissive,
		},
		{
			name:      "BSD-3-Clause Go authors",
			text:      "Copyright 2009 The Go Authors.\n\nRedistribution and use in source and binary forms, with or without\nmodification, are permitted provided that the following conditions are\nmet:\n\n   * Redistributions of source code must retain the above copyright\nnotice, this list of conditions and the following disclaimer.\n   * Redistributions in binary form must reproduce the above\ncopyright notice, this list of conditions and the following disclaimer\nin the documentation and/or other materials provided with the\ndistribution.\n   * Neither the name of Google LLC nor the names of its\ncontributors may be used to endorse or promote products derived from\nthis software without specific prior written permission.\n",
			wantID:    "BSD-3-Clause",
			wantClass: LicensePermissive,
		},
		{
			name:      "BSD-2-Clause",
			text:      "Redistribution and use in source and binary forms, with or without modification, are permitted provided that the following conditions are met:\n1. Redistributions of source code must retain...\n2. Redistributions in binary form must reproduce...",
			wantID:    "BSD-2-Clause",
			wantClass: LicensePermissive,
		},
		{
			name:      "ISC License (Coder variant)",
			text:      "Permission to use, copy, modify, and distribute this software for any purpose with or without fee is hereby granted, provided that the above copyright notice and this permission notice appear in all copies.",
			wantID:    "ISC",
			wantClass: LicensePermissive,
		},
		{
			name:      "0BSD",
			text:      "Permission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted.\n\nTHE SOFTWARE IS PROVIDED \"AS IS\"...",
			wantID:    "0BSD",
			wantClass: LicensePermissive,
		},
		{
			name:      "CC0 1.0 Universal",
			text:      "The person who associated a work with this deed has dedicated the work to the public domain by waiving all of his or her rights to the work worldwide under copyright law... CC0 1.0 Universal",
			wantID:    "CC0-1.0",
			wantClass: LicensePermissive,
		},
		{
			name:      "Unlicense",
			text:      "This is free and unencumbered software released into the public domain.\n\nAnyone is free to copy, modify, publish, use, compile, sell, or distribute this software...",
			wantID:    "Unlicense",
			wantClass: LicensePermissive,
		},
		{
			name:      "GPL-3.0 non-permissive",
			text:      "GNU GENERAL PUBLIC LICENSE\nVersion 3, 29 June 2007\n\nCopyright (C) 2007 Free Software Foundation, Inc. <https://fsf.org/>",
			wantID:    "GPL-3.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "MPL-2.0 non-permissive",
			text:      "Mozilla Public License Version 2.0\n==================================\n\n1. Definitions\n1.1. \"Contributor\"...",
			wantID:    "MPL-2.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "LGPL-3.0 non-permissive",
			text:      "GNU LESSER GENERAL PUBLIC LICENSE\nVersion 3, 29 June 2007",
			wantID:    "LGPL-3.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "SPDX-License-Identifier in header",
			text:      "// SPDX-License-Identifier: Apache-2.0\npackage main\n",
			wantID:    "Apache-2.0",
			wantClass: LicensePermissive,
		},
		{
			name:      "Unknown proprietary text",
			text:      "All rights reserved. Proprietary and confidential. Unauthorized copying is strictly prohibited.",
			wantID:    "UNKNOWN",
			wantClass: LicenseUnknown,
		},
		{
			name:      "Empty license text",
			text:      "",
			wantID:    "UNKNOWN",
			wantClass: LicenseUnknown,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, class := ClassifyLicense(tc.text)
			if class != tc.wantClass {
				t.Errorf("class = %v, want %v", class, tc.wantClass)
			}
			if tc.wantID != "" && id != tc.wantID {
				t.Errorf("spdxID = %q, want %q", id, tc.wantID)
			}
		})
	}
}

func TestLicenseGate(t *testing.T) {
	// A valid permissive SBOM
	permDoc := &SPDXDocument{
		SPDXVersion: "SPDX-2.3",
		Packages: []SPDXPackage{
			{Name: "bashy", LicenseConcluded: "MIT", LicenseClassification: LicensePermissive},
			{Name: "github.com/spf13/cobra", LicenseConcluded: "Apache-2.0", LicenseClassification: LicensePermissive},
		},
	}
	if err := ValidateLicenseGate(permDoc); err != nil {
		t.Errorf("expected permissive doc to pass gate, got: %v", err)
	}

	// Non-permissive module fails gate
	nonPermDoc := &SPDXDocument{
		SPDXVersion: "SPDX-2.3",
		Packages: []SPDXPackage{
			{Name: "bashy", LicenseConcluded: "MIT", LicenseClassification: LicensePermissive},
			{Name: "github.com/hashicorp/yamux", LicenseConcluded: "MPL-2.0", LicenseClassification: LicenseNonPermissive},
		},
	}
	if err := ValidateLicenseGate(nonPermDoc); err == nil {
		t.Error("expected non-permissive doc to fail gate, got nil")
	} else if !strings.Contains(err.Error(), "github.com/hashicorp/yamux") {
		t.Errorf("expected error to mention yamux, got: %v", err)
	}

	// Unknown module fails gate
	unknownDoc := &SPDXDocument{
		SPDXVersion: "SPDX-2.3",
		Packages: []SPDXPackage{
			{Name: "bashy", LicenseConcluded: "MIT", LicenseClassification: LicensePermissive},
			{Name: "example.com/mystery", LicenseConcluded: "UNKNOWN", LicenseClassification: LicenseUnknown},
		},
	}
	if err := ValidateLicenseGate(unknownDoc); err == nil {
		t.Error("expected unknown doc to fail gate, got nil")
	} else if !strings.Contains(err.Error(), "example.com/mystery") {
		t.Errorf("expected error to mention example.com/mystery, got: %v", err)
	}
}

func TestVerifyPermissiveClosureDeps(t *testing.T) {
	// Must have no yamux and no outpost in go list -deps
	cmd := exec.Command("go", "list", "-deps", "./cmd/bashy")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps ./cmd/bashy: %v\n%s", err, out)
	}
	lines := strings.Fields(string(out))
	for _, line := range lines {
		if strings.Contains(line, "yamux") {
			t.Errorf("closure contains prohibited MPL tunnel dependency yamux: %s", line)
		}
		if line == "github.com/qiangli/outpost" || strings.HasPrefix(line, "github.com/qiangli/outpost/") {
			t.Errorf("closure contains prohibited outpost dependency: %s", line)
		}
		// Five non-permissive grammars dropped in Sprint 369
		for _, nonPermGrammar := range []string{
			"caddy", "disassembly", "tree_sitter_jq", "tree_sitter_ebnf", "tree_sitter_nim",
		} {
			if strings.Contains(line, nonPermGrammar) {
				t.Errorf("closure contains non-permissive grammar package: %s", line)
			}
		}
	}
}

func TestReleaseSbomCLI(t *testing.T) {
	// Build or locate bashy binary to test `bashy release sbom`
	binPath := filepath.Join(t.TempDir(), "test_bashy")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./cmd/bashy")
	buildCmd.Dir = filepath.Join("..", "..")
	buildCmd.Env = os.Environ()
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build test binary: %v\n%s", err, out)
	}

	outFile := filepath.Join(t.TempDir(), "sbom.spdx.json")
	stdout, stderr, err := runReleaseCmd(t, "sbom", "--binary", binPath, "-o", outFile)
	if err != nil {
		t.Fatalf("bashy release sbom failed: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("failed to read generated SBOM file: %v", err)
	}

	var doc SPDXDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("failed to parse generated SPDX JSON: %v\n%s", err, data)
	}

	if doc.SPDXVersion != "SPDX-2.3" {
		t.Errorf("spdxVersion = %q, want SPDX-2.3", doc.SPDXVersion)
	}
	if doc.DataLicense != "CC0-1.0" {
		t.Errorf("dataLicense = %q, want CC0-1.0", doc.DataLicense)
	}
	if len(doc.Packages) == 0 {
		t.Fatal("expected at least one package in SBOM")
	}

	// Verify root package
	rootPkg := doc.Packages[0]
	if rootPkg.Name != "bashy" {
		t.Errorf("root package name = %q, want bashy", rootPkg.Name)
	}
	if len(rootPkg.Checksums) == 0 || rootPkg.Checksums[0].ChecksumValue == "" {
		t.Errorf("root package missing SHA256 checksum")
	}

	// Verify every package has a concluded license and is permissive
	for _, pkg := range doc.Packages {
		if pkg.LicenseConcluded == "" || pkg.LicenseConcluded == "UNKNOWN" {
			t.Errorf("package %s has non-permissive or unknown license: %s", pkg.Name, pkg.LicenseConcluded)
		}
	}
}
