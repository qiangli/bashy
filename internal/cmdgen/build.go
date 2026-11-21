// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package cmdgen

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/qiangli/yoke/pkg/shape/slot"
)

// BuildOptions controls on-demand command binary compilation, tree-hash caching,
// and shape-slot staging/activation.
type BuildOptions struct {
	// Home is BASHY_HOME (defaults to ~/.bashy). Cache lives under
	// <Home>/shape-cache/cmd/<name>/<treeHash>/ and slots under
	// <Home>/shape-host/<name>/.
	Home string
	// GoTool is the Go executable (defaults to "go").
	GoTool string
	// GoWork is an optional existing go.work path (for example, the umbrella
	// workspace) whose `use` and `replace` entries are merged with the
	// generated command directory so local sibling packages build offline.
	GoWork string
	// LocalModules maps Go module paths to local source checkouts. When
	// provided (for example during shaping or umbrella development), those
	// paths are mounted in a temporary go.work alongside the generated main
	// and included in the tree hash.
	LocalModules map[string]string
	// GoMod supplies pinned versions and replace directives for standalone
	// generation when building outside a workspace.
	GoMod GoModInfo
	// GOOS / GOARCH default to runtime.GOOS / runtime.GOARCH.
	GOOS   string
	GOARCH string
	// Strip controls -s -w on non-Linux (-w on Linux). Defaults to true.
	NoStrip bool
	// Revision optionally overrides the revision recorded on slot.Version.
	// When empty, defaults to "tree:<treeHash[:12]>".
	Revision string
	// Evidence optionally records verification evidence on slot.Version.
	Evidence string
	// BuildTimeout bounds `go build` (defaults to 3 minutes).
	BuildTimeout time.Duration
}

// BuildResult reports the build cache outcome and staged slot artifact.
type BuildResult struct {
	TreeHash  string
	CachePath string
	CacheHit  bool
	Duration  time.Duration
	Version   slot.Version
	State     slot.State
}

func (o BuildOptions) home() (string, error) {
	if o.Home != "" {
		return o.Home, nil
	}
	if env := strings.TrimSpace(os.Getenv("BASHY_HOME")); env != "" {
		return env, nil
	}
	u, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(u, ".bashy"), nil
}

func (o BuildOptions) goTool() string {
	if o.GoTool != "" {
		return o.GoTool
	}
	return "go"
}

func (o BuildOptions) goos() string {
	if o.GOOS != "" {
		return o.GOOS
	}
	return runtime.GOOS
}

func (o BuildOptions) goarch() string {
	if o.GOARCH != "" {
		return o.GOARCH
	}
	return runtime.GOARCH
}

func (o BuildOptions) timeout() time.Duration {
	if o.BuildTimeout > 0 {
		return o.BuildTimeout
	}
	return 3 * time.Minute
}

func binaryFilename(name, goos string) string {
	if goos == "windows" {
		return name + ".exe"
	}
	return name
}

// ComputeTreeHash returns a deterministic SHA-256 hex digest covering the
// generated main/go.mod bytes, target platform/toolchain/tags, and (when a
// local module checkout exists for entry.Module) the target package's source
// tree. Any edit to the command's source package changes the tree hash.
func ComputeTreeHash(entry Entry, gen GeneratedFiles, opts BuildOptions) (string, error) {
	h := sha256.New()
	fmt.Fprintf(h, "schema=bashy-cmdgen-tree-v1\n")
	fmt.Fprintf(h, "name=%s\n", entry.Name)
	fmt.Fprintf(h, "goos=%s\ngoarch=%s\n", opts.goos(), opts.goarch())
	fmt.Fprintf(h, "goversion=%s\n", runtime.Version())
	fmt.Fprintf(h, "tags=%s\n", strings.Join(entry.BuildTags, ","))
	fmt.Fprintf(h, "strip=%t\n", !opts.NoStrip)
	fmt.Fprintf(h, "spec=%s\n", gen.SpecHash)

	modDirs := resolvedLocalModules(opts)
	if modDir, ok := modDirs[entry.Module]; ok {
		if rel, ok := entry.PackageRelDir(); ok {
			pkgDir := filepath.Join(modDir, filepath.FromSlash(rel))
			if err := hashGoDir(h, "pkg:"+entry.Package, pkgDir); err != nil {
				return "", fmt.Errorf("cmdgen: hash package %s: %w", entry.Package, err)
			}
		}
		// Include the owning module's go.mod so dependency pin bumps bust cache.
		if b, err := os.ReadFile(filepath.Join(modDir, "go.mod")); err == nil {
			fmt.Fprintf(h, "mod:%s/go.mod:%d\x00", entry.Module, len(b))
			h.Write(b)
			h.Write([]byte{'\n'})
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func hashGoDir(h interface{ Write([]byte) (int, error) }, label, root string) error {
	st, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("%s is not a directory", root)
	}
	var files []string
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			base := d.Name()
			if base == ".git" || base == "node_modules" || base == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(d.Name(), "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	fmt.Fprintf(h, "tree:%s:%d\n", label, len(files))
	for _, rel := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		b, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		fmt.Fprintf(h, "file:%s:%d:%x\n", rel, len(b), sum)
	}
	return nil
}

// EnsureSlot returns the verified active slot binary for entry, building and
// activating it on demand only when no active slot exists yet. Existing active
// slots (including Undo / Restore selections) are returned untouched.
func EnsureSlot(ctx context.Context, entry Entry, opts BuildOptions) (slot.Version, BuildResult, error) {
	home, err := opts.home()
	if err != nil {
		return slot.Version{}, BuildResult{}, err
	}
	store := slot.Store{Home: home, App: entry.Name}
	active, found, err := store.Select()
	if err != nil {
		return slot.Version{}, BuildResult{}, err
	}
	if found {
		return active, BuildResult{Version: active, CacheHit: true}, nil
	}
	res, err := BuildAndActivateSlot(ctx, entry, opts)
	if err != nil {
		return slot.Version{}, res, err
	}
	return res.Version, res, nil
}

// BuildToCache generates the tiny main for entry, checks the tree-hash build
// cache under <Home>/shape-cache/cmd/<name>/<treeHash>/, and builds atomically
// on a cache miss. It does not mutate slot state.
func BuildToCache(ctx context.Context, entry Entry, opts BuildOptions) (BuildResult, error) {
	start := time.Now()
	if err := entry.Validate(); err != nil {
		return BuildResult{}, err
	}
	home, err := opts.home()
	if err != nil {
		return BuildResult{}, err
	}
	genWork, err := os.MkdirTemp("", "bashy-cmdgen-src-*")
	if err != nil {
		return BuildResult{}, err
	}
	defer os.RemoveAll(genWork)
	if real, err := filepath.EvalSymlinks(genWork); err == nil {
		genWork = real
	}

	genOpts := GenerateOptions{
		GoMod: opts.GoMod,
	}
	gen, err := GenerateDir(entry, genWork, genOpts)
	if err != nil {
		return BuildResult{}, err
	}
	treeHash, err := ComputeTreeHash(entry, gen, opts)
	if err != nil {
		return BuildResult{}, err
	}

	cacheDir := filepath.Join(home, "shape-cache", "cmd", entry.Name, treeHash)
	cacheBin := filepath.Join(cacheDir, binaryFilename(entry.Name, opts.goos()))
	if st, err := os.Stat(cacheBin); err == nil && st.Mode().IsRegular() {
		return BuildResult{
			TreeHash:  treeHash,
			CachePath: cacheBin,
			CacheHit:  true,
			Duration:  time.Since(start),
		}, nil
	}

	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return BuildResult{}, err
	}
	tmpOut, err := os.CreateTemp(cacheDir, ".build-*")
	if err != nil {
		return BuildResult{}, err
	}
	tmpPath := tmpOut.Name()
	_ = tmpOut.Close()
	defer os.Remove(tmpPath)

	goworkEnv := "GOWORK=off"
	if workFile, err := buildGoWorkFile(genWork, opts); err != nil {
		return BuildResult{}, err
	} else if workFile != "" {
		goworkEnv = "GOWORK=" + workFile
	}

	buildArgs := []string{"build", "-trimpath"}
	if !opts.NoStrip {
		if opts.goos() == "linux" {
			buildArgs = append(buildArgs, "-ldflags", "-w")
		} else {
			buildArgs = append(buildArgs, "-ldflags", "-s -w")
		}
	}
	if len(entry.BuildTags) > 0 {
		buildArgs = append(buildArgs, "-tags", strings.Join(entry.BuildTags, ","))
	}
	buildArgs = append(buildArgs, "-o", tmpPath, ".")

	bctx, cancel := context.WithTimeout(ctx, opts.timeout())
	defer cancel()
	cmd := exec.CommandContext(bctx, opts.goTool(), buildArgs...)
	cmd.Dir = genWork
	cmd.Env = setEnvOverrides(os.Environ(), map[string]string{
		"GOWORK":      strings.TrimPrefix(goworkEnv, "GOWORK="),
		"CGO_ENABLED": "0",
		"GOOS":        opts.goos(),
		"GOARCH":      opts.goarch(),
	})
	var log bytes.Buffer
	cmd.Stdout, cmd.Stderr = &log, &log
	if err := cmd.Run(); err != nil {
		return BuildResult{}, fmt.Errorf("cmdgen: go build %s (%s): %w\n%s", entry.Name, entry.Package, err, strings.TrimSpace(log.String()))
	}
	if opts.goos() != "windows" {
		_ = os.Chmod(tmpPath, 0o755)
	}
	if err := os.Rename(tmpPath, cacheBin); err != nil {
		// Another concurrent builder may have published the identical content-addressed binary.
		if st, statErr := os.Stat(cacheBin); statErr != nil || !st.Mode().IsRegular() {
			return BuildResult{}, fmt.Errorf("cmdgen: publish cached binary: %w", err)
		}
	}
	return BuildResult{
		TreeHash:  treeHash,
		CachePath: cacheBin,
		CacheHit:  false,
		Duration:  time.Since(start),
	}, nil
}

// BuildAndActivateSlot builds entry (via the tree-hash build cache), stages the
// binary into yoke/pkg/shape/slot, and activates it atomically. On first
// activation the built binary is also retained as the slot's Release baseline
// so Undo and Restore("release") work immediately.
func BuildAndActivateSlot(ctx context.Context, entry Entry, opts BuildOptions) (BuildResult, error) {
	res, err := BuildToCache(ctx, entry, opts)
	if err != nil {
		return res, err
	}
	home, err := opts.home()
	if err != nil {
		return res, err
	}
	rev := opts.Revision
	if rev == "" {
		rev = "tree:" + res.TreeHash[:12]
	}
	store := slot.Store{
		Home:            home,
		App:             entry.Name,
		ReleaseRevision: rev,
	}
	ver, err := store.Stage(res.CachePath, rev, opts.Evidence)
	if err != nil {
		return res, fmt.Errorf("cmdgen: stage slot %s: %w", entry.Name, err)
	}
	st, err := store.Activate(ver, res.CachePath)
	if err != nil {
		return res, fmt.Errorf("cmdgen: activate slot %s: %w", entry.Name, err)
	}
	res.Version = ver
	res.State = st
	return res, nil
}

func resolvedLocalModules(opts BuildOptions) map[string]string {
	out := map[string]string{}
	if opts.GoWork != "" {
		uses, _ := parseGoWork(opts.GoWork)
		for _, dir := range uses {
			if mod := readModulePath(dir); mod != "" {
				out[mod] = dir
			}
		}
	}
	for mod, dir := range opts.LocalModules {
		if abs, err := filepath.Abs(dir); err == nil {
			out[mod] = abs
		} else {
			out[mod] = dir
		}
	}
	return out
}

func buildGoWorkFile(genWork string, opts BuildOptions) (string, error) {
	if opts.GoWork == "" && len(opts.LocalModules) == 0 {
		return "", nil
	}
	var useDirs []string
	var replaces []string
	goVer := opts.GoMod.GoVersion
	if goVer == "" {
		goVer = "1.27.0"
	}
	toolchain := opts.GoMod.Toolchain

	if opts.GoWork != "" {
		u, r := parseGoWork(opts.GoWork)
		useDirs = append(useDirs, u...)
		replaces = append(replaces, r...)
		if gv, tc := parseGoWorkHeader(opts.GoWork); gv != "" {
			goVer = gv
			if tc != "" {
				toolchain = tc
			}
		}
	}

	// Explicit LocalModules override any go.work directory for the same module.
	if len(opts.LocalModules) > 0 {
		overridden := map[string]bool{}
		for mod := range opts.LocalModules {
			overridden[mod] = true
		}
		filtered := useDirs[:0]
		for _, d := range useDirs {
			if m := readModulePath(d); overridden[m] {
				continue
			}
			filtered = append(filtered, d)
		}
		useDirs = filtered
		var mods []string
		for mod := range opts.LocalModules {
			mods = append(mods, mod)
		}
		sort.Strings(mods)
		for _, mod := range mods {
			dir := opts.LocalModules[mod]
			if abs, err := filepath.Abs(dir); err == nil {
				dir = abs
			}
			if real, err := filepath.EvalSymlinks(dir); err == nil {
				dir = real
			}
			useDirs = append(useDirs, dir)
		}
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, "go %s\n", goVer)
	if toolchain != "" {
		fmt.Fprintf(&b, "\ntoolchain %s\n", toolchain)
	}
	b.WriteString("\nuse (\n")
	b.WriteString("\t.\n")
	seenUse := map[string]bool{}
	for _, d := range useDirs {
		if seenUse[d] {
			continue
		}
		seenUse[d] = true
		fmt.Fprintf(&b, "\t%q\n", d)
	}
	b.WriteString(")\n")
	if len(replaces) > 0 {
		b.WriteByte('\n')
		for _, r := range replaces {
			fmt.Fprintf(&b, "%s\n", r)
		}
	}
	workPath := filepath.Join(genWork, "go.work")
	if err := os.WriteFile(workPath, b.Bytes(), 0o644); err != nil {
		return "", err
	}
	return workPath, nil
}

func parseGoWorkHeader(workPath string) (goVer, toolchain string) {
	b, err := os.ReadFile(workPath)
	if err != nil {
		return "", ""
	}
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if strings.HasPrefix(line, "go ") {
			goVer = strings.TrimSpace(strings.TrimPrefix(line, "go "))
		} else if strings.HasPrefix(line, "toolchain ") {
			toolchain = strings.TrimSpace(strings.TrimPrefix(line, "toolchain "))
		}
	}
	return goVer, toolchain
}

func parseGoWork(workPath string) (uses []string, replaces []string) {
	if abs, err := filepath.Abs(workPath); err == nil {
		workPath = abs
	}
	if real, err := filepath.EvalSymlinks(workPath); err == nil {
		workPath = real
	}
	b, err := os.ReadFile(workPath)
	if err != nil {
		return nil, nil
	}
	baseDir := filepath.Dir(workPath)
	inUse := false
	inReplace := false
	resolveDir := func(p string) string {
		p = strings.Trim(p, `"`)
		if filepath.IsAbs(p) {
			return p
		}
		return filepath.Clean(filepath.Join(baseDir, p))
	}
	rewriteReplace := func(line string) string {
		left, right, ok := strings.Cut(line, "=>")
		if !ok {
			return ""
		}
		rf := strings.Fields(right)
		if len(rf) == 1 && (strings.HasPrefix(rf[0], ".") || strings.HasPrefix(rf[0], "/") || strings.HasPrefix(rf[0], `"./`) || strings.HasPrefix(rf[0], `"../`)) {
			return fmt.Sprintf("replace %s => %q", strings.TrimSpace(left), resolveDir(rf[0]))
		}
		return "replace " + strings.TrimSpace(line)
	}

	for _, raw := range strings.Split(string(b), "\n") {
		line := raw
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if inUse {
			if line == ")" {
				inUse = false
				continue
			}
			uses = append(uses, resolveDir(line))
			continue
		}
		if inReplace {
			if line == ")" {
				inReplace = false
				continue
			}
			if r := rewriteReplace(line); r != "" {
				replaces = append(replaces, r)
			}
			continue
		}
		switch {
		case line == "use (":
			inUse = true
		case strings.HasPrefix(line, "use "):
			uses = append(uses, resolveDir(strings.TrimSpace(strings.TrimPrefix(line, "use "))))
		case line == "replace (":
			inReplace = true
		case strings.HasPrefix(line, "replace "):
			if r := rewriteReplace(strings.TrimSpace(strings.TrimPrefix(line, "replace "))); r != "" {
				replaces = append(replaces, r)
			}
		}
	}
	return uses, replaces
}

func readModulePath(dir string) string {
	b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return ""
	}
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module "))
		}
	}
	return ""
}

func setEnvOverrides(env []string, overrides map[string]string) []string {
	out := make([]string, 0, len(env)+len(overrides))
	for _, kv := range env {
		k, _, ok := strings.Cut(kv, "=")
		if ok {
			if _, replaced := overrides[k]; replaced {
				continue
			}
		}
		out = append(out, kv)
	}
	keys := make([]string, 0, len(overrides))
	for k := range overrides {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		out = append(out, k+"="+overrides[k])
	}
	return out
}
