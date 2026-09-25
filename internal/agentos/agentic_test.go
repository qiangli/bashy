package agentos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/yoke/pkg/bscript"
)

func TestAgenticCommandClassifiesNativeExternalAndScript(t *testing.T) {
	if _, kind, yields := agenticCommand([]string{"printf", "ok"}); kind != bscript.Command || !yields {
		t.Fatalf("native: kind=%q yields=%v", kind, yields)
	}
	if cmd, kind, yields := agenticCommand([]string{"definitely-not-a-bashy-command"}); kind != bscript.Command || yields || cmd.Path == bashySelfPath() {
		t.Fatalf("external: path=%q kind=%q yields=%v", cmd.Path, kind, yields)
	}
	script := filepath.Join(t.TempDir(), "broken.sh")
	if err := os.WriteFile(script, []byte("#!/usr/bin/env bashy\nfalse\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, kind, yields := agenticCommand([]string{script}); kind != bscript.Script || !yields {
		t.Fatalf("script: kind=%q yields=%v", kind, yields)
	}
}

// When bashy runs embedded in another program, shims must not re-enter the
// host's CLI: os.Executable() is the host there, so the self path resolves
// to BASHY_SELF, then bashy on PATH, never the host binary.
func TestBashySelfPathPrefersExplicitSelf(t *testing.T) {
	t.Setenv("BASHY_SELF", "/opt/bashy/bin/bashy")
	if got := bashySelfPath(); got != "/opt/bashy/bin/bashy" {
		t.Fatalf("bashySelfPath() = %q, want BASHY_SELF", got)
	}
}

func TestIsBashyExecutable(t *testing.T) {
	for path, want := range map[string]bool{
		"/usr/local/bin/bashy": true, "/x/bashy.real": true, "/b/bashy.exe": true,
		"/usr/local/bin/ycode": false, "/x/agent-mini": false, "/x/bashyish": false,
	} {
		if got := isBashyExecutable(path); got != want {
			t.Errorf("isBashyExecutable(%q) = %v, want %v", path, got, want)
		}
	}
}
