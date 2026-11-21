package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/bashy/custom/internal/toolshortcut"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := toolshortcut.DispatchToolShortcut("muse", []string{"--help"}, nil, &out, &errBuf)
	if code != 0 || !strings.Contains(out.String(), "session:  muse") {
		t.Fatalf("muse --help: code=%d out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "muse", "github.com/qiangli/bashy/custom/internal/toolshortcut")
}
