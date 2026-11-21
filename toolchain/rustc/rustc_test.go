package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/rust"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := rust.NewRustcCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 {
		t.Fatalf("rustc --explain exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("rustc --explain produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "rustc", "github.com/qiangli/yoke/external/rust")
}
