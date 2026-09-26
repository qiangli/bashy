//go:build linux

package main

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"testing"
)

// The exported bundle is real PEM a child tool can load, written once per
// content hash.
func TestWriteFallbackCABundle(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	path, err := writeFallbackCABundle()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(data) {
		t.Fatal("bundle is not loadable PEM")
	}
	n := 0
	for rest := data; ; n++ {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			break
		}
	}
	if n < 100 {
		t.Fatalf("only %d roots exported", n)
	}
	again, err := writeFallbackCABundle()
	if err != nil || again != path {
		t.Fatalf("second write: %q, %v (want the same file)", again, err)
	}
}

// A caller's own setting always wins.
func TestExportFallbackRespectsCaller(t *testing.T) {
	t.Setenv("SSL_CERT_FILE", "/caller/bundle.pem")
	exportFallbackCABundle()
	if got := os.Getenv("SSL_CERT_FILE"); got != "/caller/bundle.pem" {
		t.Fatalf("SSL_CERT_FILE overridden: %q", got)
	}
}
