package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/weave"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := weave.NewWeaveCmd()
	configureWeaveResourceAdmission(cmd)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy weave:", nil)
	if code != 0 {
		t.Fatalf("weave --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("weave --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "weave")
}
