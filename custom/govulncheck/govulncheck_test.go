package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/custom/internal/adapter"
	"github.com/qiangli/yoke/external/govulncheck"
)

func TestStandaloneExplainAndDeps(t *testing.T) {
	cmd := govulncheck.NewGovulncheckCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("govulncheck --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	// Frozen yoke/external/govulncheck imports external/gotoolchain solely as a
	// library helper (gotoolchain.Ensure) to ensure `go` is on PATH.
	adapter.AssertStandaloneDeps(t, "govulncheck", "github.com/qiangli/yoke/external/govulncheck", "github.com/qiangli/yoke/external/gotoolchain")
}
