package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/mirror"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := mirror.NewMirrorCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "", nil)
	if code != 0 {
		t.Fatalf("mirror --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("mirror --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "mirror", "github.com/qiangli/yoke/external/rclone")
}
