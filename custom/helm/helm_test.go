package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/helm"
)

func TestStandaloneExplainAndDeps(t *testing.T) {
	cmd := helm.NewHelmCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("helm --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "helm", "github.com/qiangli/yoke/external/helm", "github.com/qiangli/yoke/external/kube")
}
