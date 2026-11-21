package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/secrets"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := secrets.NewSecretsCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if code := adapter.RunCobraArgs(cmd, []string{"--help"}, "secret:", nil); code != 0 || out.Len() == 0 {
		t.Fatalf("secret --help code=%d out=%q", code, out.String())
	}
	adapter.AssertStandaloneDeps(t, "secret")
}
