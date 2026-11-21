package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/broker"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := broker.NewCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy llm:", nil)
	if code != 0 {
		t.Fatalf("llm --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("llm --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "llm")
}
