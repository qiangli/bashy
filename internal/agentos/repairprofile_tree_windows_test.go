// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestRepairTreeWindowsResumeAndReap(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "-test.run=^$")
	reap, err := startRepairTree(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer reap()
	if err := cmd.Wait(); err != nil {
		t.Fatalf("suspended child did not resume and finish: %v", err)
	}
	if err := reap(); err != nil {
		t.Fatal(err)
	}
	if err := reap(); err != nil {
		t.Fatalf("repeated reap: %v", err)
	}
}

func TestRepairTreeWindowsCancellationDuringStart(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		cmd := exec.CommandContext(ctx, exe, "-test.run=^TestRepairTreeWindowsSleepingChild$")
		cmd.Env = append(os.Environ(), "BASHY_REPAIR_TREE_CHILD=1")
		reap, startErr := startRepairTree(cmd)
		if startErr == nil {
			_ = cmd.Wait()
		}
		if err := reap(); err != nil {
			t.Fatal(err)
		}
		cancel()
	}
}

func TestRepairTreeWindowsSleepingChild(t *testing.T) {
	if os.Getenv("BASHY_REPAIR_TREE_CHILD") != "1" {
		return
	}
	time.Sleep(time.Minute)
	os.Exit(0)
}

func TestRepairTreeWindowsFailedStartReap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(t.TempDir(), "missing.exe"))
	reap, err := startRepairTree(cmd)
	if err == nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("missing executable launched")
	}
	if err := reap(); err != nil {
		t.Fatal(err)
	}
}
