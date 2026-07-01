// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func TestDispatchCoreutilsToolFetchHelp(t *testing.T) {
	var out, err bytes.Buffer
	code := dispatchCoreutilsTool("fetch", []string{"--help"}, tool.Stdio{
		Out: &out,
		Err: &err,
	})
	if code != 0 {
		t.Fatalf("fetch --help exit = %d, stderr = %q", code, err.String())
	}
	if !strings.Contains(out.String(), "Usage: fetch") {
		t.Fatalf("fetch help missing usage:\n%s", out.String())
	}
}

func TestDispatchCoreutilsToolUnknown(t *testing.T) {
	var err bytes.Buffer
	code := dispatchCoreutilsTool("__missing__", nil, tool.Stdio{Err: &err})
	if code != 127 {
		t.Fatalf("missing tool exit = %d, stderr = %q", code, err.String())
	}
	if !strings.Contains(err.String(), "No such command") {
		t.Fatalf("missing tool stderr should explain failure, got %q", err.String())
	}
}
