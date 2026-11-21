package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/fleet"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := fleet.NewAdaptersCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy adapter:", nil)
	if code != 0 {
		t.Fatalf("adapter --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("adapter --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "adapter")
}
