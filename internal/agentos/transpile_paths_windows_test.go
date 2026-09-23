//go:build windows

package agentos

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"mvdan.cc/sh/v3/pathconv"
)

// Story 676: the shell publishes /c/... paths on Windows, and a normal Bashy
// script operand already accepts them. Exercise the real AgentOS dispatcher so
// transpile's input, output, and explicit map paths keep the same contract.
func TestWindowsTranspileMSYSPathOperands(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "source.bsh")
	output := filepath.Join(dir, "generated.go")
	mapFile := filepath.Join(dir, "generated.map")
	if err := os.WriteFile(input, []byte("var x int = 10\necho hi\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	args, err := json.Marshal([]string{
		"bashy", "transpile", "--bashsharp", pathconv.FromOS(input),
		"-o", pathconv.FromOS(output), "--map", pathconv.FromOS(mapFile),
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestTranspileRegisteredCLIDispatch$")
	cmd.Env = append(os.Environ(),
		"GO_WANT_HELPER_PROCESS=1",
		"TEST_OS_ARGS_JSON="+string(args),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("MSYS-path transpile failed: %v\n%s", err, out)
	}
	for _, path := range []string{output, mapFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected transpile artifact %s: %v", path, err)
		}
	}
}
