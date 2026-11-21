package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/coreutils/pkg/schedule"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := schedule.NewScheduleCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "", nil)
	if code != 0 {
		t.Fatalf("schedule --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("schedule --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "schedule")
}
