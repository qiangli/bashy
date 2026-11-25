package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

var sha256PinRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ComputeCoreDigest returns the deterministic "sha256:<hex>" digest over the
// form server module source files in appRoot (go.mod, go.sum, and all regular
// non-hidden files under server/), normalizing CRLF line endings so the digest
// is identical across Linux, macOS, and Windows checkouts.
func ComputeCoreDigest(appRoot string) (string, error) {
	if appRoot == "" {
		return "", errors.New("empty app root")
	}
	rels := []string{"go.mod", "go.sum"}
	for _, r := range rels {
		fi, err := os.Stat(filepath.Join(appRoot, r))
		if err != nil {
			return "", fmt.Errorf("core source missing %s: %w", r, err)
		}
		if !fi.Mode().IsRegular() {
			return "", fmt.Errorf("core source %s is not a regular file", r)
		}
	}
	serverDir := filepath.Join(appRoot, "server")
	err := filepath.WalkDir(serverDir, func(p string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(appRoot, p)
		if err != nil {
			return err
		}
		rels = append(rels, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("walk server core: %w", err)
	}
	sort.Strings(rels)

	var manifest bytes.Buffer
	for _, rel := range rels {
		raw, err := os.ReadFile(filepath.Join(appRoot, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		norm := bytes.ReplaceAll(raw, []byte{'\r'}, nil)
		sum := sha256.Sum256(norm)
		fmt.Fprintf(&manifest, "%x  %s\n", sum, rel)
	}
	total := sha256.Sum256(manifest.Bytes())
	return "sha256:" + hex.EncodeToString(total[:]), nil
}

// VerifyCoreDigest checks that the core source in appRoot matches server.digest
// in manifestPath (shaped.json). It fails closed on missing, malformed, or
// mismatched digests.
func VerifyCoreDigest(appRoot, manifestPath string) error {
	if manifestPath == "" {
		manifestPath = filepath.Join(appRoot, "shaped.json")
	}
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", manifestPath, err)
	}
	var doc struct {
		Server *struct {
			Digest string `json:"digest"`
		} `json:"server"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", manifestPath, err)
	}
	if doc.Server == nil || strings.TrimSpace(doc.Server.Digest) == "" {
		return fmt.Errorf("%s: missing server.digest pin for trusted core", manifestPath)
	}
	want := strings.TrimSpace(doc.Server.Digest)
	if !sha256PinRe.MatchString(want) {
		return fmt.Errorf("%s: invalid server.digest %q (want sha256:<64 hex>)", manifestPath, want)
	}
	got, err := ComputeCoreDigest(appRoot)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("form core source digest mismatch: pinned %s in %s, got %s (server/ is the trusted core and cannot be modified)", want, manifestPath, got)
	}
	return nil
}

// openFormURL opens a validated HTTP(S) loopback/console URL in the user's
// browser. FORM_BROWSER (then BROWSER) overrides the launcher; "none" or ":"
// succeeds without spawning a browser for non-GUI test harnesses.
func openFormURL(targetURL string) error {
	u, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("refusing to open non-HTTP(S) URL %q", targetURL)
	}
	clean := u.String()
	launcher := strings.TrimSpace(os.Getenv("FORM_BROWSER"))
	if launcher == "" {
		launcher = strings.TrimSpace(os.Getenv("BROWSER"))
	}
	if launcher != "" {
		if launcher == "none" || launcher == ":" || launcher == "true" {
			return nil
		}
		out, err := exec.Command(launcher, clean).CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if msg != "" {
				return fmt.Errorf("%s: %v (%s)", launcher, err, msg)
			}
			return fmt.Errorf("%s: %w", launcher, err)
		}
		return nil
	}
	switch runtime.GOOS {
	case "darwin":
		if out, err := exec.Command("open", clean).CombinedOutput(); err != nil {
			return fmt.Errorf("open: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	case "windows":
		if out, err := exec.Command("rundll32", "url.dll,FileProtocolHandler", clean).CombinedOutput(); err != nil {
			return fmt.Errorf("rundll32: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	default:
		if os.Getenv("DISPLAY") == "" && os.Getenv("WAYLAND_DISPLAY") == "" {
			if wslview, lerr := exec.LookPath("wslview"); lerr == nil {
				if out, err := exec.Command(wslview, clean).CombinedOutput(); err != nil {
					return fmt.Errorf("wslview: %v (%s)", err, strings.TrimSpace(string(out)))
				}
				return nil
			}
			// Headless Linux session (e.g. gate host without X11/Wayland or BROWSER):
			// do not fail form creation when no desktop session exists.
			return nil
		}
		if out, err := exec.Command("xdg-open", clean).CombinedOutput(); err != nil {
			return fmt.Errorf("xdg-open: %v (%s)", err, strings.TrimSpace(string(out)))
		}
		return nil
	}
}
