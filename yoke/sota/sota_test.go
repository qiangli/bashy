package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/sota"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := sota.NewSotaCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "", nil)
	if code != 0 {
		t.Fatalf("sota --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("sota --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "sota")
}
