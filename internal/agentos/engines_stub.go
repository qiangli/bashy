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
	name := engineAlias(arg) // `bashy docker` -> podman engine
	switch name {
	case "podman", "ollama":
		if name == "ollama" {
			if blocked, msg := ollamaCloudGate(os.Args[2:]); blocked {
				fmt.Fprint(os.Stderr, msg)
				os.Exit(2)
			}
		}
		bin := resolveEngineBinary(name)
		if bin == "" {
			bin = provisionEngine(name)
		}
		if bin != "" {
			if name == "ollama" {
				applyOllamaIsolationEnv() // keep the lean ollama isolated like the embedded one
			}
			os.Exit(execEnginePassthrough(bin, os.Args[2:]))
		}
		fmt.Fprint(os.Stderr, engineNotFoundMessage(name))
		os.Exit(127)
	}
}

// provisionEngine resolves a missing engine via the SHARED binmgr pipeline
// (cache → fetch bashy's permissive blob → build from source) — the same process
// outpost and Tessaro apps use, standalone (no paired host assumed). Returns "" so
// dispatch falls to the install/mesh message when nothing is available.
func provisionEngine(name string) string {
	if name == "ollama" {
		// ollama is embedded (cgo) only in the -tags bashy_engines host build.
		// In the lean binary there is no bashy-built ollama blob to fetch; instead
		// pull the OFFICIAL ollama release — MIT-licensed, so download+exec of the
		// permissive upstream binary is allowed (docs/licensing-supply-chain-policy.md;
		// not bundled — a separate process). It ships as a tarball of the ollama
		// binary + its ggml runner libs, so we tree-extract and exec in place.
		return provisionOllama(context.Background())
	}
	path, err := binmgr.ProvisionManaged(context.Background(), engineSpec(name))
	if err != nil {
		return ""
	}
	return path
}

// provisionOllama fetches the official ollama release for this platform (Tree
// extraction: ollama + its sibling ggml runner libs) into bashy's managed cache
// and returns the ollama entrypoint path. Returns "" (→ install/mesh message) on
// any error, or on platforms whose official asset bashy can't yet extract
// (linux ships .tar.zst — zstd, unsupported by binmgr's tar.gz/zip extractor).
func provisionOllama(ctx context.Context) string {
	entry := ollamaEntrypoint()
	if entry == "" {
		return ""
	}
	if p := cachedOllama(entry); p != "" {
		return p // already extracted — no network, no "fetching" line
	}
	t, err := binmgr.ResolveGitHub(ctx, binmgr.GitHubSpec{
		Name:       "ollama",
		Repo:       ollamaOfficialRepo(),
		Version:    "latest",
		Tree:       true,
		Entrypoint: entry,
		AssetMatch: ollamaAssetMatch,
	})
	if err != nil {
		return ""
	}
	fmt.Fprintln(os.Stderr, "bashy ollama: fetching the official ollama runtime ("+t.Version+") — first run only…")
	path, err := binmgr.Ensure(ctx, t)
	if err != nil {
		return ""
	}
	return path
}

// cachedOllama returns an already-extracted ollama entrypoint from binmgr's cache
// (any version, newest wins) so a cache hit needs no GitHub round-trip. Mirrors
// binmgr.Ensure's layout: <CacheDir>/ollama/<version>/<entry>.
func cachedOllama(entry string) string {
	root, err := binmgr.CacheDir()
	if err != nil {
		return ""
	}
	matches, _ := filepath.Glob(filepath.Join(root, "ollama", "*", filepath.FromSlash(entry)))
	best := ""
	var bestMod int64
	for _, m := range matches {
		fi, err := os.Stat(m)
		if err != nil || !isExecutable(m) {
			continue
		}
		if mt := fi.ModTime().UnixNano(); best == "" || mt > bestMod {
			best, bestMod = m, mt
		}
	}
	return best
}

// ollamaOfficialRepo is the upstream ollama release repo; $BASHY_OLLAMA_REPO
// overrides it for forks/mirrors.
func ollamaOfficialRepo() string {
	if r := strings.TrimSpace(os.Getenv("BASHY_OLLAMA_REPO")); r != "" {
		return r
	}
	return "ollama/ollama"
}

// ollamaEntrypoint is the ollama executable's path within the official archive
// for this platform (darwin/windows only; "" = not auto-provisionable here).
func ollamaEntrypoint() string {
	switch runtime.GOOS {
	case "darwin":
		return "ollama"
	case "windows":
		return "ollama.exe"
	default:
		return "" // linux uses .tar.zst — provision via host/mesh instead
	}
}

// ollamaAssetMatch selects the plain (non -rocm/-mlx/-jetpack) official ollama
// asset bashy can extract for this platform: the universal darwin .tgz or the
// per-arch windows .zip. linux (.tar.zst) matches nothing here.
func ollamaAssetMatch(name, goos, goarch string) bool {
	n := strings.ToLower(name)
	switch goos {
	case "darwin":
		return n == "ollama-darwin.tgz"
	case "windows":
		return n == "ollama-windows-"+goarch+".zip"
	}
	return false
}

// applyOllamaIsolationEnv points a freshly-exec'd official ollama at bashy's own
// port + model store (matching the embedded managed daemon: never 11434, models
// under ~/.agents/bashy/ollama) unless the caller set them explicitly — so the
// lean ollama never fights a host ollama.
func applyOllamaIsolationEnv() {
	if strings.TrimSpace(os.Getenv("OLLAMA_HOST")) == "" {
		os.Setenv("OLLAMA_HOST", "127.0.0.1:11435")
	}
	if strings.TrimSpace(os.Getenv("OLLAMA_MODELS")) == "" {
		if home, err := os.UserHomeDir(); err == nil {
			os.Setenv("OLLAMA_MODELS", filepath.Join(home, ".agents", "bashy", "ollama", "models"))
		}
	}
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
		if runtime.GOOS == "linux" {
			return "bashy ollama: no ollama found on PATH or in bashy's cache.\n" +
				"  The official linux ollama ships as .tar.zst (not yet auto-extracted); install it\n" +
				"  (https://ollama.com/download) or run it on a paired host node over the mesh.\n"
		}
		return "bashy ollama: could not fetch the official ollama runtime.\n" +
			"  Check network access to github.com/ollama/ollama releases, install ollama\n" +
			"  (https://ollama.com/download), or run it on a paired host node over the mesh.\n"
	}
}
