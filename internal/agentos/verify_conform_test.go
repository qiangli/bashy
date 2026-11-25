package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dhnt/conform"
)

// TestConformSuiteSubcommandsShipInProcess pins that `bashy conform` carries the
// dhnt/conform suite at the go.mod version, next to bashy's own batteries.
func TestConformSuiteSubcommandsShipInProcess(t *testing.T) {
	var out bytes.Buffer
	cmd := verifyCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != conform.Version {
		t.Fatalf("bashy conform version = %q, want %q", got, conform.Version)
	}

	dir := t.TempDir()
	contract := "### C-01\nEcho a fixed line.\n```console\n$ echo hello\nhello\n```\nexit: 0\n"
	if err := os.WriteFile(filepath.Join(dir, "CONTRACT.md"), []byte(contract), 0o644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	cmd = verifyCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"run", filepath.Join(dir, "CONTRACT.md"), "--dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "PASS C-01") {
		t.Fatalf("bashy conform run output = %q", out.String())
	}

	for _, s := range suiteRegistry() {
		for _, name := range append([]string{s.Name}, s.Aliases...) {
			switch name {
			case "run", "cert", "mutate", "version":
				t.Fatalf("battery %q collides with a conform suite subcommand", name)
			}
		}
	}
}
