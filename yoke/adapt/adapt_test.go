package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/adapt"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := adapt.NewCmd(adapt.PrepareJSONLens)
	adapter.DisableCompletion(cmd)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})
	if code := adapt.ExitCode(cmd.Execute()); code != 0 || out.Len() == 0 {
		t.Fatalf("adapt --help code=%d out=%q", code, out.String())
	}
	adapter.AssertStandaloneDeps(t, "adapt")
}
