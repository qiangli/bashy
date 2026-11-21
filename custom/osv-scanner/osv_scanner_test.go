package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/osvscanner"
)

func TestStandaloneExplainAndDeps(t *testing.T) {
	cmd := osvscanner.NewOsvScannerCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("osv-scanner --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "osv-scanner", "github.com/qiangli/yoke/external/osvscanner")
}
