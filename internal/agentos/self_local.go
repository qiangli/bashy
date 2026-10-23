package agentos

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	ygit "github.com/qiangli/yoke/git"
	"github.com/qiangli/yoke/pkg/src"
)

func nowUTC() string { return time.Now().UTC().Format(time.RFC3339) }

// self_local.go — `bashy self build --local` (M2b deliverable 5, Go half).
//
// It builds the `local` branch tip into the src build cache
// ($BASHY_HOME/src/.builds/<repo>/<short-local>/), recording a manifest
// the run-local dispatcher trusts only while the live State matches it.
// The build refuses when the pristine..local diff touches core paths: a
// cached build must never carry locally-rewritten certified core.
//
// Two shapes, one cache:
//   - --repo bashy (default): the bashy source checkout's own `local`
//     branch, built as ./cmd/bashy from a clean clone at that tip;
//   - --repo yoke|coreutils|sh|filebrowser: the fetched checkout's `local`
//     tip, applied to this bashy tree through a Go replace directive, so
//     the cached binary contains exactly the merged local source.

func runSelfBuildLocal(home, repo string) (string, error) {
	if home == "" {
		home = src.Home()
	}
	if repo == "" {
		repo = "bashy"
	}
	if repo == "bashy" {
		return buildBashyLocal(home)
	}
	return buildRepoLocal(home, repo)
}

// localTipState resolves the pristine/local pins for a build source dir
// that already follows the src branch vocabulary. A missing `local`
// branch means there is nothing local to build; a missing `pristine`
// branch means the base cannot be verified, which refuses the build.
func localTipState(dir, repo string) (base, local string, err error) {
	base, err = ygit.ResolveCommit(dir, "pristine")
	if err != nil {
		return "", "", fmt.Errorf("self build --local: no `pristine` branch in %s: cannot verify the base, refusing", dir)
	}
	local, err = ygit.ResolveCommit(dir, "local")
	if err != nil {
		return "", "", fmt.Errorf("self build --local: no `local` branch in %s: nothing local to build", dir)
	}
	return base, local, nil
}

func buildBashyLocal(home string) (string, error) {
	root, ok := findBashySourceRoot(".")
	if !ok {
		return "", fmt.Errorf("self build --local: no bashy source checkout found (run inside one)")
	}
	base, local, err := localTipState(root, "bashy")
	if err != nil {
		return "", err
	}
	if base == local {
		return "", fmt.Errorf("self build --local: `local` == `pristine` (%s): nothing differs from the release; the release binary stays", shortSHA12(base))
	}
	if core, err := localChangedFiles(root, base, local); err != nil {
		return "", err
	} else {
		var touched []string
		for _, f := range core {
			if src.PathClass("bashy", f) == src.ClassCore {
				touched = append(touched, f)
			}
		}
		if len(touched) > 0 {
			return "", fmt.Errorf("self build --local: refusing: core path(s) %s differ from release (no approval can unlock them)", strings.Join(touched, ", "))
		}
	}
	clone, err := cloneAtBranch(root, "local")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(clone)
	out := filepath.Join(localBuildsDir(home), "bashy", shortSHA12(local), releaseBinaryName())
	if err := buildBashyInDir(clone, out, "local-"+shortSHA12(local)); err != nil {
		return "", err
	}
	m := localBuildManifest{Repo: "bashy", Base: base, Local: local, State: localRepoState(home, "bashy", base, local), Binary: out, BuiltAt: nowUTC()}
	if err := writeLocalBuildManifest(home, "bashy", m); err != nil {
		return "", err
	}
	return out, nil
}

func buildRepoLocal(home, repo string) (string, error) {
	pin, ok := srcModulePins[repo]
	if !ok {
		return "", fmt.Errorf("self build --local: unknown repo %q (one of: bashy, yoke, coreutils, sh, filebrowser)", repo)
	}
	dir := filepath.Join(src.SrcDir(home), repo)
	if st, err := os.Stat(filepath.Join(dir, ".git")); err != nil || !st.IsDir() {
		return "", fmt.Errorf("self build --local: repo %q is not fetched (no checkout at %s)", repo, dir)
	}
	st, err := src.State(home, repo)
	if err != nil {
		return "", fmt.Errorf("self build --local: %v", err)
	}
	if st.State == src.StatePristine {
		return "", fmt.Errorf("self build --local: %s is pristine (local == release %s): nothing differs; the release binary stays", repo, shortSHA12(st.Base))
	}
	if core, err := localBuildCorePaths(home, repo, st); err != nil {
		return "", err
	} else if len(core) > 0 {
		return "", fmt.Errorf("self build --local: refusing: core path(s) %s differ from release (no approval can unlock them)", strings.Join(core, ", "))
	}
	root, ok := findBashySourceRoot(".")
	if !ok {
		return "", fmt.Errorf("self build --local: no bashy source checkout found (run inside one)")
	}
	// A clean clone at the local tip: the replace target is exactly the
	// merged source, never whatever the worktree happens to hold.
	clone, err := cloneAtBranch(dir, "local")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(clone)
	out := filepath.Join(localBuildsDir(home), repo, shortSHA12(st.Local), releaseBinaryName())
	if err := buildBashyWithReplace(root, pin.module, clone, out, "local-"+shortSHA12(st.Local)); err != nil {
		return "", err
	}
	m := localBuildManifest{Repo: repo, Base: st.Base, Local: st.Local, State: string(st.State), Binary: out, BuiltAt: nowUTC()}
	if err := writeLocalBuildManifest(home, repo, m); err != nil {
		return "", err
	}
	return out, nil
}

// localRepoState re-derives the tier for a manifest without trusting it.
func localRepoState(home, repo, base, local string) string {
	if base == local {
		return src.StatePristine
	}
	st, err := src.State(home, repo)
	if err != nil {
		return "unknown"
	}
	return string(st.State)
}

// cloneAtBranch clones src (a local path) at branch into a temp dir.
func cloneAtBranch(srcPath, branch string) (string, error) {
	tmp, err := os.MkdirTemp("", "bashy-local-build-*")
	if err != nil {
		return "", err
	}
	dst := filepath.Join(tmp, "work")
	if _, err := ygit.Clone(ygit.CloneOptions{URL: srcPath, Path: dst, Branch: branch, SingleBranch: true}); err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("self build --local: clone at %s: %w", branch, err)
	}
	return dst, nil
}

// buildBashyInDir builds ./cmd/bashy in dir with the managed Go toolchain
// (the same door `self build` uses), stamping the local version.
func buildBashyInDir(dir, target, version string) error {
	if strings.TrimSpace(version) == "" {
		version = "dev"
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return fmt.Errorf("self build --local: cannot resolve current executable for managed Go build")
	}
	ldflags := "-s -w -X github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-" + version
	c := exec.Command(exe, "go", "build", "-trimpath", "-ldflags", ldflags, "-o", target, "./cmd/bashy")
	c.Dir = dir
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}

// buildBashyWithReplace builds this bashy tree with repo module pointed at
// the local-tip clone, through a throwaway -modfile so the working tree's
// go.mod is never touched.
func buildBashyWithReplace(root, module, replacement, target, version string) error {
	modData, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return err
	}
	sumData, err := os.ReadFile(filepath.Join(root, "go.sum"))
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "bashy-local-modfile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	altBase := filepath.Join(tmp, "local")
	if err := os.WriteFile(altBase+".mod", modData, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(altBase+".sum", sumData, 0o644); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil || exe == "" {
		return fmt.Errorf("self build --local: cannot resolve current executable for managed Go build")
	}
	edit := exec.Command(exe, "go", "mod", "edit", "-modfile="+altBase+".mod", "-replace", module+"="+replacement)
	edit.Dir = root
	if out, err := edit.CombinedOutput(); err != nil {
		return fmt.Errorf("self build --local: replace %s: %v\n%s", module, err, out)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	ldflags := "-s -w -X github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-" + version
	build := exec.Command(exe, "go", "build", "-modfile="+altBase+".mod", "-trimpath", "-ldflags", ldflags, "-o", target, "./cmd/bashy")
	build.Dir = root
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		return fmt.Errorf("self build --local: %w", err)
	}
	return nil
}
