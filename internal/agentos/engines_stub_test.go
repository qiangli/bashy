// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines

package agentos

import (
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
