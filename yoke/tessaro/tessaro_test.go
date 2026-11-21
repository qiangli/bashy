package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/external/tessaro"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := tessaro.NewTessaroCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "", nil)
	if code != 0 {
		t.Fatalf("tessaro --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("tessaro --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "tessaro", "github.com/qiangli/yoke/external/tessaro", "github.com/qiangli/yoke/external/meshagent")
}
