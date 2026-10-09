// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

//go:build !windows

package agentos

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTestBinaryReExecAsShellDoesNotRunSuite is the regression guard for the
// recursion that hung `go test ./internal/agentos`: a bashy self-exec path
// (probeShell's `<shell> -c "echo ok"`, hasDAGTarget's `<self> dag …`, …) can
// resolve os.Executable() back to this test binary. If TestMain ran m.Run()
// for that child it re-ran the whole suite, which re-triggered the self-exec
// and forked until the host died. TestMain must instead detect the foreign
// (non -test.*) argv and act as a trivial echo shell.
func TestTestBinaryReExecAsShellDoesNotRunSuite(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-c", "echo ok")
	// Clear the env gates so this exercises the arg-based backstop, not the
	// toolFenceEchoEnv / supervisord dispatches.
	cmd.Env = append(os.Environ(), toolFenceEchoEnv+"=", "BASHY_SUPERVISORD_TEST_CHILD=")
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("re-exec as `-c echo ok` did not terminate (recursion?); output:\n%s", out)
	}
	if err != nil {
		t.Fatalf("re-exec exited non-zero: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("re-exec did not satisfy an `echo ok` probe: %q", out)
	}
	// A child that ran the suite would emit the testing framework's own
	// markers. None may appear.
	for _, marker := range []string{"=== RUN", "--- PASS", "--- FAIL", "\nPASS\n", "\nFAIL\n", "\nok  \t"} {
		if strings.Contains(string(out), marker) {
			t.Fatalf("re-exec appears to have run the test suite (saw %q):\n%s", marker, out)
		}
	}
}

// snapshotDir records a directory's regular files by content so a poisoning
// that rewrites with the same mtime is still caught.
func snapshotDir(dir string) map[string][]byte {
	snap := map[string][]byte{}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return snap
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(dir, e.Name())); err == nil {
			snap[e.Name()] = b
		}
	}
	return snap
}

// TestShimWritesAreIsolatedFromRealHome proves the story's goal: an
// install-agent shim write lands in the isolated /tmp dir TestMain created
// (via BASHY_SHIM_DIR), never in the developer's real ~/.bashy/shims.
func TestShimWritesAreIsolatedFromRealHome(t *testing.T) {
	if testIsolatedShimDir == "" {
		t.Skip("isolation not installed (run under go test)")
	}
	if !strings.HasPrefix(testIsolatedShimDir, "/tmp/") {
		t.Fatalf("isolated shim dir not under /tmp: %q", testIsolatedShimDir)
	}
	if got := shimDir(); got != testIsolatedShimDir {
		t.Fatalf("shimDir() = %q, want isolated %q", got, testIsolatedShimDir)
	}

	// Resolve the real shim dir TestMain shadowed.
	realShimDir := ""
	switch {
	case testRealShimSet && testRealShimDir != "":
		realShimDir = testRealShimDir
	case testRealHomeSet && testRealHome != "":
		realShimDir = filepath.Join(testRealHome, ".bashy", "shims")
	default:
		t.Skip("no distinct real shim dir to protect")
	}
	if realShimDir == testIsolatedShimDir {
		t.Fatalf("real shim dir equals isolated dir; isolation not distinct")
	}
	before := snapshotDir(realShimDir)

	// A fake, non-.test target so WriteShellShim accepts it.
	target := filepath.Join(t.TempDir(), "fake-bashy")
	if err := os.WriteFile(target, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, agent := range []string{"gemini", "copilot", "agy"} {
		if _, err := shimInstaller(agent).install(target, false); err != nil {
			t.Fatalf("shimInstaller(%q).install: %v", agent, err)
		}
	}
	if _, err := codexInstaller(false).install(target, false); err != nil {
		t.Fatalf("codexInstaller.install: %v", err)
	}

	// The write landed in the isolated dir.
	for _, name := range shimNames {
		b, err := os.ReadFile(filepath.Join(testIsolatedShimDir, name))
		if err != nil {
			t.Fatalf("isolated shim %q missing after install: %v", name, err)
		}
		if !bytes.Contains(b, []byte(target)) {
			t.Fatalf("isolated shim %q does not point at target: %q", name, b)
		}
	}
	// The real dir is byte-for-byte unchanged.
	after := snapshotDir(realShimDir)
	if len(before) != len(after) {
		t.Fatalf("real shim dir %q changed: %d entries before, %d after", realShimDir, len(before), len(after))
	}
	for name, b := range before {
		if !bytes.Equal(after[name], b) {
			t.Fatalf("real shim %q content changed under isolated install", name)
		}
	}
}
