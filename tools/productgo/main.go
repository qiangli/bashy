// Command productgo builds Bashy with its explicitly opted-in startup signal
// contract. The installed Go source tree is read-only; unknown sources fail.
package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
`

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
	probe := goCommand("env", "-json", "GOROOT", "GOVERSION", "GOOS", "GOMODCACHE")
	data, err := probe.Output()
	if err != nil {
		return err
	}
	var cfg struct{ GOROOT, GOVERSION, GOOS, GOMODCACHE string }
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if cfg.GOVERSION != "go1.27.1" {
		return fmt.Errorf("requires go1.27.1, found %s", cfg.GOVERSION)
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

		// Private per-invocation files avoid races with concurrent builds. Go's
		// build cache still keys compiled runtime objects by their contents.
		dir, err := os.MkdirTemp("", "bashy-runtime-overlay-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		buildRoot, err = overlayRoot(cfg.GOROOT, cfg.GOMODCACHE, dir)
		if err != nil {
			return err
		}
		source = filepath.Join(buildRoot, "src", "runtime", "signal_unix.go")
		// Verify the bytes actually used by the relocated build too.
		copied, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if _, err := patchRuntimeSource(copied); err != nil {
			return err
		}
		replacement := filepath.Join(dir, "signal_unix.go")
		if err := os.WriteFile(replacement, []byte(patched), 0600); err != nil {
			return err
		}
		overlay, _ := json.Marshal(map[string]any{"Replace": map[string]string{source: replacement}})
		path := filepath.Join(dir, "overlay.json")
		if err := os.WriteFile(path, overlay, 0600); err != nil {
			return err
		}
		ldflags := ""
		remaining := []string{}
		for i := 1; i < len(args); i++ {
			arg := args[i]
			if arg == "-overlay" || strings.HasPrefix(arg, "-overlay=") {
				return fmt.Errorf("caller overlay unsupported; refusing to replace it")
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
		if strings.Contains(os.Getenv("GOFLAGS"), "-overlay") || strings.Contains(os.Getenv("GOFLAGS"), "-ldflags") {
			return fmt.Errorf("GOFLAGS overlay/ldflags unsupported; pass flags explicitly")
		}
		if strings.Contains(ldflags, "runtime.bashyInheritedIgnore") {
			return fmt.Errorf("caller cannot override the product startup contract")
		}
		ldflags += " -X runtime.bashyInheritedIgnore=1"
		args = append([]string{args[0], "-overlay=" + path, "-ldflags=" + ldflags}, remaining...)
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
