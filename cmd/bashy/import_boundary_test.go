package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// commandFolders are the five folders whose entries are independently built
// commands reached through their Bash# adapters (adapter_dispatch.go). bashy
// runs them across the process boundary; it never links their packages.
var commandFolders = []string{"optional", "toolchain", "yoke", "custom", "examples"}

func isCommandEntry(pkg string) bool {
	for _, folder := range commandFolders {
		prefix := "github.com/qiangli/bashy/" + folder
		if pkg == prefix || strings.HasPrefix(pkg, prefix+"/") {
			return true
		}
	}
	return false
}

// TestBashyImportBoundary is the link proof for the thin core (P5): the
// cmd/bashy dependency graph, on every release OS and build profile, holds no
// package from a command folder. There is no allowlist.
func TestBashyImportBoundary(t *testing.T) {
	for _, tc := range []struct{ goos, tags string }{
		{"linux", ""},
		{"darwin", ""},
		{"windows", ""},
		{"linux", "bashy_scratch"},
		{"linux", "bashy_engines bashy_obs"},
		{"darwin", "bashy_engines bashy_obs"},
	} {
		t.Run(tc.goos+"/"+strings.ReplaceAll(tc.tags, " ", "+"), func(t *testing.T) {
			cmd := exec.Command("go", "list", "-deps", "-tags", tc.tags, ".")
			cmd.Env = append(os.Environ(), "GOOS="+tc.goos, "GOARCH=amd64", "CGO_ENABLED=0")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go list -deps: %v\n%s", err, out)
			}
			required := map[string]bool{
				"github.com/qiangli/bashy/internal/agentos": false,
				"github.com/bashsharp/bashsharp/cli":        false,
				"github.com/qiangli/coreutils/cmds/all":     false,
				"github.com/qiangli/bash/interp":            false,
			}
			for _, pkg := range strings.Fields(string(out)) {
				if _, ok := required[pkg]; ok {
					required[pkg] = true
				}
				if isCommandEntry(pkg) {
					t.Errorf("cmd/bashy links command entry package %s", pkg)
				}
			}
			for pkg, seen := range required {
				if !seen {
					t.Errorf("cmd/bashy does not link %s", pkg)
				}
			}
		})
	}
}
