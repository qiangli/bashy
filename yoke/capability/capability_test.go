package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/capability"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := capability.NewCapabilityCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy capability:", nil)
	if code != 0 {
		t.Fatalf("capability --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("capability --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "capability")
}
