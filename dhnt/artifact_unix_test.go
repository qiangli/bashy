//go:build !windows

package dhnt

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestTreeRejectsSpecialFile(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "fifo")
	if err := syscall.Mkfifo(name, 0o600); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	if _, err := HashArtifact(root, ArtifactTree, DigestSHA256TreeV1); err == nil ||
		!strings.Contains(err.Error(), "special files") {
		t.Fatalf("got %v, want special-file rejection", err)
	}
}

func TestTreeRejectsInvalidUTF8FilesystemName(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, string([]byte{'b', 'a', 'd', 0xff}))
	if err := os.WriteFile(name, []byte("contents"), 0o600); err != nil {
		t.Skipf("filesystem does not permit invalid UTF-8 names: %v", err)
	}
	if _, err := HashArtifact(root, ArtifactTree, DigestSHA256TreeV1); err == nil ||
		!strings.Contains(err.Error(), "valid UTF-8") {
		t.Fatalf("got %v, want invalid UTF-8 rejection", err)
	}
}
