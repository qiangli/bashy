package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/qiangli/bashy/optional/internal/toolrun"
	"github.com/qiangli/coreutils/tool"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := toolrun.Run("realpath", []string{"--help"}, tool.Stdio{
		In:  strings.NewReader(""),
		Out: &out,
		Err: &errBuf,
	})
	if code != 0 {
		t.Fatalf("realpath --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("realpath --help produced empty stdout")
	}
	toolrun.AssertStandaloneDeps(t, "realpath", "github.com/qiangli/coreutils/cmds/realpath")
}
