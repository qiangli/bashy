package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/kubectl"
)

func TestStandaloneExplainAndDeps(t *testing.T) {
	cmd := kubectl.NewKubectlCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("kubectl --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "kubectl", "github.com/qiangli/yoke/external/kubectl", "github.com/qiangli/yoke/external/kube")
}
