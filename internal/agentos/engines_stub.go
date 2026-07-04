// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

//go:build !bashy_engines || (windows && (!remote || !containers_image_openpgp))

package agentos

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
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

// provisionEngine fetches a bashy-built, permissive engine blob from bashy's own
// release into the binmgr cache and returns its path — the self-contained Tier-2
// rung. Best-effort: returns "" (→ the install/mesh message) when no such asset
// exists yet or the fetch fails, so there is never a regression. For podman it
// also pulls the VM helpers (vfkit/gvproxy) it needs on macOS.
func provisionEngine(name string) string {
	cb, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	binDir := filepath.Join(cb, "bashy", "bin")
	if name == "podman" && runtime.GOOS == "darwin" {
		for _, helper := range []string{"gvproxy", "vfkit"} {
			if !isExecutable(filepath.Join(binDir, helper)) {
				_, _ = fetchReleaseBlob(helper, binDir) // best-effort; podman machine init needs them
			}
		}
	}
	dest, err := fetchReleaseBlob(name, binDir)
	if err != nil {
		return ""
	}
	return dest
}

// fetchReleaseBlob downloads <name>-<goos>-<goarch>.gz from the latest bashy
// release, gunzips it into binDir/<name>, marks it executable, and returns the
// path. Self-contained (net/http + gzip); no third-party dependency.
func fetchReleaseBlob(name, binDir string) (string, error) {
	asset := fmt.Sprintf("%s-%s-%s.gz", name, runtime.GOOS, runtime.GOARCH)
	url := latestReleaseAssetURL("qiangli/bashy", asset)
	if url == "" {
		return "", fmt.Errorf("no release asset %s", asset)
	}
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", err
	}
	fmt.Fprintf(os.Stderr, "bashy %s: fetching bashy's permissive %s (built from source, gitignored cache)\n", name, name)
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	dest := filepath.Join(binDir, name)
	tmp := dest + ".partial"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, gz); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	f.Close()
	if err := os.Rename(tmp, dest); err != nil { // atomic install
		os.Remove(tmp)
		return "", err
	}
	return dest, nil
}

func latestReleaseAssetURL(repo, assetName string) string {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var rel struct {
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return ""
	}
	for _, a := range rel.Assets {
		if a.Name == assetName {
			return a.URL
		}
	}
	return ""
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
