package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/capability"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := capability.NewLeaderboardCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy leaderboard:", nil)
	if code != 0 {
		t.Fatalf("leaderboard --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("leaderboard --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "leaderboard")
}
