package site

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestStandaloneDeps proves this module (both the root library package and
// ./cmd/server) builds alone with stdlib only: with GOWORK=off its dependency
// closure holds no external module, no other bashy package, and no
// yoke/coreutils command package.
func TestStandaloneDeps(t *testing.T) {
	list := func(args ...string) []string {
		cmd := exec.Command("go", append([]string{"list"}, args...)...)
		cmd.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("GOWORK=off go list %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return strings.Fields(string(out))
	}
	self := list("-m")[0]
	for _, item := range list("-deps", "-f", "{{.ImportPath}}={{.Standard}}", "./...") {
		pkg, std, _ := strings.Cut(item, "=")
		if pkg == self || strings.HasPrefix(pkg, self+"/") {
			continue
		}
		if std != "true" {
			t.Errorf("non-stdlib dependency %s (template must use Go stdlib only)", pkg)
		}
	}
}
