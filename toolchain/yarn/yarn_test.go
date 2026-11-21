package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/node"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := node.NewYarnCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 {
		t.Fatalf("yarn --explain exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("yarn --explain produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "yarn", "github.com/qiangli/yoke/external/node")
}
