package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/kopia"
)

func TestStandaloneExplainAndDeps(t *testing.T) {
	cmd := kopia.NewKopiaCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("kopia --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "kopia", "github.com/qiangli/yoke/external/kopia")
}
