package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/handoff"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := handoff.NewResumeCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy resume:", nil)
	if code != 0 {
		t.Fatalf("resume --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("resume --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "resume")
}
