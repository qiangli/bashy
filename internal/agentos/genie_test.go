package agentos

// Sprint: #290; Story: #933; Story-ID: 6a0507862385

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeGenieSource(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dag := "# genie — Bashy workflow\n\n## Tasks\n\n### solve\nEffects: read\n\n```bsh\n:\n```\n"
	if err := os.WriteFile(filepath.Join(dir, "dag.md"), []byte(dag), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGenieBundleResolution(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("GENIE_BAR", "")
	if _, err := genieBundle(); err == nil || !strings.Contains(err.Error(), "bashy genie build") {
		t.Fatalf("missing bundle should point at build, got %v", err)
	}
	installed := filepath.Join(home, "genie", "genie.bar")
	if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("bar"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := genieBundle(); err != nil || got != installed {
		t.Fatalf("installed bundle: got %q, %v", got, err)
	}
	explicit := filepath.Join(t.TempDir(), "other.bar")
	if err := os.WriteFile(explicit, []byte("bar"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GENIE_BAR", explicit)
	if got, err := genieBundle(); err != nil || got != explicit {
		t.Fatalf("GENIE_BAR wins: got %q, %v", got, err)
	}
	t.Setenv("GENIE_BAR", filepath.Join(t.TempDir(), "missing.bar"))
	if _, err := genieBundle(); err == nil {
		t.Fatal("a missing GENIE_BAR must be an error, not a fallback")
	}
}

func TestGenieSourceDiscovery(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "ycode", "examples", "genie")
	writeGenieSource(t, source)
	nested := filepath.Join(root, "some", "project")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)
	t.Setenv("GENIE_SOURCE", "")
	got, err := genieSource("")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(source)
	if resolved, _ := filepath.EvalSymlinks(got); resolved != want {
		t.Fatalf("found %q, want %q", got, source)
	}
	if _, err := genieSource(nested); err == nil {
		t.Fatal("--from a directory without a genie dag.md must be refused")
	}
	if got, err := genieSource(source); err != nil || got != source {
		t.Fatalf("--from: got %q, %v", got, err)
	}
}

func TestDispatchGenieUsageAndPull(t *testing.T) {
	if code := dispatchGenie(nil); code != 2 {
		t.Fatalf("no arguments: exit %d, want 2", code)
	}
	if code := dispatchGenie([]string{"pull"}); code != 2 {
		t.Fatalf("pull before a release exists: exit %d, want 2", code)
	}
}
