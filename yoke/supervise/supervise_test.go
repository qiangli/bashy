package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/supervise"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := supervise.NewSuperviseCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy supervise:", nil)
	if code != 0 {
		t.Fatalf("supervise --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("supervise --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "supervise")
}
