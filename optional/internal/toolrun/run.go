// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package toolrun

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/tool"
)

func InvokedAs(def string) string {
	if v := strings.TrimSpace(os.Getenv("BASHY_INVOKED_AS")); v != "" {
		return v
	}
	return def
}

func Main(def string) {
	os.Exit(Run(InvokedAs(def), os.Args[1:], tool.Stdio{
		In:  os.Stdin,
		Out: os.Stdout,
		Err: os.Stderr,
	}))
}

func Run(name string, args []string, stdio tool.Stdio) int {
	t := tool.Lookup(name)
	if t == nil {
		fmt.Fprintf(stdio.Err, "bashy: %s: No such command\n", name)
		return 127
	}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stdio.Err, "bashy: %s: %v\n", name, err)
		return 1
	}
	rc := &tool.RunContext{
		Ctx:   context.Background(),
		Dir:   dir,
		Env:   os.Environ(),
		FS:    tool.NewLocalFS(),
		Stdio: stdio,
	}
	return t.Run(rc, args)
}

// AssertStandaloneDeps verifies the command builds/lists alone under GOWORK=off
// and links no other command's package.
func AssertStandaloneDeps(t *testing.T, selfEntry string, allowedCmdPkgs ...string) {
	t.Helper()
	allow := map[string]bool{
		"github.com/qiangli/bashy/optional/" + selfEntry:        true,
		"github.com/qiangli/bashy/optional/internal/toolrun":    true,
	}
	for _, p := range allowedCmdPkgs {
		allow[p] = true
	}
	cmd := exec.Command("go", "list", "-deps", ".")
	cmd.Env = append(os.Environ(), "GOWORK=off")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GOWORK=off go list -deps .: %v\n%s", err, out)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if allow[pkg] {
			continue
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/bashy/") {
			t.Errorf("%s links another bashy package: %s", selfEntry, pkg)
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/yoke/cmds/internal/") {
			t.Errorf("%s links another yoke command package: %s", selfEntry, pkg)
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/") && !strings.HasPrefix(pkg, "github.com/qiangli/coreutils/cmds/internal/") {
			t.Errorf("%s links another coreutils command package: %s", selfEntry, pkg)
		}
		if strings.HasPrefix(pkg, "github.com/qiangli/yoke/external/") {
			t.Errorf("%s links external command package: %s", selfEntry, pkg)
		}
	}
}
