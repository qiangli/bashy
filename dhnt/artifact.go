package dhnt

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

const treeDigestDomain = "dhnt.sha256-tree/v1\x00"

type treeEntry struct {
	path   string
	digest [sha256.Size]byte
}

// HashArtifact hashes one local filesystem artifact according to the closed v2
// artifact contract. It rejects symlinks observed during traversal, but it is
// not a hostile concurrent-filesystem boundary; callers needing that guarantee
// must provide an opened, quiescent workspace such as ExecuteTask's boundary.
func HashArtifact(name string, kind ArtifactKind, algorithm DigestAlgorithm) (string, error) {
	switch {
	case kind == ArtifactFile && algorithm == DigestSHA256FileV1:
		return hashRegularFile(name)
	case kind == ArtifactTree && algorithm == DigestSHA256TreeV1:
		return hashTree(name)
	default:
		return "", fmt.Errorf("unsupported artifact kind/digest combination %q/%q", kind, algorithm)
	}
}

func hashRegularFile(name string) (string, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%q: file artifact must be a regular file", name)
	}
	file, err := os.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return "", err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(info, openedInfo) {
		return "", fmt.Errorf("%q: file changed identity while being opened", name)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashTree(root string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q: tree artifact must be a directory", root)
	}
	var entries []treeEntry
	err = filepath.WalkDir(root, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if name == root {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if err := validateTreePath(relative); err != nil {
			return fmt.Errorf("%q: %w", name, err)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil // Empty directories and directory modes are excluded.
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("%q: tree artifacts reject symlinks and special files", name)
		}
		digest, err := digestRegularFileBytes(name, info)
		if err != nil {
			return err
		}
		entries = append(entries, treeEntry{path: relative, digest: digest})
		return nil
	})
	if err != nil {
		return "", err
	}
	return canonicalTreeDigest(entries)
}

func digestRegularFileBytes(name string, expected fs.FileInfo) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	file, err := os.Open(name)
	if err != nil {
		return digest, err
	}
	defer file.Close()
	openedInfo, err := file.Stat()
	if err != nil {
		return digest, err
	}
	if !openedInfo.Mode().IsRegular() || !os.SameFile(expected, openedInfo) {
		return digest, fmt.Errorf("%q: tree entry changed identity while being opened", name)
	}
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return digest, err
	}
	copy(digest[:], hash.Sum(nil))
	return digest, nil
}

func validateTreePath(value string) error {
	if value == "" || value == "." || strings.HasPrefix(value, "/") || strings.Contains(value, `\`) {
		return errors.New("tree entry path must be non-empty slash-relative")
	}
	if !utf8.ValidString(value) {
		return errors.New("tree entry path must be valid UTF-8")
	}
	if !norm.NFC.IsNormalString(value) {
		return errors.New("tree entry path must be Unicode NFC")
	}
	if path.Clean(value) != value || value == ".." || strings.HasPrefix(value, "../") {
		return errors.New("tree entry path must be clean and must not traverse")
	}
	return nil
}

func canonicalTreeDigest(entries []treeEntry) (string, error) {
	entries = append([]treeEntry(nil), entries...)
	for _, entry := range entries {
		if err := validateTreePath(entry.path); err != nil {
			return "", fmt.Errorf("tree entry %q: %w", entry.path, err)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	for i := 1; i < len(entries); i++ {
		if entries[i-1].path == entries[i].path {
			return "", fmt.Errorf("duplicate tree entry path %q", entries[i].path)
		}
	}
	hash := sha256.New()
	_, _ = io.WriteString(hash, treeDigestDomain)
	writeUint64(hash, uint64(len(entries)))
	for _, entry := range entries {
		writeUint64(hash, uint64(len(entry.path)))
		_, _ = io.WriteString(hash, entry.path)
		_, _ = hash.Write(entry.digest[:])
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeUint64(w io.Writer, value uint64) {
	var encoded [8]byte
	binary.BigEndian.PutUint64(encoded[:], value)
	_, _ = w.Write(encoded[:])
}
