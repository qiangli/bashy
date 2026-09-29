package cli

// Sprint: #323; Story: #1139; Story-ID: 954b8b79aac5

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A capitalised first word must not reach a lower-case executable through a
// case-insensitive file system: on macOS "Read TASK.md and do the task."
// stat'ed /usr/bin/Read (= /usr/bin/read), ran as a command, and the agent
// never saw the prompt (agent-bench l4/t1-pivot on genie, 2026-09-28).
func TestOnPathExactCase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows command names are case-insensitive by design")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "read"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !onPath("read", dir) {
		t.Fatalf("onPath(read) = false, want true")
	}
	if onPath("Read", dir) {
		t.Fatalf("onPath(Read) = true: a case-insensitive match is not a command on this OS")
	}
}
