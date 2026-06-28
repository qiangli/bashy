// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines || windows

package agentos

import (
	"fmt"
	"os"
)

// dispatchEngine (default lean build, and ALL Windows builds) reports that the
// container/LLM engines are not compiled in. podman (btrfs/devmapper cgo) and
// ollama (Apple MLX) are heavy unix-host features pulled in only by an explicit
// host build (`-tags bashy_engines` on a unix host). The default worker is a
// self-contained shell + git + dag + `bashy go` + userland that cross-compiles
// everywhere with CGO_ENABLED=0.
func dispatchEngine(arg string) {
	switch arg {
	case "podman", "ollama":
		fmt.Fprintf(os.Stderr,
			"bashy %s: the container/LLM engines are not in this build "+
				"(rebuild with -tags bashy_engines on a unix host, or run them on a host node in the mesh)\n", arg)
		os.Exit(1)
	}
}
