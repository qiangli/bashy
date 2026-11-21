package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/gitscm"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	if _, _, err := splitGitExternal([]string{"--external=bad"}); err == nil {
		t.Fatal("expected error for malformed --external")
	}
	cmd := gitscm.NewGitSCMCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("git-scm --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "git-scm", "github.com/qiangli/yoke/external/gitscm")
}
