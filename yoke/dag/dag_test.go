package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/dag"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := dag.NewDagCmd()
	dag.AddCapacityCommands(cmd, dag.CapacityServices{})
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy dag:", nil)
	if code != 0 {
		t.Fatalf("dag --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("dag --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "dag")
}
