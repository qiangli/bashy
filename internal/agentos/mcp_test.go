// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestMCPUsageNoArgs(t *testing.T) {
	if got := dispatchMCP(nil); got != 2 {
		t.Fatalf("dispatchMCP(nil) = %d, want 2", got)
	}
	if got := dispatchMCP([]string{"--help"}); got != 2 {
		t.Fatalf("dispatchMCP(--help) = %d, want 2", got)
	}
}

func TestMCPTransportHTTPUnsupported(t *testing.T) {
	out := captureMCPStderr(t, func() {
		if got := dispatchMCP([]string{"serve", "--transport", "http"}); got != 2 {
			t.Fatalf("dispatchMCP(serve --transport http) = %d, want 2", got)
		}
	})
	if !strings.Contains(out, "bashy mcp: --transport http is not yet supported") {
		t.Fatalf("stderr = %q, want the exact --transport http message", out)
	}
}

// The yoke mcp package owns stdio round-trip coverage; a full client
// handshake here would duplicate it. dispatchMCP's stdio path is a thin
// call to yokemcp.ServeStdio, so no goroutine/pipe probe is kept here.

// captureMCPStderr runs fn with os.Stderr replaced by a pipe and returns
// everything fn wrote to it.
func captureMCPStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()
	fn()
	_ = w.Close()
	os.Stderr = old
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
