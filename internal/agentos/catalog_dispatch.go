// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/qiangli/bashy/internal/agentos/cmdcatalog"
	"github.com/qiangli/bashy/internal/cmdgen"
	"github.com/qiangli/yoke/pkg/shape/slot"
	"github.com/qiangli/yoke/pkg/src"
)

// Testable seams mirroring shape_slot.go so unit tests exercise routing
// without spawning real compilers.
var (
	catalogCommandLookup     = cmdcatalog.Lookup
	catalogCommandEnsureSlot = defaultEnsureCatalogSlot
)

func catalogDispatchEnabled() bool {
	if !catalogDispatchCompiledIn {
		return false
	}
	if certProfile() {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("BASHY_CMD_DISPATCH")), "inprocess") {
		return false
	}
	return true
}

// dispatchCatalogCommand routes a catalog-declared front-door command through
// the existing yoke/pkg/shape/slot side-slot path:
//  1. If an active verified slot binary exists (slot.Store.Select), exec it
//     directly (honoring Undo / Restore / shape approve selections).
//  2. Otherwise, generate its tiny main + build on demand into the tree-hash
//     build cache + Stage/Activate its initial slot via cmdgen.EnsureSlot.
//  3. Launch through launchShapeSlot with slot.EntryEnvironment so stdin,
//     stdout, stderr, and exit codes pass through identically.
//
// Returns (exitCode, true) when the verb is owned by the command catalog, or
// (0, false) to fall through to the existing switch.
func dispatchCatalogCommand(ctx context.Context, name string, args []string) (int, bool) {
	if !catalogDispatchEnabled() {
		return 0, false
	}
	entry, ok := catalogCommandLookup(name)
	if !ok {
		return 0, false
	}
	active, found, err := shapeSlotSelect(entry.Name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy: %s: shape slot error: %v\n", name, err)
		return 126, true
	}
	if !found {
		active, err = catalogCommandEnsureSlot(ctx, entry)
		if err != nil {
			fmt.Fprintf(os.Stderr, "bashy: %s: on-demand slot build failed: %v\n", name, err)
			return 126, true
		}
	}
	argv := append([]string{active.Path}, args...)
	resolved, err := shapedRegisteredArgv(entry.Name, argv)
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", name, err)
		return 126, true
	}
	env, err := slot.EntryEnvironment(resolved[0], os.Environ())
	if err != nil {
		fmt.Fprintf(os.Stderr, "bashy: %s: %v\n", name, err)
		return 126, true
	}
	launchArgs := append([]string{name}, resolved[1:]...)
	return shapeSlotLaunch(resolved[0], launchArgs, env), true
}

func defaultEnsureCatalogSlot(ctx context.Context, entry cmdgen.Entry) (slot.Version, error) {
	opts := catalogBuildOptions(entry)
	ver, _, err := cmdgen.EnsureSlot(ctx, entry, opts)
	return ver, err
}

func catalogBuildOptions(entry cmdgen.Entry) cmdgen.BuildOptions {
	home := src.Home()
	opts := cmdgen.BuildOptions{
		Home:   home,
		GoTool: resolveCatalogGoTool(),
	}
	if gw := strings.TrimSpace(os.Getenv("BASHY_CMD_GOWORK")); gw != "" {
		opts.GoWork = gw
	} else {
		opts.GoWork = findNearbyGoWork()
	}
	// If a local source checkout exists under BASHY_HOME/src/<repo> and no
	// workspace already supplies entry.Module, mount it as a local module.
	if opts.GoWork == "" {
		if repoName := repoNameForModule(entry.Module); repoName != "" {
			cand := filepath.Join(home, "src", repoName)
			if _, err := os.Stat(filepath.Join(cand, "go.mod")); err == nil {
				opts.LocalModules = map[string]string{entry.Module: cand}
			}
		}
	}
	if gm := strings.TrimSpace(os.Getenv("BASHY_CMD_GOMOD")); gm != "" {
		if info, err := cmdgen.ParseGoModFile(gm); err == nil {
			opts.GoMod = info
		}
	} else if opts.GoWork != "" {
		if info, err := cmdgen.ParseGoModFile(filepath.Join(filepath.Dir(opts.GoWork), "bashy", "go.mod")); err == nil {
			opts.GoMod = info
		}
	}
	return opts
}

func repoNameForModule(mod string) string {
	switch mod {
	case "github.com/qiangli/yoke":
		return "yoke"
	case "github.com/qiangli/coreutils":
		return "coreutils"
	case "github.com/qiangli/bashy":
		return "bashy"
	default:
		return ""
	}
}

func findNearbyGoWork() string {
	var starts []string
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		starts = append(starts, cwd)
	}
	if exe, err := os.Executable(); err == nil && exe != "" {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		starts = append(starts, filepath.Dir(exe))
	}
	for _, dir := range starts {
		d := dir
		for depth := 0; depth < 5; depth++ {
			cand := filepath.Join(d, "go.work")
			if st, err := os.Stat(cand); err == nil && st.Mode().IsRegular() {
				return cand
			}
			parent := filepath.Dir(d)
			if parent == d {
				break
			}
			d = parent
		}
	}
	return ""
}

func resolveCatalogGoTool() string {
	if _, err := exec.LookPath("go"); err == nil {
		return "go"
	}
	self := bashySelfPath()
	out, err := exec.Command(self, "go", "env", "GOROOT").Output()
	if err != nil {
		return "go"
	}
	root := strings.TrimSpace(string(out))
	if i := strings.LastIndexAny(root, "\r\n"); i >= 0 {
		root = strings.TrimSpace(root[i+1:])
	}
	if root == "" {
		return "go"
	}
	name := "go"
	if runtime.GOOS == "windows" {
		name = "go.exe"
	}
	cand := filepath.Join(root, "bin", name)
	if _, err := os.Stat(cand); err == nil {
		return cand
	}
	return "go"
}
