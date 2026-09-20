//go:build windows

// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// todo:7cdf6c0b — `bashy upgrade install` with no path replaces the bashy
// that is running it. Windows refuses to overwrite a running image, so the
// install must rename it aside and put the new file in its place.
func TestInstallExecutableReplacesRunningImage(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "target.exe")
	cmdExe := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	body, err := os.ReadFile(cmdExe)
	if err != nil {
		t.Skip("cmd.exe unreadable:", err)
	}
	if err := os.WriteFile(dst, body, 0o755); err != nil {
		t.Fatal(err)
	}
	// Keep the target image running: cmd.exe waiting on a stdin we never close.
	run := exec.Command(dst, "/c", "pause")
	stdin, err := run.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := run.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = run.Process.Kill(); _ = stdin.Close(); _, _ = run.Process.Wait() }()

	// A plain overwrite of the running image must fail — that is the bug.
	if err := os.WriteFile(dst, []byte("x"), 0o755); err == nil {
		t.Fatal("expected Windows to refuse writing a running image; the test premise is gone")
	}

	src := filepath.Join(dir, "new.bin")
	if err := os.WriteFile(src, []byte("new binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installExecutable(src, dst); err != nil {
		t.Fatalf("install over the running image: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new binary" {
		t.Fatalf("dst body = %q, want the new binary", got)
	}
	if _, err := os.Stat(dst + ".old"); err != nil {
		t.Fatalf("the running image should have been parked at %s.old: %v", dst, err)
	}
	// A second install while the parked image is still running must park it
	// under a unique name rather than fail on the busy `.old`.
	if err := os.WriteFile(src, []byte("newer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installExecutable(src, dst); err != nil {
		t.Fatalf("second install: %v", err)
	}
	got, _ = os.ReadFile(dst)
	if string(got) != "newer" {
		t.Fatalf("second dst body = %q", got)
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "target.exe.old*"))
	if len(matches) < 1 {
		t.Fatalf("no parked images found: %v", matches)
	}
	for _, tmp := range mustGlob(t, filepath.Join(dir, ".target.exe.tmp-*")) {
		t.Fatalf("temp file left behind: %s", tmp)
	}
}

func mustGlob(t *testing.T, pattern string) []string {
	t.Helper()
	m, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
