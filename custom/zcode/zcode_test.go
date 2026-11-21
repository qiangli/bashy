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
	code := toolshortcut.DispatchToolShortcut("zcode", []string{"--help"}, nil, &out, &errBuf)
	if code != 0 || !strings.Contains(out.String(), "session:  zcode") {
		t.Fatalf("zcode --help: code=%d out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "zcode", "github.com/qiangli/bashy/custom/internal/toolshortcut")
}
