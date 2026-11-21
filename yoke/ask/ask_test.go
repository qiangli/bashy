package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/ask"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := ask.NewAskCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if code := adapter.RunCobraArgs(cmd, []string{"--help"}, "", nil); code != 0 || out.Len() == 0 {
		t.Fatalf("ask --help code=%d out=%q", code, out.String())
	}
	adapter.AssertStandaloneDeps(t, "ask")
}
