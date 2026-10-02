//go:build bashy_core

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoreImportBoundary(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-tags", "bashy_core", "./cmd/bashy")
	cmd.Dir = filepath.Join("..", "..")
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list core imports: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		for _, forbidden := range []string{
			"github.com/qiangli/bashy/internal/agentos",
			"github.com/qiangli/ycode",
			"github.com/qiangli/filebrowser",
			"github.com/qiangli/yoke/pkg/telemetry",
			"modernc.org/sqlite",
		} {
			if pkg == forbidden || strings.HasPrefix(pkg, forbidden+"/") {
				t.Errorf("core import graph contains optional package %s", pkg)
			}
		}
	}
}
