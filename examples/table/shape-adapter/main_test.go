package main

import (
	"path/filepath"
	"testing"
)

func TestRuntimeSourceRequiresPrivatePrincipalData(t *testing.T) {
	home := t.TempDir()
	good := filepath.Join(home, "shape", "table", "requester", "data")
	got, err := runtimeSource(good)
	if err != nil || got != filepath.Join(home, "table", "runtime") {
		t.Fatalf("runtimeSource(%q) = %q, %v", good, got, err)
	}
	for _, bad := range []string{filepath.Join(home, "table", "data"), filepath.Join(home, "shape", "notes", "requester", "data"), home} {
		if _, err := runtimeSource(bad); err == nil { t.Errorf("accepted %q", bad) }
	}
}
