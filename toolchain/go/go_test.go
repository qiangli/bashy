package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/gotoolchain"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := gotoolchain.NewGoCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("go --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "go", "github.com/qiangli/yoke/external/gotoolchain")
}
