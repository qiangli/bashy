package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/external/tessaro"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := tessaro.NewLoginCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 {
		t.Fatalf("login --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("login --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "login", "github.com/qiangli/yoke/external/tessaro", "github.com/qiangli/yoke/external/meshagent")
}
