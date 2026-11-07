package agentos

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"slices"

	"mvdan.cc/sh/v3/interp"
)

// hostConvertersHandler hands the Windows path-spelling converters to the
// host's own executable whenever PATH provides one.
//
// bashy's userland resolves ahead of PATH on purpose (Tier 0 shell → 1 pure-Go
// userland → … → 3 PATH; docs/source-layout.md), and `type`/`command -v` learn
// about a pre-PATH rung through interp.CommandResolver. cygpath and wslpath are
// the exception, for two reasons that both point the same way (todo 9f947a58):
//
//   - They are the ONE part of the userland that emulates a tool belonging to a
//     specific host installation. A Git-Bash/MSYS2/Cygwin or WSL host's own
//     converter knows that installation's mount table; bashy's is the portable
//     stand-in for the hosts (every non-Windows one included) that have none.
//   - Nothing reports them as a pre-PATH rung, so the shell CONTRADICTED
//     itself: `type -a cygpath` and `command -v cygpath` named the PATH file
//     while a bare `cygpath` ran the applet, so a script that probes before
//     calling — the portable idiom, and what examples/outpost-profile does —
//     got one answer from the probe and another from the call. GNU Bash has no
//     cygpath builtin and runs the PATH file; so does bashy now, which is also
//     the answer introspection was already giving.
//
// A host file that IS this bashy binary (a multicall link in a BASHY_ROOT
// userland layout) is skipped: the applet serves it in process instead of
// re-executing bashy, and the result is identical either way.
//
// The rung sits outside the applet handler and rewrites argv[0] to the resolved
// absolute path, so the applet handler no longer recognises the name and the
// interpreter's own exec runs the host file. With no host file on PATH, argv is
// untouched and the applet answers exactly as before. Every other userland name
// keeps the documented pure-Go-first precedence.
func hostConvertersHandler() func(interp.ExecHandlerFunc) interp.ExecHandlerFunc {
	return func(next interp.ExecHandlerFunc) interp.ExecHandlerFunc {
		return func(ctx context.Context, args []string) error {
			if len(args) == 0 || !slices.Contains(hostPreferredConverters, args[0]) {
				return next(ctx, args)
			}
			host := findHostConverter(interp.HandlerCtx(ctx).Env.Get("PATH").String(), args[0])
			if host == "" {
				return next(ctx, args)
			}
			return next(ctx, append([]string{host}, args[1:]...))
		}
	}
}

// hostPreferredConverters are the applet names a host executable outranks:
// the path-spelling converters that exist to emulate a host-provided tool.
// The rest of the userland is unaffected.
var hostPreferredConverters = []string{"cygpath", "wslpath"}

// findHostConverter returns the first executable named name on path that is
// not this bashy binary, or "" when the host provides none.
func findHostConverter(path, name string) string {
	self, _ := os.Executable()
	var selfInfo os.FileInfo
	if self != "" {
		selfInfo, _ = os.Stat(self)
	}
	exts := []string{""}
	if runtime.GOOS == "windows" {
		exts = []string{".exe", ".com", ""}
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			cand := filepath.Join(dir, name+ext)
			fi, err := os.Stat(cand)
			if err != nil || fi.IsDir() {
				continue
			}
			if runtime.GOOS != "windows" && fi.Mode()&0o111 == 0 {
				continue
			}
			if selfInfo != nil && os.SameFile(fi, selfInfo) {
				continue
			}
			return cand
		}
	}
	return ""
}
