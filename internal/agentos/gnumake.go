package agentos

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"mvdan.cc/sh/v3/interp"
)

// gnuMakeHandler hands `make` to the host's GNU make when the makefile in
// effect is a GNUmakefile. bashy's in-process make is the certified POSIX
// make, and a POSIX make never reads GNUmakefile — that name is GNU make's
// own marker for "this file needs GNU make". Everything else, and every
// certification run, stays with the POSIX make.
func gnuMakeHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 || args[0] != "make" || certProfile() {
				return next(ctx, args)
			}
			hc := interp.HandlerCtx(ctx)
			if !wantsGNUMake(hc.Dir, args[1:]) {
				return next(ctx, args)
			}
			gnu := findGNUMake(hc.Env.Get("PATH").String())
			if gnu == "" {
				fmt.Fprintln(hc.Stderr, "make: this directory's GNUmakefile needs GNU make, and none is on PATH (install gmake or GNU make)")
				return interp.ExitStatus(2)
			}
			return next(ctx, append([]string{gnu}, args[1:]...))
		}
	}
}

// wantsGNUMake reports whether a make invocation in dir would read a
// GNUmakefile: an explicit -f naming one, or no -f and a directory (after
// -C) holding GNUmakefile but neither makefile nor Makefile. Names are
// compared exactly, so a case-insensitive filesystem cannot mistake
// GNUmakefile's absence for its presence or vice versa.
func wantsGNUMake(dir string, args []string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		switch {
		case a == "-f" || a == "--file" || a == "--makefile":
			return i+1 < len(args) && filepath.Base(args[i+1]) == "GNUmakefile"
		case strings.HasPrefix(a, "--file=") || strings.HasPrefix(a, "--makefile="):
			return filepath.Base(a[strings.IndexByte(a, '=')+1:]) == "GNUmakefile"
		case strings.HasPrefix(a, "-f") && len(a) > 2:
			return filepath.Base(a[2:]) == "GNUmakefile"
		case a == "-C" || a == "--directory":
			if i+1 < len(args) {
				dir = joinDir(dir, args[i+1])
				i++
			}
		case strings.HasPrefix(a, "--directory="):
			dir = joinDir(dir, a[len("--directory="):])
		case strings.HasPrefix(a, "-C") && len(a) > 2:
			dir = joinDir(dir, a[2:])
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	var gnu, posix bool
	for _, e := range entries {
		switch e.Name() {
		case "GNUmakefile":
			gnu = true
		case "makefile", "Makefile":
			posix = true
		}
	}
	return gnu && !posix
}

func joinDir(base, d string) string {
	if filepath.IsAbs(d) {
		return d
	}
	return filepath.Join(base, d)
}

var gnuMakeProbe sync.Map // path -> bool

// findGNUMake returns the first gmake on PATH, else the first make on PATH
// that reports "GNU Make" and is not this bashy binary.
func findGNUMake(path string) string {
	self, _ := os.Executable()
	var selfInfo os.FileInfo
	if self != "" {
		selfInfo, _ = os.Stat(self)
	}
	exe := ""
	if runtime.GOOS == "windows" {
		exe = ".exe"
	}
	for _, name := range []string{"gmake", "make"} {
		for _, d := range filepath.SplitList(path) {
			if d == "" {
				continue
			}
			cand := filepath.Join(d, name+exe)
			fi, err := os.Stat(cand)
			if err != nil || fi.IsDir() || fi.Mode()&0o111 == 0 && runtime.GOOS != "windows" {
				continue
			}
			if selfInfo != nil && os.SameFile(fi, selfInfo) {
				continue
			}
			if isGNUMake(cand) {
				return cand
			}
		}
	}
	return ""
}

func isGNUMake(path string) bool {
	if v, ok := gnuMakeProbe.Load(path); ok {
		return v.(bool)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	out, _ := exec.CommandContext(ctx, path, "--version").Output()
	ok := bytes.Contains(out, []byte("GNU Make"))
	gnuMakeProbe.Store(path, ok)
	return ok
}
