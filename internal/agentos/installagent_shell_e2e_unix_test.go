//go:build e2e && !windows

package agentos

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/qiangli/yoke/pkg/execlog"
)

func TestE2EInstallAgentShimRecordsExecution(t *testing.T) {
	bin := bashyBinary(t)
	t.Setenv("HOME", t.TempDir())
	for _, name := range []string{"codex", "agy", "gemini", "copilot"} {
		t.Run(name, func(t *testing.T) {
			ins := shimInstaller(name)
			if name == "codex" {
				ins = codexInstaller(false)
			}
			if _, err := ins.install(bin, false); err != nil {
				t.Fatal(err)
			}
			root := t.TempDir()
			t.Setenv("BASHY_EXECHIST", root)
			t.Setenv("BASHY_AGENTIC", "1")
			cmd := exec.Command(filepath.Join(shimDir(), "bash"), "-c", "seq 3791527 3791527")
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("shell: %v\n%s", err, out)
			}
			records, _, err := execlog.Read(root, execlog.Query{Cmd: "seq"})
			if err != nil {
				t.Fatal(err)
			}
			if len(records) != 1 || records[0].Exit == nil || *records[0].Exit != 0 || !records[0].Observed {
				t.Fatalf("expected one observed successful seq execution; got %+v", records)
			}
		})
	}
}
