package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/bus"
	"github.com/qiangli/yoke/pkg/weave"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	wireMessageBoard()
	cmd := bus.NewMessageBoardCmdWithSprintResolver(weave.ResolveSprintScopeByHandle)
	wrapAuthoredClaimErrors(cmd)
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy mb:", nil)
	if code != 0 {
		t.Fatalf("mb --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("mb --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "mb")
}
