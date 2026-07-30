package dhnt

import (
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileArtifactDigestIsRawSHA256(t *testing.T) {
	root := t.TempDir()
	name := filepath.Join(root, "artifact")
	if err := os.WriteFile(name, []byte("file bytes\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := HashArtifact(name, ArtifactFile, DigestSHA256FileV1)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256([]byte("file bytes\n"))
	if got != hexDigest(want) {
		t.Fatalf("got %s, want raw file sha256 %s", got, hexDigest(want))
	}
}

func TestTreeDigestOrderModesAndEmptyDirectories(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	writeTreeFile(t, first, "b/item", "two", 0o600)
	writeTreeFile(t, first, "a", "one", 0o755)
	if err := os.Mkdir(filepath.Join(first, "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTreeFile(t, second, "a", "one", 0o600)
	writeTreeFile(t, second, "b/item", "two", 0o777)

	a, err := HashArtifact(first, ArtifactTree, DigestSHA256TreeV1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashArtifact(second, ArtifactTree, DigestSHA256TreeV1)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("order, modes, or empty directories changed tree digest: %s != %s", a, b)
	}
}

func TestTreeDigestTamperingChangesIdentity(t *testing.T) {
	root := t.TempDir()
	writeTreeFile(t, root, "data", "before", 0o600)
	before, err := HashArtifact(root, ArtifactTree, DigestSHA256TreeV1)
	if err != nil {
		t.Fatal(err)
	}
	writeTreeFile(t, root, "data", "after", 0o600)
	after, err := HashArtifact(root, ArtifactTree, DigestSHA256TreeV1)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("tampered content retained the same tree identity")
	}
}

func TestTreeDigestEmptyTreeIsStableAndDomainSeparated(t *testing.T) {
	a, err := HashArtifact(t.TempDir(), ArtifactTree, DigestSHA256TreeV1)
	if err != nil {
		t.Fatal(err)
	}
	b, err := HashArtifact(t.TempDir(), ArtifactTree, DigestSHA256TreeV1)
	if err != nil {
		t.Fatal(err)
	}
	rawEmpty := sha256.Sum256(nil)
	if a != b || a == hexDigest(rawEmpty) {
		t.Fatalf("empty tree digest is not stable and domain-separated: %s / %s", a, b)
	}
	const golden = "f73fabe81939be81294fcbcbf9523441bf4236cfbcf782144f0e638631bc79f4"
	if a != golden {
		t.Fatalf("empty tree encoding changed: got %s, want %s", a, golden)
	}
}

func TestTreeRejectsSymlinksAndSpecialFiles(t *testing.T) {
	t.Run("symlink", func(t *testing.T) {
		root := t.TempDir()
		writeTreeFile(t, root, "target", "bytes", 0o600)
		if err := os.Symlink("target", filepath.Join(root, "link")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := HashArtifact(root, ArtifactTree, DigestSHA256TreeV1); err == nil ||
			!strings.Contains(err.Error(), "reject symlinks") {
			t.Fatalf("got %v, want symlink rejection", err)
		}
	})
	t.Run("fifo", func(t *testing.T) {
		// Portable Go has no FIFO constructor. A directory passed as a file
		// still proves the closed regular-file rule on every platform.
		if _, err := HashArtifact(t.TempDir(), ArtifactFile, DigestSHA256FileV1); err == nil {
			t.Fatal("non-regular file artifact was accepted")
		}
	})
}

func TestCanonicalTreeEncodingRejectsNonPortablePaths(t *testing.T) {
	digest := sha256.Sum256([]byte("x"))
	for _, value := range []string{"../escape", "a/../b", "/absolute", `a\b`, "e\u0301"} {
		t.Run(value, func(t *testing.T) {
			if _, err := canonicalTreeDigest([]treeEntry{{path: value, digest: digest}}); err == nil {
				t.Fatalf("accepted non-portable tree path %q", value)
			}
		})
	}
	if _, err := canonicalTreeDigest([]treeEntry{{path: "same", digest: digest}, {path: "same", digest: digest}}); err == nil {
		t.Fatal("accepted duplicate tree path")
	}
}

func TestCanonicalTreeEncodingIsLengthDelimited(t *testing.T) {
	a := sha256.Sum256([]byte("a"))
	b := sha256.Sum256([]byte("b"))
	first, err := canonicalTreeDigest([]treeEntry{{path: "a", digest: a}, {path: "bc", digest: b}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := canonicalTreeDigest([]treeEntry{{path: "ab", digest: a}, {path: "c", digest: b}})
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("path boundaries were ambiguous")
	}
}

func writeTreeFile(t *testing.T, root, relative, contents string, mode os.FileMode) {
	t.Helper()
	name := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(contents), mode); err != nil {
		t.Fatal(err)
	}
}

func hexDigest(value [sha256.Size]byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, sha256.Size*2)
	for i, b := range value {
		out[i*2] = digits[b>>4]
		out[i*2+1] = digits[b&15]
	}
	return string(out)
}
