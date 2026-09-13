package agentos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/coreutils/pkg/bscript"
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
