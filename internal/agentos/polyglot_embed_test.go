package agentos

import (
	"os"
	"path/filepath"
	"testing"
)

// newFenceEmbedFixtureDir keeps embed fixtures on the package's volume. On
// Windows, t.TempDir may be on a different drive than the checkout, making
// filepath.Rel unable to produce a path the parser can resolve.
func newFenceEmbedFixtureDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp(".", "embedfixture-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("remove embed fixture directory: %v", err)
		}
	})
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(cwd, dir)
}

func fenceEmbedRelPath(t *testing.T, path string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(cwd, path)
	if err != nil {
		t.Fatal(err)
	}
	return filepath.ToSlash(rel)
}
