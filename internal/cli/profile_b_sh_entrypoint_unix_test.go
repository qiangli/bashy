// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build unix

package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestProfileBShUtilityEntrypointContract drives the installed shape of the
// Profile B shell: cmd/bash reached through an executable named "sh". This is
// deliberately a process-level test. Package-only runner tests cannot prove
// argv[0] selection or the utility's -c operand, environment, stream, and exit
// status contract together.
func TestProfileBShUtilityEntrypointContract(t *testing.T) {
	bin := builtBashBin(t)
	dir := t.TempDir()
	sh := filepath.Join(dir, "sh")
	if err := os.Symlink(bin, sh); err != nil {
		t.Fatal(err)
	}

	const script = `printf 'zero=%s one=%s env=%s\n' "$0" "$1" "$ENTRY_ENV"
IFS= read -r line
printf 'stdin=%s\n' "$line"
printf 'diag=%s\n' "$2" >&2
exit 23`
	cmd := exec.Command(sh, "-c", script, "command-name", "operand-one", "operand-two")
	cmd.Env = []string{"PATH=/bin:/usr/bin", "HOME=" + dir, "ENTRY_ENV=from-environment"}
	cmd.Stdin = bytes.NewBufferString("from-standard-input\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 23 {
		t.Fatalf("sh exit error = %v, want status 23", err)
	}
	if got, want := stdout.String(), "zero=command-name one=operand-one env=from-environment\nstdin=from-standard-input\n"; got != want {
		t.Fatalf("sh stdout = %q, want %q", got, want)
	}
	if got, want := stderr.String(), "diag=operand-two\n"; got != want {
		t.Fatalf("sh stderr = %q, want %q", got, want)
	}
}
