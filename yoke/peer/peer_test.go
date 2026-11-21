package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/external/sphere"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := sphere.NewSphereCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "", nil)
	if code != 0 {
		t.Fatalf("peer --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("peer --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "peer", "github.com/qiangli/yoke/external/sphere", "github.com/qiangli/yoke/external/meshagent")
}
