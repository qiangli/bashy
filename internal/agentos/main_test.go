package agentos

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates the registered-command ring for the WHOLE package: the
// catalog, listing, atlas and dispatch tests read it through
// registeredLookup/registeredNames, and a developer's real
// ~/.config/bashy/commands must never change what those tests see. Tests that
// need a populated ring point BASHY_COMMANDS_DIR at their own temp dir.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bashy-agentos-commands-")
	if err != nil {
		panic(err)
	}
	os.Setenv("BASHY_COMMANDS_DIR", filepath.Join(dir, "commands"))
	os.Setenv("BASHY_COMMANDS_PATH", "")
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
