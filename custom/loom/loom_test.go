package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/loom"
)

func TestStandaloneExplainAndDeps(t *testing.T) {
	cmd := loom.NewLoomCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("loom --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	// Frozen yoke/external/loom imports external/gitscm solely as a library helper
	// (gitscm.Ensure) to locate MinGit for Gitea hooks.
	adapter.AssertStandaloneDeps(t, "loom", "github.com/qiangli/yoke/external/loom", "github.com/qiangli/yoke/external/gitscm")
}
