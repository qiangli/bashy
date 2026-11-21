package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/curlbin"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	if got := mapNullDevice("windows", []string{"-o", "/dev/null", "--output=/dev/null"}); got[1] != "NUL" || got[2] != "--output=NUL" {
		t.Fatalf("mapNullDevice = %v", got)
	}
	cmd := curlbin.NewCurlCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("curl --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "curl", "github.com/qiangli/yoke/external/curlbin")
}
