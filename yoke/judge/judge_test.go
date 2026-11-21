package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/judge"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := judge.NewJudgeCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy judge:", nil)
	if code != 0 {
		t.Fatalf("judge --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("judge --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "judge")
}
