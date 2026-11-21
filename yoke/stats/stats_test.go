package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/stats"
)

func TestStatsStandaloneParity(t *testing.T) {
	cmd := stats.NewCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetIn(strings.NewReader("{\"instance_id\":\"i1\",\"agent\":\"a1\",\"resolved\":true,\"cost_usd\":0.10}\n"))
	code := adapter.RunCobraArgs(cmd, []string{"summary", "--arm", "agent", "--json"}, "bashy stats:", nil)
	if code != 0 {
		t.Fatalf("RunCobraArgs exited %d: stderr=%s", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "\"bashy-stats-summary-v1\"") || !strings.Contains(out.String(), "\"a1\"") {
		t.Fatalf("unexpected summary output: %s", out.String())
	}
	adapter.AssertStandaloneDeps(t, "stats")
}
