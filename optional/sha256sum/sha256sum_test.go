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
	code := toolrun.Run("sha256sum", []string{"--help"}, tool.Stdio{
		In:  strings.NewReader(""),
		Out: &out,
		Err: &errBuf,
	})
	if code != 0 {
		t.Fatalf("sha256sum --help exited %d: stderr=%s", code, errBuf.String())
	}
	if out.Len() == 0 {
		t.Fatalf("sha256sum --help produced empty stdout")
	}
	toolrun.AssertStandaloneDeps(t, "sha256sum", "github.com/qiangli/coreutils/cmds/sha256sum")
}
