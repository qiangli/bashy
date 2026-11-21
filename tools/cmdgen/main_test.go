// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCmdgenCLIVerbSubsetAndOutDir(t *testing.T) {
	dir := t.TempDir()
	catDir := filepath.Join(dir, "catalog")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catDir, "stats.yaml"), []byte(`name: stats
kind: cobra
module: github.com/qiangli/yoke
version: v0.0.0-test
package: github.com/qiangli/yoke/pkg/stats
constructor: NewCmd
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(catDir, "tz.yaml"), []byte(`name: tz
kind: tool
module: github.com/qiangli/yoke
version: v0.0.0-test
package: github.com/qiangli/yoke/cmds/tz
`), 0o644); err != nil {
		t.Fatal(err)
	}
	verbsFile := filepath.Join(dir, "batch.txt")
	if err := os.WriteFile(verbsFile, []byte("# P4 worker subset\nstats\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "out")
	if err := run([]string{"-catalog", catDir, "-verbs-file", verbsFile, "-out", outDir}); err != nil {
		t.Fatalf("run: %v", err)
	}
	mainBytes, err := os.ReadFile(filepath.Join(outDir, "stats", "main.go"))
	if err != nil {
		t.Fatalf("missing generated main.go: %v", err)
	}
	if !strings.Contains(string(mainBytes), `cmdpkg "github.com/qiangli/yoke/pkg/stats"`) {
		t.Fatalf("unexpected main.go:\n%s", mainBytes)
	}
	if _, err := os.Stat(filepath.Join(outDir, "tz")); !os.IsNotExist(err) {
		t.Fatalf("unselected verb tz should not have been generated")
	}
}
