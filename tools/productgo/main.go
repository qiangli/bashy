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
		return fmt.Errorf("usage: productgo GOOS GOARCH CGO_ENABLED build|test|run [args]")
	}
	for i, key := range []string{"GOOS", "GOARCH", "CGO_ENABLED"} {
		if os.Args[i+1] == "" {
			os.Unsetenv(key)
		} else {
			os.Setenv(key, os.Args[i+1])
		}
	}
	args := os.Args[4:]
	if args[0] != "build" && args[0] != "test" && args[0] != "run" {
		return fmt.Errorf("unsupported product command %q", args[0])
	}
	probe := exec.Command("go", "env", "-json", "GOROOT", "GOVERSION", "GOOS")
	data, err := probe.Output()
	if err != nil {
		return err
	}
	var cfg struct{ GOROOT, GOVERSION, GOOS string }
	if err := json.Unmarshal(data, &cfg); err != nil {
		return err
	}
	if cfg.GOVERSION != "go1.27.1" {
		return fmt.Errorf("requires go1.27.1, found %s", cfg.GOVERSION)
	}
	if cfg.GOOS == "linux" || cfg.GOOS == "darwin" {
		source := filepath.Join(cfg.GOROOT, "src", "runtime", "signal_unix.go")
		original, err := os.ReadFile(source)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(original)) != sourceHash {
			return fmt.Errorf("unrecognized runtime source: %s", source)
		}
		anchor := "\tt := &sigtable[sig]\n\tif t.flags&_SigSetStack != 0 {"
		if strings.Count(string(original), anchor) != 1 {
			return fmt.Errorf("runtime patch anchor not unique")
		}
		patched := strings.Replace(string(original), anchor, "\tt := &sigtable[sig]\n"+insertion+"\tif t.flags&_SigSetStack != 0 {", 1) + helpers
		// Private per-invocation files avoid races with concurrent builds. Go's
		// build cache still keys compiled runtime objects by their contents.
		dir, err := os.MkdirTemp("", "bashy-runtime-overlay-")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		replacement := filepath.Join(dir, "signal_unix.go")
		if err := os.WriteFile(replacement, []byte(patched), 0600); err != nil {
			return err
		}
		overlay, _ := json.Marshal(map[string]any{"Replace": map[string]string{source: replacement}})
		path := filepath.Join(dir, "overlay.json")
		if err := os.WriteFile(path, overlay, 0600); err != nil {
			return err
		}
		ldflags := "-X runtime.bashyInheritedIgnore=1"
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
		args = append([]string{args[0], "-overlay=" + path, "-ldflags=" + ldflags}, remaining...)
	}
	cmd := exec.Command("go", args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("go exited %d", e.ExitCode())
		}
		return err
	}
	return nil
}
