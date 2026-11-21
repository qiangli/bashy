package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/pwsh"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := pwsh.NewCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 {
		t.Fatalf("pwsh --explain exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("pwsh --explain produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "pwsh", "github.com/qiangli/yoke/external/pwsh")
}
