// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines || (windows && (!remote || !containers_image_openpgp))

package agentos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/coreutils/pkg/binmgr"
)

// dispatchEngine (lean build). The container/LLM engines are not LINKED into this
// binary — by design (docs/bashy-execution-path.md: "exec, never link"). But a
// command listed in `bashy commands` must still work with no rebuild, so we walk
// the dispatch ladder:
//
//	Tier 3  host $PATH or bashy's binmgr cache — exec transparently.
//	Tier 2  provision: fetch bashy's own permissive engine blob (built from source
//	        in CI, published as <name>-<goos>-<goarch>.gz on bashy's release —
//	        docs/licensing-supply-chain-policy.md), gunzip → cache → exec.
//	Tier 4  none available → point at install or a paired host node — never a
//	        rebuild.
func dispatchEngine(arg string) {
	switch arg {
	case "podman", "ollama":
		bin := resolveEngineBinary(arg)
		if bin == "" {
			bin = provisionEngine(arg)
		}
		if bin != "" {
			os.Exit(execEnginePassthrough(bin, os.Args[2:]))
		}
		fmt.Fprint(os.Stderr, engineNotFoundMessage(arg))
		os.Exit(127)
	}
}

// provisionEngine resolves a missing engine via the SHARED binmgr pipeline
// (cache → fetch bashy's permissive blob → build from source) — the same process
// outpost and Tessaro apps use, standalone (no paired host assumed). Returns "" so
// dispatch falls to the install/mesh message when nothing is available.
func provisionEngine(name string) string {
	path, err := binmgr.ProvisionManaged(context.Background(), engineSpec(name))
	if err != nil {
		return ""
	}
	return path
}

// engineSpec builds the ManagedSpec for a bashy engine: fetch its permissive blob
// from bashy's release into bashy's engine cache. podman on macOS also provisions
// the vfkit/gvproxy VM helpers it needs. (A Build hook — build from source when no
// blob exists — is added per engine as those recipes are wired.)
func engineSpec(name string) binmgr.ManagedSpec {
	spec := binmgr.ManagedSpec{
		Name:        name,
		DestDir:     engineCacheDir(),
		ReleaseRepo: engineReleaseRepo(),
		Log:         func(m string) { fmt.Fprintln(os.Stderr, m) },
	}
	if name == "podman" && runtime.GOOS == "darwin" {
		spec.Deps = []binmgr.ManagedSpec{
			{Name: "gvproxy", ReleaseRepo: spec.ReleaseRepo},
			{Name: "vfkit", ReleaseRepo: spec.ReleaseRepo},
		}
	}
	return spec
}

// engineReleaseRepo is the repo whose release carries bashy's permissive engine
// blobs. Overridable via $BASHY_ENGINE_REPO for forks/mirrors.
func engineReleaseRepo() string {
	if r := strings.TrimSpace(os.Getenv("BASHY_ENGINE_REPO")); r != "" {
		return r
	}
	return "qiangli/bashy"
}

// resolveEngineBinary finds a usable engine binary — the host $PATH first (Tier 3),
// then bashy's binmgr cache (where managed downloads land) — while never resolving
// back to this bashy binary (the bare-name shim would recurse).
func resolveEngineBinary(name string) string {
	self, _ := os.Executable()
	if p, err := exec.LookPath(name); err == nil && p != "" && !sameFile(p, self) {
		return p
	}
	if dir := engineCacheDir(); dir != "" {
		if cand := filepath.Join(dir, name); isExecutable(cand) {
			return cand
		}
	}
	return ""
}

// engineCacheDir is bashy's managed-binary cache — $BASHY_BIN_CACHE if set (as
// binmgr honors it), else <UserCacheDir>/bashy/bin.
func engineCacheDir() string {
	if d := strings.TrimSpace(os.Getenv("BASHY_BIN_CACHE")); d != "" {
		return d
	}
	cb, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(cb, "bashy", "bin")
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
