package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/resources"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := resources.NewCommand()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy resource:", nil)
	if code != 0 {
		t.Fatalf("resource --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("resource --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "resource")
}
