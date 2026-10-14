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

func TestLeanOllamaFailsClosed(t *testing.T) {
	msg := engineNotFoundMessage("ollama")
	for _, want := range []string{"bashy_engines", "embed_runner", "embedded signed runner"} {
		if !strings.Contains(msg, want) {
			t.Errorf("Ollama error %q lacks %q", msg, want)
		}
	}
	if got := provisionEngine("ollama"); got != "" {
		t.Errorf("lean build provisioned Ollama: %q", got)
	}
	if probe := probeOllamaEngine(); probe.Binary != "" || probe.Provisionable {
		t.Errorf("lean build reports Ollama available: %+v", probe)
	}
}

func TestResolveEngineBinaryUnknown(t *testing.T) {
	if got := resolveEngineBinary("definitely-not-an-engine-xyz"); got != "" {
		t.Errorf("unknown engine should resolve to empty, got %q", got)
	}
}

func TestLeanOllamaIgnoresExternalBinary(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("BASHY_BIN_CACHE", cache)
	path := filepath.Join(cache, "ollama")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external"), 0o755); err != nil {
		t.Fatal(err)
	}
	if probe := probeOllamaEngine(); probe.Binary != "" || probe.Provisionable {
		t.Errorf("lean build selected external Ollama: %+v", probe)
	}
}
