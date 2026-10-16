// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestClassifyLicense(t *testing.T) {
	tests := []struct {
		name      string
		content   string
		wantLic   string
		wantClass LicenseClass
	}{
		{
			name:      "Empty",
			content:   "",
			wantLic:   "UNKNOWN",
			wantClass: LicenseUnknown,
		},
		{
			name:      "Whitespace only",
			content:   "   \n\t  ",
			wantLic:   "UNKNOWN",
			wantClass: LicenseUnknown,
		},
		{
			name:      "SPDX MIT",
			content:   "// SPDX-License-Identifier: MIT\npackage foo",
			wantLic:   "MIT",
			wantClass: LicensePermissive,
		},
		{
			name:      "SPDX Apache-2.0 OR MIT",
			content:   "`SPDX-License-Identifier: Apache-2.0 OR MIT`",
			wantLic:   "Apache-2.0 OR MIT",
			wantClass: LicensePermissive,
		},
		{
			name:      "SPDX BSD-3-Clause AND MPL-2.0",
			content:   "`SPDX-License-Identifier: BSD-3-Clause AND MPL-2.0`",
			wantLic:   "BSD-3-Clause AND MPL-2.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "SPDX GPL-3.0",
			content:   "SPDX-License-Identifier: GPL-3.0",
			wantLic:   "GPL-3.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "MIT Text",
			content:   "Permission is hereby granted, free of charge, to any person obtaining a copy without restriction",
			wantLic:   "MIT",
			wantClass: LicensePermissive,
		},
		{
			name:      "Apache-2.0 Text",
			content:   "Apache License\nVersion 2.0, January 2004\nhttp://www.apache.org/licenses/",
			wantLic:   "Apache-2.0",
			wantClass: LicensePermissive,
		},
		{
			name:      "BSD-3-Clause Text",
			content:   "Redistribution and use in source and binary forms, with or without modification, are permitted provided that neither the name of the author nor the names of contributors may be used",
			wantLic:   "BSD-3-Clause",
			wantClass: LicensePermissive,
		},
		{
			name:      "BSD-2-Clause Text",
			content:   "Redistribution and use in source and binary forms, with or without modification, are permitted provided that copyright notice appears",
			wantLic:   "BSD-2-Clause",
			wantClass: LicensePermissive,
		},
		{
			name:      "0BSD Text",
			content:   "Zero-Clause BSD\nPermission to use, copy, modify, and/or distribute this software for any purpose with or without fee is hereby granted.",
			wantLic:   "0BSD",
			wantClass: LicensePermissive,
		},
		{
			name:      "ISC Text",
			content:   "ISC License\nPermission to use, copy, modify, and distribute this software for any purpose",
			wantLic:   "ISC",
			wantClass: LicensePermissive,
		},
		{
			name:      "CC0-1.0 Text",
			content:   "CC0 1.0 Universal\nCreative Commons Zero Public Domain Dedication",
			wantLic:   "CC0-1.0",
			wantClass: LicensePermissive,
		},
		{
			name:      "Unlicense Text",
			content:   "This is free and unencumbered software released into the public domain.",
			wantLic:   "Unlicense",
			wantClass: LicensePermissive,
		},
		{
			name:      "Zlib Text",
			content:   "This software is provided 'as-is', without any express or implied warranty. In no event will the authors be held liable for any damages arising from the use of this software.",
			wantLic:   "Zlib",
			wantClass: LicensePermissive,
		},
		{
			name:      "MPL-2.0 Text",
			content:   "Mozilla Public License, version 2.0\n1. Definitions\n1.1. Contributor",
			wantLic:   "MPL-2.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "CPAL-1.0 Text",
			content:   "Common Public Attribution License Version 1.0 (CPAL)",
			wantLic:   "CPAL-1.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "GPL-3.0 Text",
			content:   "GNU General Public License\nVersion 3, 29 June 2007",
			wantLic:   "GPL-3.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "GPL-2.0 Text",
			content:   "GNU GENERAL PUBLIC LICENSE\nVersion 2, June 1991",
			wantLic:   "GPL-2.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "LGPL-3.0 Text",
			content:   "GNU Lesser General Public License\nVersion 3, 29 June 2007",
			wantLic:   "LGPL-3.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "LGPL-2.1 Text",
			content:   "GNU LESSER GENERAL PUBLIC LICENSE\nVersion 2.1, February 1999",
			wantLic:   "LGPL-2.1",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "SSPL-1.0 Text",
			content:   "Server Side Public License\nVERSION 1, OCTOBER 16, 2018",
			wantLic:   "SSPL-1.0",
			wantClass: LicenseNonPermissive,
		},
		{
			name:      "Unknown Proprietary Text",
			content:   "All rights reserved. Unauthorized copying or redistribution is strictly prohibited.",
			wantLic:   "UNKNOWN",
			wantClass: LicenseUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLic, gotClass := ClassifyLicense(tt.content)
			if gotClass != tt.wantClass {
				t.Errorf("ClassifyLicense() class = %v, want %v", gotClass, tt.wantClass)
			}
			if gotLic != tt.wantLic {
				t.Errorf("ClassifyLicense() lic = %v, want %v", gotLic, tt.wantLic)
			}
		})
	}
}

func TestLoadReviewRegistry(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("WrappedExceptionsFormat", func(t *testing.T) {
		p := filepath.Join(tempDir, "review1.json")
		content := `{
			"schema": "bashy-license-review-v1",
			"exceptions": [
				{
					"path": "github.com/foo/bar/LICENSE",
					"sha256": "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
					"reason": "reviewed exception"
				}
			]
		}`
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		reg, err := LoadReviewRegistry(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reg.Exceptions) != 1 {
			t.Fatalf("expected 1 exception, got %d", len(reg.Exceptions))
		}
		if reg.Exceptions[0].Path != "github.com/foo/bar/LICENSE" {
			t.Errorf("wrong path: %s", reg.Exceptions[0].Path)
		}
	})

	t.Run("InputsMapFormat", func(t *testing.T) {
		p := filepath.Join(tempDir, "review2.json")
		content := `{
			"inputs": {
				"deps/foo/LICENSE": {
					"sha256": "1111111111111111111111111111111111111111111111111111111111111111",
					"reason": "prima style notice"
				}
			]
		}`
		// Fix syntax for test
		content = `{
			"inputs": {
				"deps/foo/LICENSE": {
					"sha256": "1111111111111111111111111111111111111111111111111111111111111111",
					"reason": "prima style notice"
				}
			}
		}`
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		reg, err := LoadReviewRegistry(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reg.Exceptions) != 1 {
			t.Fatalf("expected 1 exception, got %d", len(reg.Exceptions))
		}
		if reg.Exceptions[0].Path != "deps/foo/LICENSE" {
			t.Errorf("wrong path: %s", reg.Exceptions[0].Path)
		}
	})

	t.Run("NonExistentFileReturnsEmpty", func(t *testing.T) {
		p := filepath.Join(tempDir, "nonexistent.json")
		reg, err := LoadReviewRegistry(p)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(reg.Exceptions) != 0 {
			t.Errorf("expected 0 exceptions, got %d", len(reg.Exceptions))
		}
	})
}

func TestMatchException(t *testing.T) {
	fakeSHA := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	registry := &ReviewRegistry{
		Exceptions: []Exception{
			{
				Path:   "github.com/foo/bar/LICENSE",
				SHA256: fakeSHA,
				Reason: "Valid approved exception",
			},
			{
				Path:   "github.com/tampered/lib/LICENSE",
				SHA256: fakeSHA,
				Reason: "Tampered test",
			},
			{
				Path:   "github.com/noreason/lib/LICENSE",
				SHA256: fakeSHA,
				Reason: "   ",
			},
		},
	}

	t.Run("ExactMatch", func(t *testing.T) {
		f := &Finding{
			Module: "github.com/foo/bar",
			File:   "LICENSE",
			Path:   "/some/abs/dir/github.com/foo/bar/LICENSE",
			SHA256: fakeSHA,
		}
		ex, matched, err := registry.MatchException(f, "/repo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !matched {
			t.Fatal("expected match")
		}
		if ex.Reason != "Valid approved exception" {
			t.Errorf("wrong reason: %s", ex.Reason)
		}
	})

	t.Run("TamperDetection", func(t *testing.T) {
		f := &Finding{
			Module: "github.com/tampered/lib",
			File:   "LICENSE",
			Path:   "/some/abs/dir/github.com/tampered/lib/LICENSE",
			SHA256: "9999999999999999999999999999999999999999999999999999999999999999",
		}
		_, matched, err := registry.MatchException(f, "/repo")
		if err == nil {
			t.Fatal("expected tamper detection error, got nil")
		}
		if matched {
			t.Fatal("should not be matched when tampered")
		}
	})

	t.Run("EmptyReasonError", func(t *testing.T) {
		f := &Finding{
			Module: "github.com/noreason/lib",
			File:   "LICENSE",
			Path:   "/some/abs/dir/github.com/noreason/lib/LICENSE",
			SHA256: fakeSHA,
		}
		_, _, err := registry.MatchException(f, "/repo")
		if err == nil {
			t.Fatal("expected empty reason error, got nil")
		}
	})

	t.Run("NoMatch", func(t *testing.T) {
		f := &Finding{
			Module: "github.com/other/pkg",
			File:   "LICENSE",
			Path:   "/some/abs/dir/github.com/other/pkg/LICENSE",
			SHA256: fakeSHA,
		}
		ex, matched, err := registry.MatchException(f, "/repo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if matched || ex != nil {
			t.Fatal("expected no match")
		}
	})
}

func TestScanVendoredDirectories(t *testing.T) {
	tempDir := t.TempDir()

	// Create third_party and vendor dirs
	tpDir := filepath.Join(tempDir, "third_party", "mytool")
	if err := os.MkdirAll(tpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lic1 := filepath.Join(tpDir, "LICENSE")
	if err := os.WriteFile(lic1, []byte("MIT License\nPermission is hereby granted"), 0o644); err != nil {
		t.Fatal(err)
	}

	vendorDir := filepath.Join(tempDir, "vendor", "subpkg")
	if err := os.MkdirAll(vendorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lic2 := filepath.Join(vendorDir, "COPYING")
	if err := os.WriteFile(lic2, []byte("Mozilla Public License, version 2.0"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create non-vendored dir that should NOT be scanned
	srcDir := filepath.Join(tempDir, "src", "pkg")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "LICENSE"), []byte("MIT"), 0o644); err != nil {
		t.Fatal(err)
	}

	files, err := ScanVendoredDirectories(tempDir)
	if err != nil {
		t.Fatalf("ScanVendoredDirectories failed: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected 2 vendored license files, got %d: %v", len(files), files)
	}

	found1 := false
	found2 := false
	for _, f := range files {
		if f == lic1 {
			found1 = true
		}
		if f == lic2 {
			found2 = true
		}
	}
	if !found1 || !found2 {
		t.Errorf("did not find expected files: %v", files)
	}
}

func TestScanModule(t *testing.T) {
	tempDir := t.TempDir()

	t.Run("PermissiveModule", func(t *testing.T) {
		modDir := filepath.Join(tempDir, "mod-mit")
		if err := os.MkdirAll(modDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := []byte("MIT License\nPermission is hereby granted, free of charge without restriction")
		if err := os.WriteFile(filepath.Join(modDir, "LICENSE"), content, 0o644); err != nil {
			t.Fatal(err)
		}

		mod := ModuleInfo{
			Path:    "example.com/mod-mit",
			Version: "v1.0.0",
			Dir:     modDir,
		}

		findings, err := ScanModule(mod)
		if err != nil {
			t.Fatalf("ScanModule failed: %v", err)
		}
		if len(findings) != 1 {
			t.Fatalf("expected 1 finding, got %d", len(findings))
		}
		if findings[0].Class != LicensePermissive {
			t.Errorf("expected permissive, got %v", findings[0].Class)
		}
		if findings[0].License != "MIT" {
			t.Errorf("expected MIT, got %s", findings[0].License)
		}
		expectedSHA := sha256.Sum256(content)
		if findings[0].SHA256 != hex.EncodeToString(expectedSHA[:]) {
			t.Errorf("SHA mismatch: got %s", findings[0].SHA256)
		}
	})

	t.Run("CopyleftModule", func(t *testing.T) {
		modDir := filepath.Join(tempDir, "mod-gpl")
		if err := os.MkdirAll(modDir, 0o755); err != nil {
			t.Fatal(err)
		}
		content := []byte("GNU General Public License\nVersion 3, 29 June 2007")
		if err := os.WriteFile(filepath.Join(modDir, "COPYING"), content, 0o644); err != nil {
			t.Fatal(err)
		}

		mod := ModuleInfo{
			Path:    "example.com/mod-gpl",
			Version: "v2.0.0",
			Dir:     modDir,
		}

		findings, err := ScanModule(mod)
		if err != nil {
			t.Fatalf("ScanModule failed: %v", err)
		}
		if len(findings) != 1 {
			t.Fatalf("expected 1 finding, got %d", len(findings))
		}
		if findings[0].Class != LicenseNonPermissive {
			t.Errorf("expected non-permissive, got %v", findings[0].Class)
		}
		if findings[0].License != "GPL-3.0" {
			t.Errorf("expected GPL-3.0, got %s", findings[0].License)
		}
	})
}

func TestRunGateCleanWithExceptions(t *testing.T) {
	tempDir := t.TempDir()

	// Create a module with MPL-2.0
	modDir := filepath.Join(tempDir, "mpl-mod")
	if err := os.MkdirAll(modDir, 0o755); err != nil {
		t.Fatal(err)
	}
	mplContent := []byte("Mozilla Public License, version 2.0")
	licPath := filepath.Join(modDir, "LICENSE")
	if err := os.WriteFile(licPath, mplContent, 0o644); err != nil {
		t.Fatal(err)
	}
	mplSHA := sha256.Sum256(mplContent)
	shaHex := hex.EncodeToString(mplSHA[:])

	// Create review-file granting reviewed exception
	reviewFile := filepath.Join(tempDir, "license-review.json")
	reviewContent := `{
		"schema": "bashy-license-review-v1",
		"exceptions": [
			{
				"path": "example.com/mpl-mod/LICENSE",
				"sha256": "` + shaHex + `",
				"reason": "Reviewed MPL exception for test"
			}
		]
	}`
	if err := os.WriteFile(reviewFile, []byte(reviewContent), 0o644); err != nil {
		t.Fatal(err)
	}

	reg, err := LoadReviewRegistry(reviewFile)
	if err != nil {
		t.Fatal(err)
	}

	finding := Finding{
		Module:  "example.com/mpl-mod",
		File:    "LICENSE",
		Path:    licPath,
		License: "MPL-2.0",
		Class:   LicenseNonPermissive,
		SHA256:  shaHex,
	}

	ex, matched, mErr := reg.MatchException(&finding, tempDir)
	if mErr != nil {
		t.Fatalf("unexpected match error: %v", mErr)
	}
	if !matched || ex == nil {
		t.Fatal("expected exception to match")
	}
}
