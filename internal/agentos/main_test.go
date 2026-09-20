package agentos

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMain isolates the registered-command ring and the compiled host's
// attest sink for the WHOLE package: the catalog, listing, atlas and dispatch
// tests read the ring through registeredLookup/registeredNames, and a
// decorated call run through the compiled (shellrt) path resolves its attest
// sink lazily on first use (compiledAttestSink, attest.go) — both read a
// developer's real ~/.config/bashy directory unless redirected here first.
// Tests that need a populated ring, or want to assert on receipts, point
// BASHY_COMMANDS_DIR / BASHY_SKILLS_DIR at their own temp dir.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "bashy-agentos-commands-")
	if err != nil {
		panic(err)
	}
	os.Setenv("BASHY_COMMANDS_DIR", filepath.Join(dir, "commands"))
	os.Setenv("BASHY_COMMANDS_PATH", "")
	os.Setenv("BASHY_SKILLS_DIR", filepath.Join(dir, "skills"))
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
