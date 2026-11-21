package main

import (
	"bytes"
	"testing"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	"github.com/qiangli/yoke/pkg/chat"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := chat.NewChatCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--help"}, "bashy chat:", nil)
	if code != 0 {
		t.Fatalf("chat --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("chat --help produced empty stdout")
	}
	adapter.AssertStandaloneDeps(t, "chat")
}
