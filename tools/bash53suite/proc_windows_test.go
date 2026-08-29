//go:build windows

package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestFixtureGuardFailsClosedOnWindows(t *testing.T) {
	watch, err := armParentDeathWatch(exec.Command("cmd.exe", "/c", "exit", "0"))
	if err == nil {
		t.Fatal("fixture guard unexpectedly permitted an uncontained Windows launch")
	}
	if watch != nil {
		t.Fatal("failed fixture guard returned a live watch")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unsupported") {
		t.Fatalf("fixture guard error = %q, want unsupported diagnostic", err)
	}
}
