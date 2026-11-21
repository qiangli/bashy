package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/fleet"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := fleet.NewReproduceCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy reproduce:", fleet.ExitCode); code != 0 || out.Len() == 0 {
		t.Fatalf("reproduce --help code=%d out=%q", code, out.String())
	}
	adapter.AssertStandaloneDeps(t, "reproduce")
}
