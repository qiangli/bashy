package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/fleet"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := fleet.NewDesktopCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	args := []string{"--help"}
	if "desktop" == "journey" {
		args = []string{"journey", "--help"}
	}
	if code := adapter.RunCobraArgs(cmd, args, "", fleet.DesktopExitCode); code != 0 || out.Len() == 0 {
		t.Fatalf("desktop --help code=%d out=%q", code, out.String())
	}
	adapter.AssertStandaloneDeps(t, "desktop")
}
