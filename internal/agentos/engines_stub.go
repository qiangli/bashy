// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines || (!windows && !cgo && !containers_image_openpgp) || (windows && (!remote || !containers_image_openpgp))

package agentos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/qiangli/yoke/external/podman/winhelper"
	"github.com/qiangli/yoke/pkg/binmgr"
)

// dispatchEngine uses external Podman when the native dependencies are absent.
// Ollama is embedded whenever bashy_engines is set, independently of Podman.
func dispatchEngine(arg string) {
	if code := guardEngine(arg, os.Args[2:]); code != 0 {
		dispatchExit(code)
	}
	name := engineAlias(arg) // `bashy docker` -> podman engine
	switch name {
	case "ollama":
		dispatchOllamaEngine()
	case "podman":
		bin := resolveEngineBinary(name)
		if bin == "" {
			bin = provisionEngine(name)
		}
		if bin != "" {
			applyPodmanHelperEnv()     // Windows: without this a machine start serves NO endpoint
			applyManagedPodmanEnv(bin) // managed podman: its own conmon/crun/netavark (linux), gvproxy/vfkit (darwin)
			dispatchExit(execEnginePassthrough(bin, os.Args[2:]))
		}
		fmt.Fprint(os.Stderr, engineNotFoundMessage(name))
		dispatchExit(127)
	case "dks":
		// Provisioning the rootful DKS VM needs the in-process podman
		// machine libraries, which are only linked in the bashy_engines
		// build. The lean build can't create a VM.
		fmt.Fprintln(os.Stderr, "bashy dks: not available in this build — the DKS VM provisioner "+
			"needs the engine build (`-tags bashy_engines`). On Linux, run k3s under rootful podman directly (no VM).")
		dispatchExit(1)
	}
}

// provisionEngine resolves a missing engine from its pinned upstream release,
// standalone (no paired host assumed). Returns "" so dispatch falls to the
// install/mesh message when nothing is available.
func provisionEngine(name string) string {
	if name == "podman" {
		return provisionPodman(context.Background())
	}
	return ""
}

// applyPodmanHelperEnv provisions podman's Windows helper binaries and
// exports the env that lets podman find them, before we exec it. A no-op
// on every other platform.
//
// This is NOT an optimization. On Windows podman does not serve its API
// itself: `podman machine start` launches win-sshproxy.exe to forward the
// VM's socket onto `\\.\pipe\podman-<machine>` and
// %TEMP%\podman\<machine>-api.sock. It finds that helper only via
// containers.conf's helper_binaries_dir or
// $CONTAINERS_HELPER_BINARY_DIR — and when it finds neither the start
// still reports SUCCESS while publishing no endpoint at all. The VM runs
// and nothing can connect, which every client (including outpost's
// podman detection, and therefore `--cluster-mode=vk-podman`) sees as
// "podman is not installed".
//
// The linked engine build has always done this; the LEAN build — the one
// GoReleaser actually ships for Windows — execs podman instead of linking
// it, and so skipped it. Failure is logged and ignored: a host whose
// containers.conf already names a helper dir works either way.
func applyPodmanHelperEnv() {
	if runtime.GOOS != "windows" {
		return
	}
	if err := winhelper.Apply(engineCacheDir()); err != nil {
		fmt.Fprintf(os.Stderr, "bashy podman: could not provision Windows helper binaries: %v\n", err)
	}
}

// resolveEngineBinary finds a usable engine binary — the host $PATH first (Tier 3),
// then bashy's binmgr cache (where managed downloads land) — while never resolving
// back to this bashy binary (the bare-name shim would recurse).
//
// The cache holds a tool in one of two layouts: FLAT (`<cache>/<name>[.exe]`, a
// single managed blob) or TREE (`<cache>/<name>/<version>/<name>[.exe]`, how
// binmgr's tree-mode Ensure lands the official podman/ollama archives). Check
// both, honor the platform's `.exe` suffix, and take the newest tree version —
// otherwise a host that already has podman cached (e.g. from a prior
// `bashy podman` run) is told "no podman found."
func resolveEngineBinary(name string) string {
	self, _ := os.Executable()
	if p, err := exec.LookPath(name); err == nil && p != "" && !sameFile(p, self) {
		return p
	}
	dir := engineCacheDir()
	if dir == "" {
		return ""
	}
	exe := name + exeSuffix()
	for _, cand := range []string{filepath.Join(dir, exe), filepath.Join(dir, name)} {
		if isExecutable(cand) {
			return cand
		}
	}
	best, bestMod := "", int64(0)
	for _, pat := range []string{filepath.Join(dir, name, "*", exe), filepath.Join(dir, name, "*", name)} {
		matches, _ := filepath.Glob(pat)
		for _, m := range matches {
			fi, err := os.Stat(m)
			if err != nil || !isExecutable(m) {
				continue
			}
			if mt := fi.ModTime().UnixNano(); best == "" || mt > bestMod {
				best, bestMod = m, mt
			}
		}
	}
	return best
}

// exeSuffix is the platform executable suffix (".exe" on Windows).
func exeSuffix() string {
	if runtime.GOOS == "windows" {
		return ".exe"
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

func engineNotFoundMessage(arg string) string {
	switch arg {
	case "podman":
		return "bashy podman: no podman found on PATH or in bashy's cache, and none could be fetched.\n" +
			"  Check network access to github.com releases (bashy fetches a pinned podman for " + binmgr.Platform() + "),\n" +
			"  install podman yourself, or run containers on a paired host node over the mesh.\n"
	default: // ollama
		return "bashy ollama: local inference requires the engines build (-tags bashy_engines,embed_runner) with the embedded signed runner.\n"
	}
}
