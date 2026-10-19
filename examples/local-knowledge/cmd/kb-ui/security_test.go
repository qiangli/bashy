// Copyright (c) 2026 Qiang Li. All rights reserved.
// Ported from kg-ui under explicit operator authorization for Bashy.
// See ATTRIBUTION.md and LICENSE for licensing information.

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFilePreviewPath(t *testing.T) {
	root := t.TempDir()
	subDir := filepath.Join(root, "docs")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	validFile := filepath.Join(subDir, "readme.txt")
	if err := os.WriteFile(validFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create symlink within root
	validSymlink := filepath.Join(root, "link_to_readme.txt")
	if err := os.Symlink(validFile, validSymlink); err != nil {
		t.Fatal(err)
	}

	// Create symlink escaping root
	outsideFile := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("outside secret"), 0644); err != nil {
		t.Fatal(err)
	}
	escapingSymlink := filepath.Join(root, "escape.txt")
	if err := os.Symlink(outsideFile, escapingSymlink); err != nil {
		t.Fatal(err)
	}

	// Create sensitive file
	gitDir := filepath.Join(root, ".git")
	_ = os.Mkdir(gitDir, 0755)
	gitConfig := filepath.Join(gitDir, "config")
	_ = os.WriteFile(gitConfig, []byte("[core]"), 0644)

	envFile := filepath.Join(root, ".env")
	_ = os.WriteFile(envFile, []byte("SECRET=123"), 0644)

	roots := []string{root}
	evalValidFile, _ := filepath.EvalSymlinks(validFile)

	// 1. Valid file directly
	p, err := ValidateFilePreviewPath(validFile, roots)
	if err != nil || p != evalValidFile {
		t.Fatalf("want valid file %s, got %s (err: %v)", evalValidFile, p, err)
	}

	// 2. Valid file with file:// scheme
	p, err = ValidateFilePreviewPath("file://"+validFile, roots)
	if err != nil || p != evalValidFile {
		t.Fatalf("want file:// valid file, got %s (err: %v)", p, err)
	}

	// 3. Valid in-root symlink
	p, err = ValidateFilePreviewPath(validSymlink, roots)
	if err != nil || p != evalValidFile {
		t.Fatalf("want resolved in-root symlink, got %s (err: %v)", p, err)
	}

	// 4. Outside-root symlink rejection
	_, err = ValidateFilePreviewPath(escapingSymlink, roots)
	if err != ErrSymlinkEscapesRoot {
		t.Fatalf("want ErrSymlinkEscapesRoot, got %v", err)
	}

	// 5. Outside root path rejection
	_, err = ValidateFilePreviewPath(outsideFile, roots)
	if err != ErrOutsideRoot {
		t.Fatalf("want ErrOutsideRoot, got %v", err)
	}

	// 6. Path traversal rejection
	traversal := filepath.Join(subDir, "..", "..", "opt", "other.txt")
	_, err = ValidateFilePreviewPath(traversal, roots)
	if err != ErrOutsideRoot {
		t.Fatalf("want traversal rejected as ErrOutsideRoot, got %v", err)
	}

	// 7. Relative path rejection
	_, err = ValidateFilePreviewPath("docs/readme.txt", roots)
	if err != ErrPathNotAbsolute {
		t.Fatalf("want ErrPathNotAbsolute, got %v", err)
	}

	// 8. Sensitive .git config rejection
	_, err = ValidateFilePreviewPath(gitConfig, roots)
	if err != ErrSensitiveFile {
		t.Fatalf("want ErrSensitiveFile for .git/config, got %v", err)
	}

	// 9. Sensitive .env rejection
	_, err = ValidateFilePreviewPath(envFile, roots)
	if err != ErrSensitiveFile {
		t.Fatalf("want ErrSensitiveFile for .env, got %v", err)
	}

	// 10. Directory rejection
	_, err = ValidateFilePreviewPath(subDir, roots)
	if err != ErrNotRegularFile {
		t.Fatalf("want ErrNotRegularFile for directory, got %v", err)
	}

	// 11. Empty roots rejection
	_, err = ValidateFilePreviewPath(validFile, []string{})
	if err != ErrOutsideRoot {
		t.Fatalf("want ErrOutsideRoot when no roots configured, got %v", err)
	}
}

func TestCheckMutationAllowed(t *testing.T) {
	// 1. GET requests are always allowed
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6674/api/stats", nil)
	if err := CheckMutationAllowed(req); err != nil {
		t.Fatalf("GET should be allowed: %v", err)
	}

	// 2. POST with cross-site Sec-Fetch-Site is rejected
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6674/api/control/pause", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("X-KB-UI", "1")
	if err := CheckMutationAllowed(req); err != ErrForbiddenCrossSite {
		t.Fatalf("want ErrForbiddenCrossSite, got %v", err)
	}

	// 3. POST with cross-origin Origin is rejected
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6674/api/control/pause", nil)
	req.Header.Set("Origin", "https://evil.com")
	req.Header.Set("X-KB-UI", "1")
	if err := CheckMutationAllowed(req); err == nil || !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("want cross-origin error, got %v", err)
	}

	// 4. POST with missing anti-CSRF header is rejected
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6674/api/control/pause", nil)
	req.Header.Set("Origin", "http://127.0.0.1:6674")
	if err := CheckMutationAllowed(req); err != ErrCSRFHeaderRequired {
		t.Fatalf("want ErrCSRFHeaderRequired, got %v", err)
	}

	// 5. POST with matching Origin and X-KB-UI header is allowed
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6674/api/control/pause", nil)
	req.Header.Set("Origin", "http://127.0.0.1:6674")
	req.Header.Set("X-KB-UI", "1")
	if err := CheckMutationAllowed(req); err != nil {
		t.Fatalf("same-origin POST with X-KB-UI should be allowed: %v", err)
	}

	// 6. POST with localhost Origin against 127.0.0.1 host (equivalent loopback) is allowed
	req = httptest.NewRequest(http.MethodPost, "http://127.0.0.1:6674/api/control/pause", nil)
	req.Header.Set("Origin", "http://localhost:6674")
	req.Header.Set("Content-Type", "application/json")
	if err := CheckMutationAllowed(req); err != nil {
		t.Fatalf("equivalent loopback host should be allowed: %v", err)
	}
}

func TestSanitizeBasePrefix(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"", "/"},
		{"/kb", "/kb/"},
		{"/app/kb/", "/app/kb/"},
		{"/h/dragon/app/kb", "/h/dragon/app/kb/"},
		{`"><script>alert(1)</script>`, "/scriptalert1/script/"},
	}

	for _, c := range cases {
		got := SanitizeBasePrefix(c.input)
		if got != c.want {
			t.Errorf("SanitizeBasePrefix(%q): want %q, got %q", c.input, c.want, got)
		}
		if strings.Contains(got, "<") || strings.Contains(got, ">") || strings.Contains(got, `"`) {
			t.Errorf("SanitizeBasePrefix(%q) contains unescaped HTML characters: %q", c.input, got)
		}
	}
}
