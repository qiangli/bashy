package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/accounts"
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := accounts.NewAccountCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "", nil)
	if code != 0 {
		t.Fatalf("account --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("account --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "account")
}
