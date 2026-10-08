// Package installpair verifies the two supported installed shell shapes.
package installpair

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// Runner executes a binary with arguments and returns its standard output.
type Runner func(exe string, args ...string) ([]byte, error)

// RefuseCompanion enforces bashy's one-file release contract.
func RefuseCompanion(base string) error {
	companion := base + ".real"
	if _, err := os.Lstat(companion); err == nil {
		return fmt.Errorf("refusing two-file build: %s exists; bashy is one-file", companion)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect %s: %w", companion, err)
	}
	return nil
}

// VerifyOptional accepts a plain Go binary when base.real is absent. When the
// payload exists, both files must be distinct regular files with the same
// --version build identity.
func VerifyOptional(base string, run Runner) error {
	payload := base + ".real"
	if _, err := os.Lstat(payload); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect %s: %w", payload, err)
	}
	baseInfo, err := regularFile(base)
	if err != nil {
		return err
	}
	payloadInfo, err := regularFile(payload)
	if err != nil {
		return err
	}
	if os.SameFile(baseInfo, payloadInfo) {
		return fmt.Errorf("%s and %s are not distinct regular files", base, payload)
	}
	equal, err := sameBytes(base, payload)
	if err != nil {
		return err
	}
	if equal {
		return fmt.Errorf("%s is a byte copy of its payload %s, not a native launcher", base, payload)
	}
	launcherVersion, err := run(base, "--version")
	if err != nil {
		return fmt.Errorf("probe launcher %s --version: %w", base, err)
	}
	payloadVersion, err := run(payload, "--version")
	if err != nil {
		return fmt.Errorf("probe payload %s --version: %w", payload, err)
	}
	if strings.TrimSpace(string(launcherVersion)) != strings.TrimSpace(string(payloadVersion)) {
		return fmt.Errorf("launcher and payload build identity differ: %s != %s", strings.TrimSpace(string(launcherVersion)), strings.TrimSpace(string(payloadVersion)))
	}
	return nil
}

func regularFile(path string) (os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", path)
	}
	return info, nil
}

func sameBytes(a, b string) (bool, error) {
	aSum, err := fileSum(a)
	if err != nil {
		return false, err
	}
	bSum, err := fileSum(b)
	if err != nil {
		return false, err
	}
	return aSum == bSum, nil
}

func fileSum(path string) ([sha256.Size]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return [sha256.Size]byte{}, err
	}
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum, nil
}
