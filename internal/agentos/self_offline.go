// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/qiangli/yoke/external/gotoolchain"
	ygit "github.com/qiangli/yoke/git"
	"github.com/qiangli/yoke/pkg/artifact"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/qiangli/yoke/pkg/gomod"
	"github.com/qiangli/yoke/pkg/prune"
	"github.com/qiangli/yoke/pkg/prune/rods"
	"github.com/qiangli/yoke/pkg/src"
)

const (
	logicalSourceBashy  = "source/bashy"
	logicalOverlay      = "gooverlay/productgo"
	logicalBinmgrSeed   = "binmgr/seed"
	mediaTypeSourceTar  = "application/x-tar"
	mediaTypeBinmgrSeed = "application/vnd.bashy.binmgr.seed.v1+tar"
	mediaTypeLangPack   = "application/vnd.bashy.lang-pack.v1+json"
)

// selfGoBuildArgsAndEnv resolves the command arguments and environment for
// building ./cmd/bashy, selecting tools/productgo/main.go when present in dir
// (unless BASHY_SKIP_PRODUCTGO=1) and placing BASHY_PRODUCTGO_CACHE outside GOMODCACHE.
func selfGoBuildArgsAndEnv(exe, dir, target, ldflags string, baseEnv []string) ([]string, []string) {
	workDir := strings.TrimSpace(dir)
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	env := append([]string{}, baseEnv...)

	lookupEnv := func(key string) string {
		prefix := key + "="
		val := ""
		for _, kv := range env {
			if strings.HasPrefix(kv, prefix) {
				val = kv[len(prefix):]
			}
		}
		return strings.TrimSpace(val)
	}

	productgoMain := filepath.Join(workDir, "tools", "productgo", "main.go")
	if lookupEnv("BASHY_SKIP_PRODUCTGO") != "1" && isRegularFile(productgoMain) {
		if lookupEnv("BASHY_EXE") == "" && exe != "" {
			env = append(env, "BASHY_EXE="+exe)
		}
		if lookupEnv("BASHY_PRODUCTGO_CACHE") == "" {
			artRoot := lookupEnv("BASHY_ARTIFACTS_DIR")
			if artRoot == "" {
				artRoot = artifact.DefaultRoot()
			}
			cacheDir := filepath.Clean(filepath.Join(artRoot, "..", "cache", "productgo"))
			env = append(env, "BASHY_PRODUCTGO_CACHE="+cacheDir)
		}
		env = append(stripEnvKey(env, "CGO_ENABLED"), "CGO_ENABLED=0")
		args := []string{
			"go", "run", productgoMain,
			runtime.GOOS, runtime.GOARCH, "0",
			"build", "-trimpath", "-ldflags", ldflags, "-o", target, "./cmd/bashy",
		}
		return args, env
	}

	args := []string{"go", "build", "-trimpath", "-ldflags", ldflags, "-o", target, "./cmd/bashy"}
	return args, env
}

func newSelfGoBuildCmd(ctx context.Context, exe, dir, target, ldflags string) *exec.Cmd {
	args, env := selfGoBuildArgsAndEnv(exe, dir, target, ldflags, os.Environ())
	c := exec.CommandContext(ctx, exe, args...)
	if strings.TrimSpace(dir) != "" {
		c.Dir = dir
	}
	c.Env = env
	return c
}

type selfBootstrapResult struct {
	SchemaVersion      string              `json:"schema_version"`
	ArtifactRoot       string              `json:"artifact_root"`
	LoomDir            string              `json:"loom_dir"`
	Generation         uint64              `json:"generation"`
	Platform           string              `json:"platform"`
	ProfileName        string              `json:"profile_name"`
	ProfileDigest      string              `json:"profile_digest"`
	ManifestDigest     string              `json:"manifest_digest"`
	GoToolchainVersion string              `json:"go_toolchain_version"`
	ProductOverlaySHA  string              `json:"product_overlay_sha"`
	RootCount          int                 `json:"root_count"`
	IngestedModules    int                 `json:"ingested_modules"`
	PublishedModules   int                 `json:"published_modules"`
	Manifest           artifact.ManifestV1 `json:"manifest"`
}

func resolveBootstrapSourceDir(explicit string) (string, error) {
	if s := strings.TrimSpace(explicit); s != "" {
		abs, err := filepath.Abs(s)
		if err != nil {
			return "", err
		}
		if !isRegularFile(filepath.Join(abs, "go.mod")) {
			return "", fmt.Errorf("self bootstrap: %s has no go.mod", abs)
		}
		return abs, nil
	}
	if root, ok := findBashySourceRoot("."); ok {
		return root, nil
	}
	home := src.Home()
	if pin := filepath.Join(src.SrcDir(home), "bashy"); isRegularFile(filepath.Join(pin, "go.mod")) {
		return pin, nil
	}
	return "", errors.New("self bootstrap: no source checkout found (pass --source <dir> or run inside a checkout)")
}

func archiveSourceTreeToTempTar(ctx context.Context, srcDir string) (string, int64, string, error) {
	tmp, err := os.CreateTemp("", "bashy-source-tree-*.tar")
	if err != nil {
		return "", 0, "", err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	var relFiles []string
	err = filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p == srcDir {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "bin" || name == "node_modules" || strings.HasPrefix(name, ".tmp") {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, p)
		if err != nil {
			return err
		}
		relFiles = append(relFiles, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", 0, "", err
	}
	slices.Sort(relFiles)

	tw := tar.NewWriter(tmp)
	fixedTime := time.Unix(0, 0).UTC()
	for _, relSlash := range relFiles {
		if err := ctx.Err(); err != nil {
			return "", 0, "", err
		}
		diskPath := filepath.Join(srcDir, filepath.FromSlash(relSlash))
		fi, err := os.Stat(diskPath)
		if err != nil {
			return "", 0, "", err
		}
		hdr := &tar.Header{
			Name:     relSlash,
			Typeflag: tar.TypeReg,
			Mode:     int64(fi.Mode().Perm()),
			Size:     fi.Size(),
			ModTime:  fixedTime,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return "", 0, "", err
		}
		in, err := os.Open(diskPath)
		if err != nil {
			return "", 0, "", err
		}
		_, copyErr := io.Copy(tw, in)
		_ = in.Close()
		if copyErr != nil {
			return "", 0, "", copyErr
		}
	}
	if err := tw.Close(); err != nil {
		return "", 0, "", err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, "", err
	}

	raw, err := os.ReadFile(tmpName)
	if err != nil {
		return "", 0, "", err
	}
	dgst := artifact.DigestBytes(raw)
	cleanup = false
	return tmpName, int64(len(raw)), dgst, nil
}

func extractSourceTarToDir(tarBytes []byte, dstDir string) error {
	cleanDst := filepath.Clean(dstDir)
	tr := tar.NewReader(bytes.NewReader(tarBytes))
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if !artifact.SafeLogicalPath(hdr.Name) {
			return fmt.Errorf("unsafe source archive path %q", hdr.Name)
		}
		outPath := filepath.Join(cleanDst, filepath.FromSlash(hdr.Name))
		if outPath != cleanDst && !strings.HasPrefix(outPath, cleanDst+string(filepath.Separator)) {
			return fmt.Errorf("source archive path escapes destination: %q", hdr.Name)
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
			return err
		}
		mode := os.FileMode(hdr.Mode)&0o777 | 0o600
		f, err := os.OpenFile(outPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(f, tr)
		closeErr := f.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}

type selfBootstrapOptions struct {
	ProfileSpec    string
	ArtifactRoot   string
	LoomDir        string
	SourceDir      string
	PublishModules []string
	GoVersion      string
	GOROOTOverride string
	GoModCache     string
	DiskFreeFunc   func(string) (uint64, error)
}

func runSelfBootstrap(ctx context.Context, opts selfBootstrapOptions) (*selfBootstrapResult, error) {
	artRoot := strings.TrimSpace(opts.ArtifactRoot)
	if artRoot == "" {
		artRoot = artifact.DefaultRoot()
	}
	artRoot, err := filepath.Abs(artRoot)
	if err != nil {
		return nil, err
	}

	loomDir := strings.TrimSpace(opts.LoomDir)
	if loomDir == "" {
		loomDir = gomod.DefaultLoomRepoDir(artRoot)
	}
	loomDir, err = filepath.Abs(loomDir)
	if err != nil {
		return nil, err
	}

	platform := artifact.CurrentPlatform()
	profSpec := strings.TrimSpace(opts.ProfileSpec)
	if profSpec == "" {
		profSpec = artifact.ProfileScopeSelfHost
	}
	profile, err := artifact.ResolveProfile(profSpec, platform)
	if err != nil {
		return nil, err
	}

	// Pre-flight disk capacity reserve check before ingesting artifacts.
	if _, err := artifact.CheckCapacityReserve(artRoot, 0, profile.Capacity.RebuildReserveBytes, opts.DiskFreeFunc); err != nil {
		return nil, err
	}

	srcDir, err := resolveBootstrapSourceDir(opts.SourceDir)
	if err != nil {
		return nil, err
	}

	goVer := strings.TrimPrefix(strings.TrimSpace(opts.GoVersion), "go")
	if goVer == "" {
		goVer = strings.TrimPrefix(strings.TrimSpace(os.Getenv("BASHY_GO_VERSION")), "go")
	}
	if goVer == "" {
		goVer = gotoolchain.DefaultVersion
	}

	var goBin, goroot string
	gorootOverride := strings.TrimSpace(opts.GOROOTOverride)
	if gorootOverride == "" {
		gorootOverride = strings.TrimSpace(os.Getenv("BASHY_BOOTSTRAP_GOROOT"))
	}
	if gorootOverride != "" {
		goroot, err = filepath.Abs(gorootOverride)
		if err != nil {
			return nil, err
		}
		goBin = filepath.Join(goroot, "bin", binmgr.BinaryName("go"))
		if !binmgr.IsExecutable(goBin) {
			return nil, fmt.Errorf("self bootstrap: go executable not found at %s", goBin)
		}
	} else {
		goBin, goroot, err = gotoolchain.Ensure(ctx, goVer)
		if err != nil {
			return nil, err
		}
	}

	backend, err := artifact.ConnectOrOpen(ctx, artRoot)
	if err != nil {
		return nil, err
	}
	defer backend.Close()

	existingManifests, err := backend.ListManifests(ctx)
	if err != nil {
		return nil, err
	}
	var maxGen uint64
	for _, m := range existingManifests {
		if m.Generation > maxGen {
			maxGen = m.Generation
		}
	}
	nextGen := maxGen + 1
	genLabel := fmt.Sprintf("gen-%d", nextGen)

	profBytes, err := profile.CanonicalBytes()
	if err != nil {
		return nil, err
	}
	profDigest := artifact.DigestBytes(profBytes)
	if _, err := backend.PutArtifact(ctx, artifact.CatalogRecordV1{
		SchemaVersion:  artifact.CatalogRecordSchemaV1,
		Digest:         profDigest,
		Size:           int64(len(profBytes)),
		MediaType:      "application/json",
		LogicalName:    "profile/" + profile.Name,
		LogicalVersion: genLabel,
		Provenance:     "self-bootstrap",
		Retention:      artifact.RetentionProtectedRoot,
		RetentionRoots: []string{"manifest:" + genLabel},
	}, bytes.NewReader(profBytes)); err != nil {
		return nil, err
	}

	var roots []artifact.RootDescriptor

	// 1. Archive and ingest Go toolchain seed (gotoolchain/go@<goVer>).
	tmpGoTar, err := os.CreateTemp("", "bashy-go-seed-*.tar")
	if err != nil {
		return nil, err
	}
	tmpGoTarPath := tmpGoTar.Name()
	_ = tmpGoTar.Close()
	defer os.Remove(tmpGoTarPath)

	seedErr := fmt.Errorf("goroot override")
	if gorootOverride == "" {
		seedErr = binmgr.ExportSeedFiltered(ctx, tmpGoTarPath, func(name, version string) bool {
			return name == "go" && version == goVer
		})
	}
	if seedErr != nil {
		if err := binmgr.BuildTreeSeedArchive(ctx, tmpGoTarPath, "go", goVer, "go", goroot); err != nil {
			return nil, fmt.Errorf("self bootstrap: archive go toolchain seed: %w", err)
		}
	}
	goSeedStat, err := os.Stat(tmpGoTarPath)
	if err != nil {
		return nil, err
	}
	goSeedFile, err := os.Open(tmpGoTarPath)
	if err != nil {
		return nil, err
	}
	goSeedBytes, readErr := io.ReadAll(goSeedFile)
	_ = goSeedFile.Close()
	if readErr != nil {
		return nil, readErr
	}
	goSeedDigest := artifact.DigestBytes(goSeedBytes)
	if _, err := backend.PutArtifact(ctx, artifact.CatalogRecordV1{
		SchemaVersion:  artifact.CatalogRecordSchemaV1,
		Digest:         goSeedDigest,
		Size:           goSeedStat.Size(),
		MediaType:      mediaTypeBinmgrSeed,
		Platform:       platform,
		LogicalName:    gotoolchain.LogicalNameToolchain,
		LogicalVersion: goVer,
		Provenance:     "self-bootstrap",
		Retention:      artifact.RetentionProtectedRoot,
		RetentionRoots: []string{"manifest:" + genLabel},
	}, bytes.NewReader(goSeedBytes)); err != nil {
		return nil, err
	}
	roots = append(roots, artifact.RootDescriptor{
		Kind:      "gotoolchain",
		Name:      gotoolchain.LogicalNameToolchain,
		Version:   goVer,
		Platform:  platform,
		MediaType: mediaTypeBinmgrSeed,
		Digest:    goSeedDigest,
		Size:      goSeedStat.Size(),
	})

	// 2. Archive any non-Go binmgr tools if present in CacheDir().
	if cacheDir, cErr := binmgr.CacheDir(); cErr == nil {
		if entries, dErr := os.ReadDir(cacheDir); dErr == nil {
			hasOtherTools := false
			for _, e := range entries {
				n := e.Name()
				if n != "go" && !strings.HasPrefix(n, ".") {
					hasOtherTools = true
					break
				}
			}
			if hasOtherTools {
				tmpToolsTar, tErr := os.CreateTemp("", "bashy-tools-seed-*.tar")
				if tErr == nil {
					tmpToolsPath := tmpToolsTar.Name()
					_ = tmpToolsTar.Close()
					if exErr := binmgr.ExportSeedFiltered(ctx, tmpToolsPath, func(name, _ string) bool {
						return name != "go"
					}); exErr == nil {
						if toolsBytes, rErr := os.ReadFile(tmpToolsPath); rErr == nil && len(toolsBytes) > 0 {
							toolsDigest := artifact.DigestBytes(toolsBytes)
							if _, pErr := backend.PutArtifact(ctx, artifact.CatalogRecordV1{
								SchemaVersion:  artifact.CatalogRecordSchemaV1,
								Digest:         toolsDigest,
								Size:           int64(len(toolsBytes)),
								MediaType:      mediaTypeBinmgrSeed,
								Platform:       platform,
								LogicalName:    logicalBinmgrSeed,
								LogicalVersion: genLabel,
								Provenance:     "self-bootstrap",
								Retention:      artifact.RetentionProtectedRoot,
								RetentionRoots: []string{"manifest:" + genLabel},
							}, bytes.NewReader(toolsBytes)); pErr == nil {
								roots = append(roots, artifact.RootDescriptor{
									Kind:      "binmgr",
									Name:      logicalBinmgrSeed,
									Version:   genLabel,
									Platform:  platform,
									MediaType: mediaTypeBinmgrSeed,
									Digest:    toolsDigest,
									Size:      int64(len(toolsBytes)),
								})
							}
						}
					}
					_ = os.Remove(tmpToolsPath)
				}
			}
		}
	}

	// 3. Ingest Product Go startup-signal overlay (tools/productgo/main.go when present).
	productgoFile := filepath.Join(srcDir, "tools", "productgo", "main.go")
	var overlayBytes []byte
	if raw, oErr := os.ReadFile(productgoFile); oErr == nil {
		overlayBytes = raw
	} else {
		overlayBytes = []byte(fmt.Sprintf(`{"overlay":"default","go_version":%q}`+"\n", goVer))
	}
	overlayDigest := artifact.DigestBytes(overlayBytes)
	if _, err := backend.PutArtifact(ctx, artifact.CatalogRecordV1{
		SchemaVersion:  artifact.CatalogRecordSchemaV1,
		Digest:         overlayDigest,
		Size:           int64(len(overlayBytes)),
		MediaType:      "text/plain; charset=utf-8",
		LogicalName:    logicalOverlay,
		LogicalVersion: goVer,
		Provenance:     "self-bootstrap",
		Retention:      artifact.RetentionProtectedRoot,
		RetentionRoots: []string{"manifest:" + genLabel},
	}, bytes.NewReader(overlayBytes)); err != nil {
		return nil, err
	}
	roots = append(roots, artifact.RootDescriptor{
		Kind:      "gooverlay",
		Name:      logicalOverlay,
		Version:   goVer,
		MediaType: "text/plain; charset=utf-8",
		Digest:    overlayDigest,
		Size:      int64(len(overlayBytes)),
	})

	// 4. Archive and ingest source tree (source/bashy).
	srcVersion := "local"
	if sha, sErr := ygit.ResolveCommit(srcDir, "HEAD"); sErr == nil && len(sha) >= 12 {
		srcVersion = sha[:12]
	}
	srcTarPath, srcTarSize, srcTarDigest, err := archiveSourceTreeToTempTar(ctx, srcDir)
	if err != nil {
		return nil, err
	}
	defer os.Remove(srcTarPath)

	srcTarReader, err := os.Open(srcTarPath)
	if err != nil {
		return nil, err
	}
	_, putSrcErr := backend.PutArtifact(ctx, artifact.CatalogRecordV1{
		SchemaVersion:  artifact.CatalogRecordSchemaV1,
		Digest:         srcTarDigest,
		Size:           srcTarSize,
		MediaType:      mediaTypeSourceTar,
		LogicalName:    logicalSourceBashy,
		LogicalVersion: srcVersion,
		Provenance:     "self-bootstrap",
		Retention:      artifact.RetentionProtectedRoot,
		RetentionRoots: []string{"manifest:" + genLabel},
	}, srcTarReader)
	_ = srcTarReader.Close()
	if putSrcErr != nil {
		return nil, putSrcErr
	}
	roots = append(roots, artifact.RootDescriptor{
		Kind:      "source",
		Name:      logicalSourceBashy,
		Version:   srcVersion,
		MediaType: mediaTypeSourceTar,
		Digest:    srcTarDigest,
		Size:      srcTarSize,
	})

	// 5. Capture Go module dependency closure + optional local module publications.
	modRes, err := gomod.CaptureModuleClosure(ctx, gomod.ModuleClosureOptions{
		ModuleDirs:   []string{srcDir},
		PublishDirs:  opts.PublishModules,
		GoBin:        goBin,
		GOROOT:       goroot,
		GoModCache:   opts.GoModCache,
		ArtifactRoot: artRoot,
		LoomRepoDir:  loomDir,
		Backend:      backend,
	})
	if err != nil {
		return nil, err
	}
	roots = append(roots, modRes.Roots...)

	// 6. Emit mandatory language-pack roots declared in profile.Languages.
	for _, lang := range profile.Languages {
		lang = strings.TrimSpace(lang)
		if lang == "" {
			continue
		}
		var deps []string
		switch lang {
		case "go":
			deps = []string{goSeedDigest, overlayDigest}
		case "bashsharp":
			deps = []string{srcTarDigest}
		}
		lpBytes, _ := json.Marshal(map[string]any{
			"schema":     "bashy-lang-pack-v1",
			"language":   lang,
			"generation": nextGen,
			"platform":   platform,
			"deps":       deps,
		})
		lpDigest := artifact.DigestBytes(lpBytes)
		if _, err := backend.PutArtifact(ctx, artifact.CatalogRecordV1{
			SchemaVersion:  artifact.CatalogRecordSchemaV1,
			Digest:         lpDigest,
			Size:           int64(len(lpBytes)),
			MediaType:      mediaTypeLangPack,
			LogicalName:    "lang-pack/" + lang,
			LogicalVersion: genLabel,
			Dependencies:   deps,
			Provenance:     "self-bootstrap",
			Retention:      artifact.RetentionProtectedRoot,
			RetentionRoots: []string{"manifest:" + genLabel},
		}, bytes.NewReader(lpBytes)); err != nil {
			return nil, err
		}
		roots = append(roots, artifact.RootDescriptor{
			Kind:      "lang-pack",
			Name:      lang,
			Version:   genLabel,
			Platform:  platform,
			MediaType: mediaTypeLangPack,
			Digest:    lpDigest,
			Size:      int64(len(lpBytes)),
			Deps:      deps,
		})
	}

	// Re-check disk capacity reserve after staging all closure blobs.
	if _, err := artifact.CheckCapacityReserve(artRoot, 0, profile.Capacity.RebuildReserveBytes, opts.DiskFreeFunc); err != nil {
		return nil, err
	}

	manifest := artifact.ManifestV1{
		SchemaVersion:       artifact.ManifestSchemaV1,
		Generation:          nextGen,
		CreatedAt:           nowUTC(),
		Platform:            platform,
		ProfileName:         profile.Name,
		ProfileDigest:       profDigest,
		GoToolchainVersion:  goVer,
		ProductOverlaySHA:   overlayDigest,
		RebuildReserveBytes: profile.Capacity.RebuildReserveBytes,
		Roots:               roots,
	}
	if err := backend.PutManifest(ctx, manifest); err != nil {
		return nil, err
	}
	if err := artifact.SaveProfile(artRoot, profile); err != nil {
		return nil, err
	}

	manifestDigest, err := manifest.Digest()
	if err != nil {
		return nil, err
	}

	return &selfBootstrapResult{
		SchemaVersion:      "bashy-self-bootstrap-v1",
		ArtifactRoot:       artRoot,
		LoomDir:            loomDir,
		Generation:         nextGen,
		Platform:           platform,
		ProfileName:        profile.Name,
		ProfileDigest:      profDigest,
		ManifestDigest:     manifestDigest,
		GoToolchainVersion: goVer,
		ProductOverlaySHA:  overlayDigest,
		RootCount:          len(roots),
		IngestedModules:    modRes.Ingested,
		PublishedModules:   len(modRes.Published),
		Manifest:           manifest,
	}, nil
}

func materializeImportedTools(ctx context.Context, backend artifact.Backend, manifest artifact.ManifestV1) error {
	for _, r := range manifest.Roots {
		if r.Kind != "gotoolchain" && r.Kind != "binmgr" {
			continue
		}
		rc, err := backend.OpenBlob(ctx, r.Digest)
		if err != nil {
			return err
		}
		tmp, err := os.CreateTemp("", "bashy-import-seed-*.tar")
		if err != nil {
			_ = rc.Close()
			return err
		}
		tmpPath := tmp.Name()
		_, copyErr := io.Copy(tmp, rc)
		closeRC := rc.Close()
		closeTmp := tmp.Close()
		if copyErr != nil || closeRC != nil || closeTmp != nil {
			_ = os.Remove(tmpPath)
			return errors.Join(copyErr, closeRC, closeTmp)
		}
		impErr := binmgr.ImportSeed(ctx, tmpPath)
		_ = os.Remove(tmpPath)
		if impErr != nil {
			return impErr
		}
	}
	return nil
}

type selfVerifyResult struct {
	Report        *artifact.VerifyReport `json:"report"`
	RebuildTested bool                   `json:"rebuild_tested"`
	RebuiltBinary string                 `json:"rebuilt_binary,omitempty"`
}

func runOfflineRebuildCheck(ctx context.Context, artRoot, loomDir, explicitSource string, manifest *artifact.ManifestV1) (string, error) {
	if manifest == nil {
		return "", fmt.Errorf("%w: no manifest available for offline rebuild", artifact.ErrIncompleteClosure)
	}
	tmpWork, err := os.MkdirTemp("", "bashy-verify-rebuild-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpWork)

	srcDir := strings.TrimSpace(explicitSource)
	if srcDir == "" {
		// Extract the protected source root from CAS so --rebuild works even on a clean recovery host without a git checkout.
		var srcDigest string
		for _, r := range manifest.Roots {
			if r.Kind == "source" {
				srcDigest = r.Digest
				break
			}
		}
		if srcDigest == "" {
			return "", fmt.Errorf("%w: no source root in manifest", artifact.ErrIncompleteClosure)
		}
		store, err := artifact.OpenStore(ctx, artifact.StoreOptions{Root: artRoot, ReadOnly: true})
		if err != nil {
			return "", err
		}
		srcBytes, fetchErr := store.FetchVerified(ctx, srcDigest)
		_ = store.Close()
		if fetchErr != nil {
			return "", fetchErr
		}
		srcDir = filepath.Join(tmpWork, "src")
		if err := os.MkdirAll(srcDir, 0o755); err != nil {
			return "", err
		}
		if err := extractSourceTarToDir(srcBytes, srcDir); err != nil {
			return "", err
		}
	} else {
		srcDir, err = filepath.Abs(srcDir)
		if err != nil {
			return "", err
		}
	}

	// Provision toolchain offline (restoring from CAS if binmgr cache is empty).
	prevArtEnv := os.Getenv("BASHY_ARTIFACTS_DIR")
	prevOffEnv := os.Getenv("BASHY_OFFLINE")
	_ = os.Setenv("BASHY_ARTIFACTS_DIR", artRoot)
	_ = os.Setenv("BASHY_OFFLINE", "1")
	goBin, goroot, err := gotoolchain.Ensure(ctx, manifest.GoToolchainVersion)
	if prevArtEnv == "" {
		_ = os.Unsetenv("BASHY_ARTIFACTS_DIR")
	} else {
		_ = os.Setenv("BASHY_ARTIFACTS_DIR", prevArtEnv)
	}
	if prevOffEnv == "" {
		_ = os.Unsetenv("BASHY_OFFLINE")
	} else {
		_ = os.Setenv("BASHY_OFFLINE", prevOffEnv)
	}
	if err != nil {
		return "", err
	}

	goCache := filepath.Join(tmpWork, "gocache")
	modCache := filepath.Join(tmpWork, "gomodcache")
	if err := os.MkdirAll(goCache, 0o755); err != nil {
		return "", err
	}
	if err := os.MkdirAll(modCache, 0o755); err != nil {
		return "", err
	}
	defer func() {
		_ = filepath.WalkDir(modCache, func(p string, _ fs.DirEntry, _ error) error {
			return os.Chmod(p, 0o755)
		})
	}()

	baseEnv := append(os.Environ(),
		"BASHY_ARTIFACTS_DIR="+artRoot,
		gomod.LoomEnv+"="+loomDir,
		"GOCACHE="+goCache,
		"GOMODCACHE="+modCache,
		"GOWORK=off",
		"CGO_ENABLED=0",
		"GOFLAGS=-mod=readonly -buildvcs=false",
		"GOPROXY=https://proxy.golang.org,direct", // intentionally poisoned to verify SealedCommandEnv strips it!
	)
	sealedEnv, cleanup, err := gotoolchain.SealedCommandEnv(ctx, baseEnv, goroot, artRoot)
	if err != nil {
		return "", err
	}
	defer cleanup()

	outBin := filepath.Join(tmpWork, releaseBinaryName())
	pkgTarget := "./cmd/bashy"
	if !isDirPath(filepath.Join(srcDir, "cmd", "bashy")) {
		pkgTarget = "."
	}

	productgoMain := filepath.Join(srcDir, "tools", "productgo", "main.go")
	var buildCmd *exec.Cmd
	if pkgTarget == "./cmd/bashy" && os.Getenv("BASHY_SKIP_PRODUCTGO") != "1" && isRegularFile(productgoMain) {
		productCache := filepath.Join(tmpWork, "productgo-cache")
		runEnv := append(sealedEnv, "BASHY_PRODUCTGO_CACHE="+productCache)
		runEnv = stripEnvKey(runEnv, "GOFLAGS")
		runEnv = append(runEnv, "GOFLAGS=-mod=readonly -buildvcs=false")
		// Ensure goBin directory is at front of PATH so productgo's `go env` probe finds the sealed GOROOT go binary.
		pathVal := filepath.Dir(goBin) + string(os.PathListSeparator) + os.Getenv("PATH")
		runEnv = append(stripEnvKey(stripEnvKey(runEnv, "PATH"), "BASHY_EXE"), "PATH="+pathVal)
		buildCmd = exec.CommandContext(ctx, goBin, "run", productgoMain, runtime.GOOS, runtime.GOARCH, "0", "build", "-trimpath", "-o", outBin, pkgTarget)
		buildCmd.Env = runEnv
	} else {
		buildCmd = exec.CommandContext(ctx, goBin, "build", "-trimpath", "-o", outBin, pkgTarget)
		buildCmd.Env = sealedEnv
	}
	buildCmd.Dir = srcDir
	var stderr bytes.Buffer
	buildCmd.Stderr = &stderr
	if err := buildCmd.Run(); err != nil {
		return "", fmt.Errorf("offline rebuild failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	fi, err := os.Stat(outBin)
	if err != nil || fi.Size() == 0 {
		return "", fmt.Errorf("offline rebuild produced no output binary at %s", outBin)
	}
	return outBin, nil
}

func selfBootstrapCmd() *cobra.Command {
	var profileSpec, artifactRoot, sourceDir, loomDir, goVersion string
	var publishModules []string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Capture the self-contained offline closure into the local artifact store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := runSelfBootstrap(cmd.Context(), selfBootstrapOptions{
				ProfileSpec:    profileSpec,
				ArtifactRoot:   artifactRoot,
				LoomDir:        loomDir,
				SourceDir:      sourceDir,
				PublishModules: publishModules,
				GoVersion:      goVersion,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "bootstrapped generation %d (%s, %d roots, %d modules) -> %s\n",
				res.Generation, res.ProfileName, res.RootCount, res.IngestedModules+res.PublishedModules, res.ArtifactRoot)
			return nil
		},
	}
	cmd.Flags().StringVar(&profileSpec, "profile", artifact.ProfileScopeSelfHost, "Offline profile: self-host, default, or path to profile JSON")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&sourceDir, "source", "", "Source checkout directory to capture")
	cmd.Flags().StringSliceVar(&publishModules, "publish-module", nil, "Local module directory to publish into durable Loom + CAS")
	cmd.Flags().StringVar(&loomDir, "loom-dir", "", "Durable bare Loom Git repository directory")
	cmd.Flags().StringVar(&goVersion, "go-version", "", "Go toolchain version to provision and archive")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func selfSealCmd() *cobra.Command {
	var artifactRoot, profileSpec, loomDir string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "seal",
		Short: "Verify closure completeness and disk reserve, then seal offline mode",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := artifact.Seal(cmd.Context(), artifact.SealOptions{
				Root:        artifactRoot,
				LoomDir:     loomDir,
				ProfileSpec: profileSpec,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(st)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sealed generation %d (baseline %d, profile %s)\n",
				st.ActiveGeneration, st.BaselineGeneration, st.ProfileName)
			return nil
		},
	}
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&loomDir, "loom-dir", "", "Durable bare Loom Git repository directory")
	cmd.Flags().StringVar(&profileSpec, "profile", "", "Profile override (self-host, default, or JSON path)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func selfVerifyCmd() *cobra.Command {
	var offline, rebuild, asJSON bool
	var artifactRoot, loomDir, sourceDir, profileSpec string

	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify offline closure digests and optionally test a clean-cache offline rebuild",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			artRoot := strings.TrimSpace(artifactRoot)
			if artRoot == "" {
				artRoot = artifact.DefaultRoot()
			}
			artRoot, err := filepath.Abs(artRoot)
			if err != nil {
				return err
			}
			lDir := strings.TrimSpace(loomDir)
			if lDir == "" {
				lDir = gomod.DefaultLoomRepoDir(artRoot)
			}

			report, err := artifact.VerifyClosure(cmd.Context(), artifact.VerifyOptions{
				Root:          artRoot,
				LoomDir:       lDir,
				ProfileSpec:   profileSpec,
				VerifyDigests: true,
			})
			if err != nil {
				return err
			}
			if !report.Status.ClosureComplete {
				return fmt.Errorf("%w: %d missing or corrupt artifact(s)", artifact.ErrIncompleteClosure, len(report.Missing))
			}
			if !report.Status.CapacityOK {
				return fmt.Errorf("%w: %s", artifact.ErrInsufficientCapacity, report.Status.CapacityError)
			}

			res := selfVerifyResult{Report: report}
			if rebuild {
				rebuilt, rbErr := runOfflineRebuildCheck(cmd.Context(), artRoot, lDir, sourceDir, report.Status.Manifest)
				if rbErr != nil {
					return rbErr
				}
				res.RebuildTested = true
				res.RebuiltBinary = rebuilt
			}
			_ = offline // VerifyDigests is always enforced; --offline is accepted explicitly for operator clarity.

			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "verified generation %d (%d blobs SHA-256 verified, rebuild=%t)\n",
				report.Status.ActiveGeneration, report.VerifiedBlobs, res.RebuildTested)
			return nil
		},
	}
	cmd.Flags().BoolVar(&offline, "offline", false, "Verify offline readiness with zero network access")
	cmd.Flags().BoolVar(&rebuild, "rebuild", false, "Run a clean-cache hermetic Go rebuild from retained closure")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&loomDir, "loom-dir", "", "Durable bare Loom Git repository directory")
	cmd.Flags().StringVar(&sourceDir, "source", "", "Optional source directory override for --rebuild")
	cmd.Flags().StringVar(&profileSpec, "profile", "", "Profile override (self-host, default, or JSON path)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func selfStatusCmd() *cobra.Command {
	var artifactRoot, loomDir, profileSpec string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "status",
		Short: "Report offline seal state, active/baseline generations, capacity reserve, and missing artifacts",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			st, err := artifact.InspectStatus(cmd.Context(), artifact.InspectStatusOptions{
				Root:        artifactRoot,
				LoomDir:     loomDir,
				ProfileSpec: profileSpec,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(st)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "sealed: %t  profile: %s  active_gen: %d  baseline_gen: %d  complete: %t  capacity_ok: %t\n",
				st.Sealed, st.Profile.Name, st.ActiveGeneration, st.BaselineGeneration, st.ClosureComplete, st.CapacityOK)
			for _, m := range st.MissingArtifacts {
				fmt.Fprintf(cmd.OutOrStdout(), "  missing: %s %s %s (%s)\n", m.Kind, m.Name, m.Version, m.Reason)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&loomDir, "loom-dir", "", "Durable bare Loom Git repository directory")
	cmd.Flags().StringVar(&profileSpec, "profile", "", "Profile override (self-host, default, or JSON path)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func selfExportCmd() *cobra.Command {
	var generation uint64
	var artifactRoot, loomDir string
	var asJSON bool

	cmd := &cobra.Command{
		Use:   "export <bundle.tar>",
		Short: "Export a portable, checksummed ORAS OCI + Loom self-recovery archive",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := artifact.ExportBundle(cmd.Context(), artifact.ExportOptions{
				Root:       artifactRoot,
				LoomDir:    loomDir,
				OutputPath: args[0],
				Generation: generation,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s (%s, gen %d, %d blobs)\n",
				res.OutputPath, res.BundleSHA256, res.Generation, res.BlobCount)
			return nil
		},
	}
	cmd.Flags().Uint64Var(&generation, "generation", 0, "Manifest generation to export (default active/latest)")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&loomDir, "loom-dir", "", "Durable bare Loom Git repository directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func selfImportCmd() *cobra.Command {
	var sealAfter, allowForeign, asJSON bool
	var artifactRoot, loomDir string

	cmd := &cobra.Command{
		Use:   "import <bundle.tar>",
		Short: "Verify and import a portable self-recovery archive as an additive generation",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			res, err := artifact.ImportBundle(cmd.Context(), artifact.ImportOptions{
				Root:                 artifactRoot,
				LoomDir:              loomDir,
				BundlePath:           args[0],
				Seal:                 sealAfter,
				AllowForeignPlatform: allowForeign,
				Materialize:          materializeImportedTools,
			})
			if err != nil {
				return err
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "imported generation %d into %s (sealed=%t, %d blobs)\n",
				res.ImportedGeneration, res.ArtifactRoot, res.Sealed, res.BlobsImported)
			return nil
		},
	}
	cmd.Flags().BoolVar(&sealAfter, "seal", false, "Seal the destination artifact store after verified import")
	cmd.Flags().BoolVar(&allowForeign, "allow-foreign-platform", false, "Allow importing a bundle built for a different OS/arch")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().StringVar(&loomDir, "loom-dir", "", "Durable bare Loom Git repository directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}

func forwardPruneArgs(older string, apply, dryRun, asJSON bool) []string {
	var args []string
	if older != "" {
		args = append(args, "--older", older)
	}
	if apply {
		args = append(args, "--apply")
	}
	if dryRun {
		args = append(args, "--dry-run")
	}
	if asJSON {
		args = append(args, "--json")
	}
	return args
}

func selfPruneCmd() *cobra.Command {
	var artifactsOnly, toolsOnly, apply, dryRun, asJSON bool
	var older, artifactRoot string

	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Reclaim evictable CAS payloads and superseded tool versions without touching protected roots",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apply && dryRun {
				return errors.New("--apply and --dry-run cannot be combined")
			}
			runArtifacts := artifactsOnly || !toolsOnly
			runTools := toolsOnly || !artifactsOnly

			makeArtifactCmd := func() *cobra.Command {
				if strings.TrimSpace(artifactRoot) != "" {
					return prune.NewNounCommand(artifact.Pruner{Root: artifactRoot})
				}
				return rods.NewArtifactPruneCommand()
			}

			// Single-noun delegation directly reuses rods.NewArtifactPruneCommand / rods.NewSelfPruneCommand.
			if runArtifacts && !runTools {
				sub := makeArtifactCmd()
				sub.SetContext(cmd.Context())
				sub.SetOut(cmd.OutOrStdout())
				sub.SetErr(cmd.ErrOrStderr())
				sub.SetArgs(forwardPruneArgs(older, apply, dryRun, asJSON))
				return sub.Execute()
			}
			if runTools && !runArtifacts {
				sub := rods.NewSelfPruneCommand()
				sub.SetContext(cmd.Context())
				sub.SetOut(cmd.OutOrStdout())
				sub.SetErr(cmd.ErrOrStderr())
				sub.SetArgs(forwardPruneArgs(older, apply, dryRun, asJSON))
				return sub.Execute()
			}

			// Combined prune across both nouns via their rods commands.
			var results []prune.Result
			var total prune.Counts
			for _, sub := range []*cobra.Command{makeArtifactCmd(), rods.NewSelfPruneCommand()} {
				var buf bytes.Buffer
				sub.SetContext(cmd.Context())
				sub.SetOut(&buf)
				sub.SetErr(cmd.ErrOrStderr())
				sub.SetArgs(forwardPruneArgs(older, apply, dryRun, true))
				if err := sub.Execute(); err != nil {
					return err
				}
				var r prune.Result
				if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
					return err
				}
				results = append(results, r)
				total.Removed += r.Counts.Removed
				total.Bytes += r.Counts.Bytes
				if !asJSON {
					prune.PrintItems(cmd.OutOrStdout(), r.Items, apply, r.Counts)
				}
			}
			if asJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(map[string]any{
					"applied": apply,
					"results": results,
					"counts":  total,
				})
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&artifactsOnly, "artifacts", false, "Prune only evictable CAS artifact payloads")
	cmd.Flags().BoolVar(&toolsOnly, "tools", false, "Prune only superseded managed tool versions")
	cmd.Flags().StringVar(&older, "older", "", "Only prune items older than duration (e.g. 30d)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Remove eligible items after rechecking protected roots and leases")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "List eligible items without removing them (default)")
	cmd.Flags().StringVar(&artifactRoot, "artifact-root", "", "Artifact store root directory")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit structured JSON")
	return cmd
}
