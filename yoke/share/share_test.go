package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/fleet"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := newCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy share:", fleet.ExitCode); code != 0 || out.Len() == 0 {
		t.Fatalf("share --help code=%d out=%q", code, out.String())
	}
	adapter.AssertStandaloneDeps(t, "share")
}
