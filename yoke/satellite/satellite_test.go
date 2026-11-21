package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/fleet"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := fleet.NewSatelliteCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy satellite:", fleet.ExitCode); code != 0 || out.Len() == 0 {
		t.Fatalf("satellite --help code=%d out=%q", code, out.String())
	}
	adapter.AssertStandaloneDeps(t, "satellite")
}
