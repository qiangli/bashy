package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestStandaloneDeps proves this helper builds alone with GOWORK=off and links
// no other bashy, yoke, or coreutils command packages.
func TestStandaloneDeps(t *testing.T) {
	list := func(args ...string) []string {
		cmd := exec.Command("go", append([]string{"list"}, args...)...)
		cmd.Env = append(os.Environ(), "GOWORK=off")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("GOWORK=off go list %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.Fields(string(out))
	}
	const bashy = "github.com/qiangli/bashy/"
	self := list("-f", "{{.ImportPath}}", ".")[0]
	module := list("-m")[0]
	for _, pkg := range list("-deps", ".") {
		switch {
		case pkg == self, strings.HasPrefix(pkg, self+"/"):
			continue
		case strings.HasPrefix(pkg, bashy):
			if module != strings.TrimSuffix(bashy, "/") && (pkg == module || strings.HasPrefix(pkg, module+"/")) {
				continue
			}
			t.Errorf("links another bashy package: %s", pkg)
		case strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/internal/"):
			t.Errorf("links a yoke command package: %s", pkg)
		case strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/internal/"):
			t.Errorf("links a coreutils command package: %s", pkg)
		}
	}
}
