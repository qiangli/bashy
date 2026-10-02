package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtensionCommandsCertExclusion(t *testing.T) {
	t.Setenv("BASHY_FLEET_DIR", t.TempDir())
	t.Setenv("VSC_PROFILE", "cert")
	if code := dispatchCommands([]string{"language", "show", "probe"}); code != 2 {
		t.Fatalf("cert extension lookup exit = %d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("BASHY_FLEET_DIR"), "languages")); !os.IsNotExist(err) {
		t.Fatalf("cert path touched extension ring: %v", err)
	}
	t.Setenv("VSC_PROFILE", "")
	if code := dispatchCommands([]string{"language", "add", "python", "--set", "fences=python"}); code == 0 {
		t.Fatal("builtin language collision was accepted")
	}
	if code := dispatchCommands([]string{"toolchain", "add", "go", "--set", "effects=pure"}); code == 0 {
		t.Fatal("base toolchain collision was accepted")
	}
	if entries, err := os.ReadDir(os.Getenv("BASHY_FLEET_DIR")); err == nil && len(entries) != 0 {
		t.Fatalf("refused extension wrote files: %v", entries)
	} else if err != nil && !os.IsNotExist(err) && !strings.Contains(err.Error(), "no such file") {
		t.Fatal(err)
	}
}
