package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/qiangli/coreutils/multicall"
	"github.com/qiangli/coreutils/tool"
)

func invocationName(argv0 string) string {
	name := filepath.Base(argv0)
	name = strings.TrimPrefix(name, "-")
	if len(name) > 4 && strings.EqualFold(name[len(name)-4:], ".exe") {
		name = name[:len(name)-4]
	}
	return name
}

func shellInvocation(argv0 string) bool {
	switch invocationName(argv0) {
	case "sh", "bash":
		return true
	}
	return false
}

func utilityInvocation(argv0 string) bool {
	name := invocationName(argv0)
	return name == "coreutils" || (name != "bashy" && !shellInvocation(argv0) && tool.Lookup(name) != nil)
}

// installInheritedSignalIgnores applies the entry snapshot only to shell
// routes. Cgo builds capture it in a pre-Go constructor; Linux pure-Go builds
// read the runtime's ELF snapshot through multicall. The interpreter needs
// these names in BASHY_HARD_IGNORE to keep inherited SIG_IGN immutable.
func installInheritedSignalIgnores() {
	captured := preGoIgnoredSignals()
	if captured == "" {
		captured = strings.Join(multicall.InheritedIgnoredSignalNames(), ",")
	}
	if captured == "" {
		return
	}
	prior := os.Getenv("BASHY_HARD_IGNORE")
	seen := make(map[string]bool)
	var names []string
	for _, name := range strings.Split(prior+","+captured, ",") {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		names = append(names, name)
	}
	_ = os.Setenv("BASHY_HARD_IGNORE", strings.Join(names, ","))
}
