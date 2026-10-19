// Copyright (c) 2026 Qiang Li. All rights reserved.
// Ported from kg-ui under explicit operator authorization for Bashy.
// See ATTRIBUTION.md and LICENSE for licensing information.

package main

import (
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrInvalidPath        = errors.New("invalid or empty file path")
	ErrPathNotAbsolute    = errors.New("path must be absolute")
	ErrSensitiveFile      = errors.New("access to sensitive credentials or system files is denied")
	ErrOutsideRoot        = errors.New("file path is outside configured roots")
	ErrSymlinkEscapesRoot = errors.New("symlink target escapes configured roots")
	ErrNotRegularFile     = errors.New("path is not a regular file")
	ErrForbiddenCrossSite = errors.New("cross-site requests are forbidden")
	ErrForbiddenOrigin    = errors.New("cross-origin requests are forbidden")
	ErrForbiddenReferer   = errors.New("referer origin mismatch")
	ErrCSRFHeaderRequired = errors.New("mutation requests require anti-CSRF header (X-KB-UI: 1)")
)

// sensitivePatterns list prefixes and file substrings that must never be previewed.
var sensitivePatterns = []string{
	string(filepath.Separator) + ".git",
	string(filepath.Separator) + ".ssh",
	string(filepath.Separator) + ".gnupg",
	string(filepath.Separator) + ".aws",
	string(filepath.Separator) + ".azure",
	string(filepath.Separator) + ".kube",
	string(filepath.Separator) + ".env",
	"id_rsa",
	"id_ed25519",
	"id_ecdsa",
	"authorized_keys",
	"known_hosts",
	"/etc/shadow",
	"/etc/passwd",
	"/etc/sudoers",
	"secrets.yaml",
	"credentials.json",
}

// isSensitive checks if a path matches known credential/system patterns.
func isSensitive(cleanPath string) bool {
	lower := strings.ToLower(cleanPath)
	base := strings.ToLower(filepath.Base(cleanPath))

	if strings.HasPrefix(base, ".env") || base == ".git" || base == ".ssh" || base == ".bash_history" || base == ".zsh_history" {
		return true
	}

	for _, p := range sensitivePatterns {
		if strings.Contains(cleanPath, p) || strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// isUnderRootsLexical checks if cleanPath is lexically located within at least one root directory.
func isUnderRootsLexical(cleanPath string, roots []string) bool {
	for _, root := range roots {
		rClean := filepath.Clean(root)
		if cleanPath == rClean || strings.HasPrefix(cleanPath, rClean+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// isUnderRootsResolved checks if resolvedPath is located within at least one symlink-resolved root directory.
func isUnderRootsResolved(resolvedPath string, roots []string) bool {
	for _, root := range roots {
		rResolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			rResolved = filepath.Clean(root)
		} else {
			rResolved = filepath.Clean(rResolved)
		}
		if resolvedPath == rResolved || strings.HasPrefix(resolvedPath, rResolved+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// ValidateFilePreviewPath verifies that rawURI points to an eligible, regular
// document under one of the explicitly allowed roots. Traversal, symlink escapes,
// and credential files are strictly denied.
func ValidateFilePreviewPath(rawURI string, roots []string) (string, error) {
	if len(roots) == 0 {
		return "", ErrOutsideRoot
	}

	rawURI = strings.TrimSpace(rawURI)
	if rawURI == "" {
		return "", ErrInvalidPath
	}

	// Strip file:// prefix if present
	cleanPath := rawURI
	if strings.HasPrefix(cleanPath, "file://") {
		cleanPath = strings.TrimPrefix(cleanPath, "file://")
	}

	// URL-unescape in case path was percent-encoded
	if unescaped, err := url.PathUnescape(cleanPath); err == nil {
		cleanPath = unescaped
	}

	cleanPath = filepath.Clean(cleanPath)
	if !filepath.IsAbs(cleanPath) {
		return "", ErrPathNotAbsolute
	}

	// Guard against sensitive credentials
	if isSensitive(cleanPath) {
		return "", ErrSensitiveFile
	}

	// Must be lexically under an eligible root before resolving symlinks
	if !isUnderRootsLexical(cleanPath, roots) {
		return "", ErrOutsideRoot
	}

	// Resolve symlinks and verify resolved target
	resolved, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		return "", err
	}
	resolvedClean := filepath.Clean(resolved)

	// Guard against resolved sensitive credentials
	if isSensitive(resolvedClean) {
		return "", ErrSensitiveFile
	}

	// Resolved target must ALSO stay within the allowed roots
	if !isUnderRootsResolved(resolvedClean, roots) {
		return "", ErrSymlinkEscapesRoot
	}

	// Verify it is a regular file
	fi, err := os.Stat(resolvedClean)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", ErrNotRegularFile
	}

	return resolvedClean, nil
}

// isLoopbackHost checks if host matches 127.0.0.1 or localhost.
func isLoopbackHost(host string) bool {
	hostname := host
	if h, _, found := strings.Cut(host, ":"); found && h != "" {
		hostname = h
	}
	return hostname == "127.0.0.1" || hostname == "localhost" || hostname == "::1"
}

// checkHostEquivalence checks if two hosts match or are equivalent loopback hosts on the same port.
func checkHostEquivalence(a, b string) bool {
	if strings.EqualFold(a, b) {
		return true
	}
	h1, p1, _ := strings.Cut(a, ":")
	h2, p2, _ := strings.Cut(b, ":")
	if isLoopbackHost(h1) && isLoopbackHost(h2) && p1 == p2 {
		return true
	}
	return false
}

// CheckMutationAllowed enforces CSRF and cross-origin defenses for state-changing
// HTTP requests (POST, PUT, DELETE, PATCH).
func CheckMutationAllowed(r *http.Request) error {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return nil
	}

	// 1. Fetch metadata defense (modern browsers)
	fetchSite := r.Header.Get("Sec-Fetch-Site")
	if strings.EqualFold(fetchSite, "cross-site") {
		return ErrForbiddenCrossSite
	}

	// 2. Origin check
	origin := r.Header.Get("Origin")
	if origin != "" {
		u, err := url.Parse(origin)
		if err != nil || !checkHostEquivalence(u.Host, r.Host) {
			return fmt.Errorf("%w: origin %q does not match host %q", ErrForbiddenOrigin, origin, r.Host)
		}
	}

	// 3. Referer check (fallback when Origin is absent)
	referer := r.Header.Get("Referer")
	if origin == "" && referer != "" {
		u, err := url.Parse(referer)
		if err != nil || !checkHostEquivalence(u.Host, r.Host) {
			return fmt.Errorf("%w: referer %q does not match host %q", ErrForbiddenReferer, referer, r.Host)
		}
	}

	// 4. Anti-CSRF token / custom header requirement.
	// Cross-site HTML forms and simple requests cannot attach custom headers.
	hasCustomHeader := r.Header.Get("X-KB-UI") != "" ||
		r.Header.Get("X-Requested-With") != "" ||
		strings.HasPrefix(r.Header.Get("Content-Type"), "application/json")

	if !hasCustomHeader {
		return ErrCSRFHeaderRequired
	}

	return nil
}

// SanitizeBasePrefix sanitizes and HTML-escapes an X-Forwarded-Prefix for safe inclusion
// in `<base href="...">`.
func SanitizeBasePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		return "/"
	}

	// Filter out characters that are invalid in URLs or could inject into HTML
	var b strings.Builder
	for _, ch := range prefix {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') ||
			(ch >= '0' && ch <= '9') || ch == '/' || ch == '-' ||
			ch == '_' || ch == '.' || ch == '~' {
			b.WriteRune(ch)
		}
	}
	clean := b.String()
	if !strings.HasPrefix(clean, "/") {
		clean = "/" + clean
	}
	return html.EscapeString(clean + "/")
}
