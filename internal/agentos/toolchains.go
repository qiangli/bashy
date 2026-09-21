package agentos

import (
	"context"
	"fmt"
	"strings"

	"github.com/qiangli/yoke/external/bun"
	"github.com/qiangli/yoke/external/gotoolchain"
	"github.com/qiangli/yoke/external/node"
	"github.com/qiangli/yoke/external/python"
	"github.com/qiangli/yoke/external/rust"
	"github.com/qiangli/yoke/external/zigcc"
	"mvdan.cc/sh/v3/polyglot"
)

// islandToolchains is the table behind the fence rule — "a fence never
// resolves its tool from PATH": one row per tool name a Bash# island or the
// Go front end may ask for, each answered by the provisioner the matching
// `bashy <tool>` front door already runs (pinned release, digest-verified,
// cached; download+exec, never bundled). The same request yields the same
// program on every host, which is what makes an island's fingerprint and its
// attestation comparable across machines.
//
// Licenses: Go BSD-3 · Zig MIT · uv MIT/Apache-2.0 + CPython PSF-2.0 · Node
// MIT + typescript Apache-2.0 · Bun MIT · rustup MIT/Apache-2.0. A row's Ensure is cache-first, so the cost is
// paid once; `bashy check --prepare` pays it ahead of a run.
var islandToolchains = map[string]func(ctx context.Context) (argv []string, why string, err error){
	"go": func(ctx context.Context) ([]string, string, error) {
		bin, _, err := gotoolchain.Ensure(ctx, "")
		return single(bin, "selected provisioned go "+gotoolchain.DefaultVersion, err)
	},
	"cc": provisionedCC,
	// rustc's -C linker= takes one program: a wrapper over zig cc.
	"cc-linker": provisionedLinker,
	"clang":     provisionedCC,
	"c++":       provisionedCXX,
	"clang++":   provisionedCXX,
	"python3": func(ctx context.Context) ([]string, string, error) {
		return provisionedPython(ctx, "")
	},
	"node": func(ctx context.Context) ([]string, string, error) {
		bin, _, err := node.Ensure(ctx, "")
		return single(bin, "selected provisioned node "+node.DefaultVersion, err)
	},
	// bun: the runtime a project selects with a bun lockfile or
	// BASHPP_TYPESCRIPT_RUNTIME=bun (node stays the default).
	"bun": func(ctx context.Context) ([]string, string, error) {
		bin, err := bun.Ensure(ctx, "")
		return single(bin, "selected provisioned "+bun.DefaultVersion, err)
	},
	// The TypeScript island's compiler MODULE (asked for by name when the
	// project carries no typescript of its own): the pinned package dir.
	"typescript": func(ctx context.Context) ([]string, string, error) {
		dir, err := node.EnsureTypeScript(ctx)
		return single(dir, "selected provisioned typescript "+node.DefaultTypeScript, err)
	},
	"rustc": func(ctx context.Context) ([]string, string, error) {
		bin, err := rust.EnsureRustc(ctx)
		return single(bin, "selected provisioned rustc ("+rust.DefaultToolchain+")", err)
	},
}

func provisionedCC(ctx context.Context) ([]string, string, error) {
	argv, err := zigcc.CC(ctx)
	return argv, "selected provisioned zig cc", err
}

func provisionedCXX(ctx context.Context) ([]string, string, error) {
	argv, err := zigcc.CXX(ctx)
	return argv, "selected provisioned zig c++", err
}

func provisionedPython(ctx context.Context, version string) ([]string, string, error) {
	bin, err := python.EnsureInterpreter(ctx, version)
	label := version
	if label == "" {
		label = python.DefaultPython
	}
	return single(bin, "selected provisioned CPython "+label+" (uv-managed)", err)
}

func single(bin, why string, err error) ([]string, string, error) {
	if err != nil {
		return nil, "", err
	}
	return []string{bin}, why, nil
}

// islandToolResolver is polyglot.ToolResolver for bashy: the table above,
// plus "pythonX.Y" for a project-declared version (a .python-version file),
// which uv honours as a managed request. A name with no row is refused with
// the names that have one — the island then fails with that line rather
// than silently reaching for PATH.
func islandToolResolver(name string) ([]string, string, error) {
	ctx := context.Background()
	if row, ok := islandToolchains[name]; ok {
		return row(ctx)
	}
	if version, ok := strings.CutPrefix(name, "python"); ok && version != "" && version[0] >= '0' && version[0] <= '9' {
		return provisionedPython(ctx, version)
	}
	return nil, "", fmt.Errorf("bashy provisions no toolchain named %q (it provisions %s); set BASHPP_* to name a program explicitly", name, strings.Join(islandToolchainNames(), ", "))
}

func islandToolchainNames() []string {
	return append([]string{"go", "cc", "c++", "python3", "node", "bun", "typescript", "rustc"}, fenceTools...)
}

// installIslandToolResolver wires the table into the engine once per process.
// Off under VSC_PROFILE=cert, where the engine keeps its PATH lookup — the
// same exclusion the registered-command ring has. There is no other global
// opt-out: a program that must use a host toolchain names it with the
// matching BASHPP_* variable, visibly and per tool.
func installIslandToolResolver() {
	if certProfile() {
		return
	}
	polyglot.ToolResolver = islandToolResolver
}
