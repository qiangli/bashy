//go:build linux

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/pem"
	"os"
	"path/filepath"

	"golang.org/x/crypto/x509roots/fallback/bundle"
)

// A FROM-scratch image (the offline bashy image) has no CA bundle. bashy's own
// TLS falls back to the embedded roots (the fallback import in main.go), but the
// toolchains it provisions and runs as separate processes — go fetching
// modules, uv, pip, curl — read the system bundle and fail. When the host has
// none and the caller set none, bashy writes the embedded roots to a PEM in its
// cache and exports SSL_CERT_FILE, which those programs honour.

// systemCABundles are the files Go's crypto/x509 reads on Linux.
var systemCABundles = []string{
	"/etc/ssl/certs/ca-certificates.crt",
	"/etc/pki/tls/certs/ca-bundle.crt",
	"/etc/ssl/ca-bundle.pem",
	"/etc/pki/tls/cacert.pem",
	"/etc/pki/ca-trust/extracted/pem/tls-ca-bundle.pem",
	"/etc/ssl/cert.pem",
}

func init() { exportFallbackCABundle() }

func exportFallbackCABundle() {
	if os.Getenv("SSL_CERT_FILE") != "" || os.Getenv("SSL_CERT_DIR") != "" {
		return
	}
	for _, path := range systemCABundles {
		if _, err := os.Stat(path); err == nil {
			return
		}
	}
	if path, err := writeFallbackCABundle(); err == nil {
		os.Setenv("SSL_CERT_FILE", path)
	}
}

// writeFallbackCABundle writes the unconstrained embedded roots as PEM (a root
// with an extra constraint cannot be expressed in PEM, so it is left out
// rather than widened). The file name carries the content hash, so a bundle
// update never reuses a stale file.
func writeFallbackCABundle() (string, error) {
	var pemBytes []byte
	for root := range bundle.Roots() {
		if root.Constraint != nil {
			continue
		}
		pemBytes = append(pemBytes, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: root.Certificate})...)
	}
	sum := sha256.Sum256(pemBytes)
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	dir = filepath.Join(dir, "bashy")
	path := filepath.Join(dir, "ca-roots-"+hex.EncodeToString(sum[:8])+".pem")
	if info, err := os.Stat(path); err == nil && info.Size() == int64(len(pemBytes)) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, ".ca-roots-*")
	if err != nil {
		return "", err
	}
	if _, err := tmp.Write(pemBytes); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return path, os.Rename(tmp.Name(), path)
}
