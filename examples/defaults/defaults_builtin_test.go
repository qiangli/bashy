package bashydefaults

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A default command must be registerable by the product binary. This catches
// collisions with embedded front doors, which the fake bootstrap registry
// cannot see (transcribe was one such collision).
func TestDefaultCommandNamesDoNotCollideWithBuiltins(t *testing.T) {
	source, err := os.ReadFile(filepath.Join(moduleRoot(t), "examples", "defaults", "bashy-defaults.bsh"))
	if err != nil {
		t.Fatal(err)
	}
	marker := "DEFAULT_COMMANDS=\""
	at := strings.Index(string(source), marker)
	if at < 0 {
		t.Fatal("DEFAULT_COMMANDS is missing")
	}
	body := string(source[at+len(marker):])
	body = strings.SplitN(body, "\"\n", 2)[0]
	ring := t.TempDir()
	for _, line := range strings.Split(body, "\n") {
		name, _, ok := strings.Cut(line, "|")
		if !ok || name == "" {
			continue
		}
		c := exec.Command(bashyBin(t), "commands", "add", name, "--set", "exec.0=bashy", "--set", "exec.1=--version")
		c.Env = append(os.Environ(), "BASHY_COMMANDS_DIR="+filepath.Join(ring, "commands"), "BASHY_FLEET_DIR="+filepath.Join(ring, "fleet"), "BASHY_TOOLS_DIR="+filepath.Join(ring, "fleet", "tools"), "BASHY_TOOLS_PATH=", "BASHY_FLEET_SEEDS=off", "BASHY_HINTS=off")
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("default %s collides with a built-in: %v\n%s", name, err, out)
		}
	}
}
