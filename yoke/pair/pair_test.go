package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/pair"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := pair.NewPairCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy pair:", nil)
	if code != 0 {
		t.Fatalf("pair --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("pair --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "pair")
}
