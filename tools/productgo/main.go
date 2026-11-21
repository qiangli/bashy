// Command productgo builds Bashy with its explicitly opted-in startup signal
// contract. The installed Go source tree is read-only; unknown sources fail.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

const toolchainVersion = "go1.27.4"
const sourceHash = "a9dd82ee8b8089ec69dc90cc8a2d3553839db41b3f01807311507c7fc117a3e0"
const insertion = `
	// Bashy's product entry preserves inherited ignores for ordinary async
	// signals before any Go code runs. Keep faults and runtime-owned signals
	// on the stock path. initsig records the ignored bit; Notify can still
	// install a handler, and Stop returns to the inherited ignore.
	if bashyInheritedIgnore == "1" && !isarchive && !islibrary &&
		atomic.Loaduintptr(&fwdSig[sig]) == _SIG_IGN &&
		t.flags&_SigNotify != 0 && t.flags&(_SigPanic|_SigUnblock|_SigSetStack) == 0 &&
		sig != sigPreempt && sig != _SIGPROF && sig != sigPerThreadSyscall {
		return false
	}
`
const helpers = `
// Default off. Only the Bashy product build links this immutable value to "1".
var bashyInheritedIgnore string

// bashyInheritedIgnoredSignals reports the caller's inherited SIG_IGN
// dispositions as a bitmask (bit n is signal n), without cgo. initsig snapshots
// runtime-managed signals in fwdSig before installing handlers; _SigDefault
// signals keep their inherited action until Notify, so read those live.
//
//go:linkname bashyInheritedIgnoredSignals
func bashyInheritedIgnoredSignals() uint64 {
	var mask uint64
	for i := uint32(1); i < _NSIG && i < 64; i++ {
		t := &sigtable[i]
		if t.flags == 0 {
			continue
		}
		if t.flags&_SigDefault != 0 {
			if atomic.Load(&handlingSig[i]) == 0 && getsig(i) == _SIG_IGN {
				mask |= 1 << i
			}
			continue
		}
		if atomic.Loaduintptr(&fwdSig[i]) == _SIG_IGN {
			mask |= 1 << i
		}
	}
	return mask
}
`

// snapshotTag selects cmd/bashy's pure-Go reader of the overlay accessor.
// Builds without the overlay never see the tag, so they never link it.
const snapshotTag = "bashy_runtime_snapshot"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "productgo:", err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) < 5 {
		return fmt.Errorf("usage: productgo GOOS GOARCH CGO_ENABLED build [args]")
	}
	for i, key := range []string{"GOOS", "GOARCH", "CGO_ENABLED"} {
		if os.Args[i+1] == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, os.Args[i+1])
		}
	}
	args := os.Args[4:]
	if args[0] != "build" {
		return fmt.Errorf("unsupported product command %q", args[0])
	}
	// Product binaries must never retain a private, subsequently deleted SDK
	// path in runtime.defaultGOROOT. Trimmed builds leave it empty, allowing
	// runtime toolchain discovery to use its normal fallback.
	if err := requireTrimpath(append(strings.Fields(os.Getenv("GOFLAGS")), args[1:]...)); err != nil {
		return err
	}
	args = append([]string{args[0], "-trimpath"}, args[1:]...)
	probe := goCommand("env", "-json", "GOROOT", "GOVERSION", "GOOS", "GOMODCACHE")
	data, err := probe.Output()
	if err != nil {
		return err
	}
	var cfg struct{ GOROOT, GOVERSION, GOOS, GOMODCACHE string }
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if cfg.GOVERSION != toolchainVersion {
		return fmt.Errorf("requires %s, found %s", toolchainVersion, cfg.GOVERSION)
	}
	buildRoot := ""
	if cfg.GOOS == "linux" || cfg.GOOS == "darwin" {
		source := filepath.Join(cfg.GOROOT, "src", "runtime", "signal_unix.go")
		original, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		patched, err := patchRuntimeSource(original)
		if err != nil {
			return err
		}

		// Deterministic content-addressed SDK/overlay cache keyed by
		// (GOVERSION, sourceHash, patchHash, hostOS/hostArch) so Go's build
		// cache stays warm across invocations and the ~500 MB downloaded SDK
		// is copied at most once per toolchain version. Both the original and
		// build-root signal_unix.go hashes are verified on every run.
		var overlayPath string
		buildRoot, overlayPath, err = cachedOverlay(cfg.GOROOT, cfg.GOMODCACHE, cfg.GOVERSION, patched)
		if err != nil {
			return err
		}
		ldflags := ""
		tags := snapshotTag
		remaining := []string{}
		for i := 1; i < len(args); i++ {
			arg := args[i]
			if arg == "-overlay" || strings.HasPrefix(arg, "-overlay=") {
				return fmt.Errorf("caller overlay unsupported; refusing to replace it")
			}
			if arg == "-tags" || strings.HasPrefix(arg, "-tags=") {
				value, ok := strings.CutPrefix(arg, "-tags=")
				if !ok {
					i++
					if i == len(args) {
						return fmt.Errorf("missing -tags value")
					}
					value = args[i]
				}
				// Go accepts comma- or space-separated lists; emit commas.
				if fields := strings.Fields(strings.ReplaceAll(value, ",", " ")); len(fields) > 0 {
					tags = strings.Join(append(fields, snapshotTag), ",")
				}
				continue
			}
			if arg == "-ldflags" {
				i++
				if i == len(args) {
					return fmt.Errorf("missing -ldflags value")
				}
				ldflags += " " + args[i]
			} else if strings.HasPrefix(arg, "-ldflags=") {
				ldflags += " " + strings.TrimPrefix(arg, "-ldflags=")
			} else {
				remaining = append(remaining, arg)
			}
		}
		if goflags := os.Getenv("GOFLAGS"); strings.Contains(goflags, "-overlay") || strings.Contains(goflags, "-ldflags") || strings.Contains(goflags, "-tags") {
			return fmt.Errorf("GOFLAGS overlay/ldflags/tags unsupported; pass flags explicitly")
		}
		if strings.Contains(ldflags, "runtime.bashyInheritedIgnore") {
			return fmt.Errorf("caller cannot override the product startup contract")
		}
		ldflags += " -X runtime.bashyInheritedIgnore=1"
		args = append([]string{args[0], "-overlay=" + overlayPath, "-ldflags=" + ldflags, "-tags=" + tags}, remaining...)
	}
	cmd := goCommand(args...)
	if buildRoot != "" {
		cmd = localGoCommand(buildRoot, args...)
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("go exited %d", e.ExitCode())
		}
		return err
	}
	return nil
}

// Accept Go's boolean spellings, but never silently override an explicit opt-out.
func requireTrimpath(flags []string) error {
	for _, flag := range flags {
		// GOFLAGS also accepts individually quoted flags.
		flag = strings.Trim(flag, "\"'")
		if value, ok := strings.CutPrefix(flag, "-trimpath="); ok {
			enabled, err := strconv.ParseBool(value)
			if err != nil || !enabled {
				return fmt.Errorf("product builds require -trimpath; unsupported %q", flag)
			}
		}
	}
	return nil
}

func overlayCacheKey(goversion, patched string) string {
	patchSum := fmt.Sprintf("%x", sha256.Sum256([]byte(patched)))
	h := sha256.Sum256([]byte(goversion + "\x00" + sourceHash + "\x00" + patchSum + "\x00" + runtime.GOOS + "/" + runtime.GOARCH))
	return fmt.Sprintf("%x", h[:16])
}

func productCacheBase(modcache string) (string, error) {
	base := strings.TrimSpace(os.Getenv("BASHY_PRODUCTGO_CACHE"))
	if base == "" {
		if ucd, err := os.UserCacheDir(); err == nil && ucd != "" {
			base = filepath.Join(ucd, "bashy", "productgo")
		} else {
			base = filepath.Join(os.TempDir(), "bashy-productgo-cache")
		}
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", fmt.Errorf("create productgo cache dir: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(base)
	if err != nil {
		return "", err
	}
	if modcache != "" {
		if resolvedCache, err := filepath.EvalSymlinks(modcache); err == nil {
			modcache = resolvedCache
		}
		rel, err := filepath.Rel(modcache, resolved)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("productgo cache directory must be outside GOMODCACHE; set BASHY_PRODUCTGO_CACHE outside %s", modcache)
		}
	}
	return resolved, nil
}

func isInsidePath(parent, child string) bool {
	if parent == "" {
		return false
	}
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeFileIfChanged(path string, content []byte, perm os.FileMode) error {
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func cachedOverlay(goroot, modcache, goversion, patched string) (string, string, error) {
	canonicalRoot, err := filepath.EvalSymlinks(goroot)
	if err != nil {
		return "", "", err
	}
	resolvedCache := modcache
	if r, err := filepath.EvalSymlinks(modcache); err == nil {
		resolvedCache = r
	}
	cacheBase, err := productCacheBase(resolvedCache)
	if err != nil {
		return "", "", err
	}
	key := overlayCacheKey(goversion, patched)
	buildRoot := canonicalRoot
	if isInsidePath(resolvedCache, canonicalRoot) {
		sdkParent := filepath.Join(cacheBase, "sdk-"+key)
		sdkDir := filepath.Join(sdkParent, "goroot")
		if _, err := os.Stat(filepath.Join(sdkDir, "src", "runtime", "signal_unix.go")); err != nil {
			stage, err := os.MkdirTemp(cacheBase, ".stage-sdk-*")
			if err != nil {
				return "", "", err
			}
			stagedRoot, err := overlayRoot(goroot, modcache, stage)
			if err != nil {
				_ = os.RemoveAll(stage)
				return "", "", err
			}
			copied, err := os.ReadFile(filepath.Join(stagedRoot, "src", "runtime", "signal_unix.go"))
			if err != nil {
				_ = os.RemoveAll(stage)
				return "", "", err
			}
			if _, err := patchRuntimeSource(copied); err != nil {
				_ = os.RemoveAll(stage)
				return "", "", err
			}
			if err := os.Rename(stage, sdkParent); err != nil {
				_ = os.RemoveAll(stage)
				// Another concurrent build may have published sdkParent first.
				if _, statErr := os.Stat(filepath.Join(sdkDir, "src", "runtime", "signal_unix.go")); statErr != nil {
					return "", "", fmt.Errorf("publish cached toolchain: %w", err)
				}
			}
		}
		buildRoot = sdkDir
	}

	// Verify the bytes actually used by the build root on every run (fail-closed).
	source := filepath.Join(buildRoot, "src", "runtime", "signal_unix.go")
	copied, err := os.ReadFile(source)
	if err != nil {
		return "", "", err
	}
	if _, err := patchRuntimeSource(copied); err != nil {
		return "", "", fmt.Errorf("cached build root failed verification: %w", err)
	}

	overlayDir := filepath.Join(cacheBase, "overlay-"+key)
	replacement := filepath.Join(overlayDir, "signal_unix.go")
	if err := writeFileIfChanged(replacement, []byte(patched), 0600); err != nil {
		return "", "", err
	}
	// Verify replacement bytes on disk match the verified patched source.
	if got, err := os.ReadFile(replacement); err != nil || string(got) != patched {
		return "", "", fmt.Errorf("cached overlay signal_unix.go verification failed")
	}

	rootTag := fmt.Sprintf("%x", sha256.Sum256([]byte(buildRoot)))[:16]
	overlayBytes, _ := json.Marshal(map[string]any{"Replace": map[string]string{source: replacement}})
	overlayPath := filepath.Join(overlayDir, "overlay-"+rootTag+".json")
	if err := writeFileIfChanged(overlayPath, overlayBytes, 0600); err != nil {
		return "", "", err
	}
	return buildRoot, overlayPath, nil
}

// Go rejects overlays anywhere beneath GOMODCACHE, including downloaded SDKs.
// Copy those SDKs out, never hard-link or edit the shared installation. CopyFS
// preserves executable bits and fails closed on unsupported entries/symlinks.
func overlayRoot(root, cache, dir string) (string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	// The cache need not exist for an installed SDK and a stdlib-only build.
	if resolved, err := filepath.EvalSymlinks(cache); err == nil {
		cache = resolved
	}
	inside := func(path string) bool {
		rel, err := filepath.Rel(cache, path)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	if !inside(root) {
		return root, nil
	}
	dir, err = filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	if inside(dir) {
		return "", fmt.Errorf("private toolchain directory must be outside GOMODCACHE; set TMPDIR outside %s", cache)
	}
	private := filepath.Join(dir, "goroot")
	if err := os.CopyFS(private, os.DirFS(root)); err != nil {
		return "", fmt.Errorf("copy managed toolchain: %w", err)
	}
	return private, nil
}

func localGoCommand(root string, args ...string) *exec.Cmd {
	name := "go"
	if os.PathSeparator == '\\' {
		name += ".exe"
	}
	cmd := exec.Command(filepath.Join(root, "bin", name), args...)
	// The front door already selected and verified the SDK. Do not allow a
	// second selection to switch back to the immutable cached installation.
	cmd.Env = append(os.Environ(), "GOROOT="+root, "GOTOOLCHAIN=local")
	return cmd
}

// Artifact/DAG builds can retain their explicitly selected managed toolchain.
func goCommand(args ...string) *exec.Cmd {
	if front := os.Getenv("BASHY_EXE"); front != "" {
		return exec.Command(front, append([]string{"go"}, args...)...)
	}
	return exec.Command("go", args...)
}

func patchRuntimeSource(original []byte) (string, error) {
	if fmt.Sprintf("%x", sha256.Sum256(original)) != sourceHash {
		return "", fmt.Errorf("unrecognized Go runtime source")
	}
	anchor := "\tt := &sigtable[sig]\n\tif t.flags&_SigSetStack != 0 {"
	if strings.Count(string(original), anchor) != 1 {
		return "", fmt.Errorf("runtime patch anchor not unique")
	}
	return strings.Replace(string(original), anchor, "\tt := &sigtable[sig]\n"+insertion+"\tif t.flags&_SigSetStack != 0 {", 1) + helpers, nil
}
