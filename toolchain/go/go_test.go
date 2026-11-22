package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/bashy/toolchain/internal/adapter"
	"github.com/qiangli/yoke/external/gotoolchain"
)

func TestStandaloneHelpAndDeps(t *testing.T) {
	cmd := gotoolchain.NewGoCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	code := adapter.RunCobraArgs(cmd, []string{"--explain"}, "", nil)
	if code != 0 || out.Len() == 0 {
		t.Fatalf("go --explain exited %d: out=%q err=%q", code, out.String(), errBuf.String())
	}
	adapter.AssertStandaloneDeps(t, "go", "github.com/qiangli/yoke/external/gotoolchain")
}

func TestGoAdapterStripsBashySelfFromTestChild(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go executable unavailable")
	}
	gorootOut, err := exec.Command(goBin, "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	realGoroot := strings.TrimSpace(string(gorootOut))
	if realGoroot == "" {
		realGoroot = runtime.GOROOT()
	}

	binCache := t.TempDir()
	const ver = "99.0.0-test"
	stageParent := filepath.Join(binCache, "go", ver)
	if err := os.MkdirAll(stageParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realGoroot, filepath.Join(stageParent, "go")); err != nil {
		t.Skipf("symlink GOROOT unavailable: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageParent, ".binmgr-tree-complete"), []byte("ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	modDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(modDir, "go.mod"), []byte("module example.com/envleakprobe\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	testSrc := `package envleakprobe

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestChildSeesNoBashySelf(t *testing.T) {
	for _, k := range []string{
		"BASHY_SELF",
		"BASHY_ADAPTER_SELF",
		"BASHY_ADAPTER_EXEC",
		"BASHY_ADAPTER_INPROC",
		"BASHY_INVOKED_AS",
	} {
		if v, ok := os.LookupEnv(k); ok {
			t.Fatalf("adapter env %s=%q leaked into go test child", k, v)
		}
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	base := strings.TrimSuffix(filepath.Base(exe), ".exe")
	if !strings.HasSuffix(base, ".test") {
		t.Fatalf("expected .test executable, got %q", exe)
	}
}
`
	if err := os.WriteFile(filepath.Join(modDir, "leak_test.go"), []byte(testSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BASHY_BIN_CACHE", binCache)
	t.Setenv("BASHY_GO_VERSION", ver)
	t.Setenv("GOWORK", "off")
	t.Setenv("GOTOOLCHAIN", "local")
	t.Setenv("BASHY_SELF", "/fake/bin/bashy")
	t.Setenv("BASHY_ADAPTER_SELF", "/fake/bin/bashy")
	t.Setenv("BASHY_ADAPTER_EXEC", "1")
	t.Setenv("BASHY_ADAPTER_INPROC", "1")
	t.Setenv("BASHY_INVOKED_AS", "go")
	t.Chdir(modDir)

	code := adapter.RunCobraArgs(gotoolchain.NewGoCmd(), []string{"test", "-count=1", "."}, "", nil)
	if code != 0 {
		t.Fatalf("toolchain/go test child failed with exit code %d", code)
	}
}
