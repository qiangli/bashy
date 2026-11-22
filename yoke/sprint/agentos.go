package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qiangli/bashy/yoke/internal/adapter"
	_ "github.com/qiangli/yoke/pkg/accounts/builtin"
)

// bashySelfPath is the bashy executable that shims and re-entries run
// (`git() { command <self> git "$@"; }`). It is the running binary only when
// that binary IS bashy: when bashy is embedded as a library (ycode's harness
// session, pkg/runner) os.Executable() is the host, and re-entering the host's
// CLI would hand bashy verbs to a different program. Order: an explicit
// BASHY_SELF from the embedder; the running executable when it is bashy (or a
// Go test binary, which stands in for it in this package's tests); bashy on
// PATH; else the bare name, so a missing bashy fails loudly instead of
// silently re-entering the host.
// RunningAsBashy is set by cmd/bashy: the running executable is a bashy
// build under any file name, so os.Executable() is the right self path.
var RunningAsBashy bool

func bashySelfPath() string {
	if self := adapter.BashySelf(); self != "" {
		return self
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		if len(os.Args) > 0 && os.Args[0] != "" {
			exe = os.Args[0]
		}
	}
	if exe != "" && (RunningAsBashy || isBashyExecutable(exe) || strings.HasSuffix(strings.TrimSuffix(filepath.Base(exe), ".exe"), ".test")) {
		return exe
	}
	if path, err := exec.LookPath("bashy"); err == nil {
		return path
	}
	return "bashy"
}

func isBashyExecutable(path string) bool {
	switch strings.TrimSuffix(strings.ToLower(filepath.Base(path)), ".exe") {
	case "bashy", "bashy.real":
		return true
	}
	return false
}
