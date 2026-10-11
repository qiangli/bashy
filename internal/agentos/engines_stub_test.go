// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines

package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEngineFallbackMessageNoRebuild pins the fix: a command listed in
// `bashy commands` must never tell the user to rebuild bashy — the settled ladder
// falls back to a PATH/cached engine (Tier 3) or a paired host node (Tier 4).
func TestEngineFallbackMessageNoRebuild(t *testing.T) {
	for _, arg := range []string{"podman", "ollama"} {
		msg := strings.ToLower(engineNotFoundMessage(arg))
		for _, bad := range []string{"-tags", "bashy_engines", "rebuild", "not in this build"} {
			if strings.Contains(msg, bad) {
				t.Errorf("%s message must not mention %q: %q", arg, bad, msg)
			}
		}
		if !strings.Contains(msg, "install") || !strings.Contains(msg, "host node") {
			t.Errorf("%s message should point to install or a host node: %q", arg, msg)
		}
	}
}

func TestResolveEngineBinaryUnknown(t *testing.T) {
	if got := resolveEngineBinary("definitely-not-an-engine-xyz"); got != "" {
		t.Errorf("unknown engine should resolve to empty, got %q", got)
	}
}

func TestCachedOllamaUsesOnlyPinnedVersion(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("BASHY_BIN_CACHE", cache)
	entry := ollamaEntrypoint()
	if entry == "" {
		t.Skip("no official Ollama release asset on this platform")
	}
	newer := filepath.Join(cache, "ollama", "v999.0.0", filepath.FromSlash(entry))
	if err := os.MkdirAll(filepath.Dir(newer), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("newer"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := cachedOllama(ollamaPinnedVersion, entry); got != "" {
		t.Fatalf("unpinned cache was selected: %q", got)
	}
	pinned := filepath.Join(cache, "ollama", ollamaPinnedVersion, filepath.FromSlash(entry))
	if err := os.MkdirAll(filepath.Dir(pinned), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pinned, []byte("pinned"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := cachedOllama(ollamaPinnedVersion, entry); got != pinned {
		t.Fatalf("pinned cache = %q, want %q", got, pinned)
	}
}
