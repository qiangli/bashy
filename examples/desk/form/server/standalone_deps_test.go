package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestStandaloneDeps proves this command builds alone: with GOWORK=off its
// dependency closure holds no other bashy command package, no sibling bashy
// module, and no yoke/coreutils command package. Anything it needs from
// another command goes through `bashy <verb>` at the process boundary.
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
			// A nested example module may use its own packages; everything
			// else under github.com/qiangli/bashy/ is another bashy package.
			if module != strings.TrimSuffix(bashy, "/") && (pkg == module || strings.HasPrefix(pkg, module+"/")) {
				continue
			}
			if pkg == bashy+"internal/transcribecontract" || pkg == bashy+"internal/transcribeseed" {
				continue // the shared stage contract, not a command package
			}
			if strings.HasPrefix(pkg, bashy+"examples/desk/kit") || strings.HasPrefix(pkg, bashy+"examples/desk/chart/fence") {
				continue // the shared desk libraries, required at a published version
			}
			t.Errorf("links another bashy package: %s", pkg)
		case strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/internal/"):
			t.Errorf("links a yoke command package: %s", pkg)
		case strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/internal/"):
			t.Errorf("links a coreutils command package: %s", pkg)
		}
	}
}
