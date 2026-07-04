// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines || (windows && (!remote || !containers_image_openpgp))

package agentos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// dispatchEngine (lean build). The container/LLM engines are not LINKED into this
// binary — by design (docs/bashy-execution-path.md: "exec, never link"). But a
// command listed in `bashy commands` must still work with no rebuild, so instead
// of erroring we continue down the dispatch ladder to Tier 3: resolve a host or
// bashy-cached `podman`/`ollama` and exec it transparently. Only when none exists
// do we say how to get one (install it, or run it on a paired host node — never
// "rebuild bashy").
func dispatchEngine(arg string) {
	switch arg {
	case "podman", "ollama":
		if bin := resolveEngineBinary(arg); bin != "" {
			os.Exit(execEnginePassthrough(bin, os.Args[2:]))
		}
		fmt.Fprint(os.Stderr, engineNotFoundMessage(arg))
		os.Exit(127)
	}
}

// resolveEngineBinary finds a usable engine binary — the host $PATH first (Tier 3),
// then bashy's binmgr cache (where managed downloads land) — while never resolving
// back to this bashy binary (the bare-name shim would recurse).
func resolveEngineBinary(name string) string {
	self, _ := os.Executable()
	if p, err := exec.LookPath(name); err == nil && p != "" && !sameFile(p, self) {
		return p
	}
	if cb, err := os.UserCacheDir(); err == nil {
		if cand := filepath.Join(cb, "bashy", "bin", name); isExecutable(cand) {
			return cand
		}
	}
	return ""
}

func sameFile(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	fa, ea := os.Stat(a)
	fb, eb := os.Stat(b)
	return ea == nil && eb == nil && os.SameFile(fa, fb)
}

func execEnginePassthrough(bin string, args []string) int {
	cmd := exec.Command(bin, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", bin, err)
		return 1
	}
	return 0
}

func engineNotFoundMessage(arg string) string {
	switch arg {
	case "podman":
		return "bashy podman: no podman found on PATH or in bashy's cache.\n" +
			"  Install it (e.g. `brew install podman` then `podman machine init && podman machine start`),\n" +
			"  or run containers on a paired host node over the mesh.\n"
	default: // ollama
		return "bashy ollama: no ollama found on PATH or in bashy's cache.\n" +
			"  Install it (https://ollama.com/download), or run it on a paired host node over the mesh.\n"
	}
}
