// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build windows

package agentos

import (
	"fmt"
	"os"
)

// dispatchEngine on Windows reports that the container/LLM engines are not
// available in this build. podman (btrfs/devmapper cgo storage drivers) and
// ollama (Apple MLX) are unix-host features; a Windows bashy is a self-contained
// shell + git + dag + userland worker, not a container/LLM host. The rest of
// AgentOS is fully functional here.
func dispatchEngine(arg string) {
	switch arg {
	case "podman", "ollama":
		fmt.Fprintf(os.Stderr,
			"bashy %s: the container/LLM engines are not available on Windows builds "+
				"(unix host feature); run them on a unix node in the mesh\n", arg)
		os.Exit(1)
	}
}
