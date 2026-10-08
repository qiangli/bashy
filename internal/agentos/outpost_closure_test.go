package agentos

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// bashy pins outpost only as a go.mod tool directive (the release builds
// outpost at that version); the bashy binary must never link outpost or its
// yamux transport. go list resolves the pinned graph CI builds (GOWORK=off).
func TestBashyBinaryDoesNotLinkOutpost(t *testing.T) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("no go toolchain on PATH")
	}
	root, ok := findBashySourceRoot(mustWd(t))
	if !ok {
		t.Skip("not in a bashy source checkout")
	}
	cmd := exec.Command(goBin, "list", "-deps", "./cmd/bashy")
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if strings.HasPrefix(pkg, "github.com/qiangli/outpost") || strings.Contains(pkg, "yamux") {
			t.Errorf("bashy links %s", pkg)
		}
	}
}

func mustWd(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return wd
}
