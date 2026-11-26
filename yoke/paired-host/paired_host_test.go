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
	if code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy paired-host:", fleet.ExitCode); code != 0 || out.Len() == 0 {
		t.Fatalf("paired-host --help code=%d out=%q", code, out.String())
	}
	adapter.AssertStandaloneDeps(t, "paired-host")
}

func TestStandaloneReachabilityCommands(t *testing.T) {
	for _, tc := range []struct {
		command string
		want    string
	}{
		{command: "disk-pressure", want: "paired-host reachability disk-pressure"},
		{command: "status", want: "--satellite"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			cmd := newCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			args := []string{"reachability", tc.command, "--help"}
			if code := adapter.RunCobraArgs(cmd, args, "bashy paired-host:", fleet.ExitCode); code != 0 || !bytes.Contains(out.Bytes(), []byte(tc.want)) {
				t.Fatalf("paired-host %v code=%d out=%q", args, code, out.String())
			}
		})
	}
}
