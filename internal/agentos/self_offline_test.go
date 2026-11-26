// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"golang.org/x/mod/module"
	modzip "golang.org/x/mod/zip"

	ygit "github.com/qiangli/yoke/git"
	"github.com/qiangli/yoke/pkg/artifact"
	"github.com/qiangli/yoke/pkg/binmgr"
	"github.com/qiangli/yoke/pkg/gomod"
	"github.com/qiangli/yoke/pkg/prune"
)

func isolateOfflineAgentOSEnv(t *testing.T, base string) (homeDir, artRoot, binCache string) {
	t.Helper()
	homeDir = filepath.Join(base, "bashy-home")
	artRoot = filepath.Join(base, "artifacts")
	binCache = filepath.Join(base, "bin-cache")
	t.Setenv("BASHY_HOME", homeDir)
	t.Setenv("BASHY_ARTIFACTS_DIR", artRoot)
	t.Setenv("BASHY_BIN_CACHE", binCache)
	t.Setenv("BASHY_FLEET_DIR", filepath.Join(base, "fleet"))
	t.Setenv("BASHY_TOOLS_DIR", filepath.Join(base, "tools"))
	t.Setenv("BASHY_COMMANDS_DIR", filepath.Join(base, "commands"))
	t.Setenv("BASHY_TOOLS_PATH", "")
	t.Setenv("BASHY_OFFLINE", "")
	t.Setenv("BASHY_LOOM_DIR", "")
	return homeDir, artRoot, binCache
}

func removeGoModCacheAll(dir string) error {
	_ = filepath.Walk(dir, func(p string, _ os.FileInfo, _ error) error {
		return os.Chmod(p, 0o777)
	})
	return os.RemoveAll(dir)
}

func makeMinimalGoSDKCopy(t *testing.T, realGoroot string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "min-goroot")
	toolArchDir := filepath.Join("pkg", "tool", runtime.GOOS+"_"+runtime.GOARCH)
	for _, sub := range []string{"bin", "src", filepath.Join("pkg", "include"), toolArchDir} {
		srcSub := filepath.Join(realGoroot, sub)
		if _, err := os.Stat(srcSub); err != nil {
			continue
		}
		dstSub := filepath.Join(dst, sub)
		if err := os.MkdirAll(filepath.Dir(dstSub), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.CopyFS(dstSub, os.DirFS(srcSub)); err != nil {
			_ = os.RemoveAll(dstSub)
			return realGoroot
		}
	}
	for _, rootFile := range []string{"go.env", "VERSION"} {
		if data, err := os.ReadFile(filepath.Join(realGoroot, rootFile)); err == nil {
			_ = os.WriteFile(filepath.Join(dst, rootFile), data, 0o644)
		}
	}
	return dst
}

func TestSelfProductGoOverlaySelection(t *testing.T) {
	dirWithOverlay := t.TempDir()
	productgoMain := filepath.Join(dirWithOverlay, "tools", "productgo", "main.go")
	if err := os.MkdirAll(filepath.Dir(productgoMain), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(productgoMain, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	artRoot := filepath.Join(t.TempDir(), "root", "artifacts")
	modCache := filepath.Join(t.TempDir(), "gomodcache")
	baseEnv := []string{
		"BASHY_ARTIFACTS_DIR=" + artRoot,
		"GOMODCACHE=" + modCache,
	}

	args, env := selfGoBuildArgsAndEnv("/fake/bashy", dirWithOverlay, "/out/bashy", "-s -w", baseEnv)
	if len(args) < 7 || args[0] != "go" || args[1] != "run" || args[2] != productgoMain || args[3] != runtime.GOOS || args[4] != runtime.GOARCH || args[5] != "0" || args[6] != "build" {
		t.Fatalf("unexpected productgo argv: %v", args)
	}
	var gotCache, gotExe string
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, "BASHY_PRODUCTGO_CACHE="); ok {
			gotCache = v
		}
		if v, ok := strings.CutPrefix(kv, "BASHY_EXE="); ok {
			gotExe = v
		}
	}
	if gotCache == "" || strings.HasPrefix(gotCache, modCache) {
		t.Fatalf("BASHY_PRODUCTGO_CACHE = %q, must be non-empty and outside GOMODCACHE %q", gotCache, modCache)
	}
	if gotExe != "/fake/bashy" {
		t.Fatalf("BASHY_EXE = %q, want /fake/bashy", gotExe)
	}

	// Fallback when tools/productgo/main.go is absent or BASHY_SKIP_PRODUCTGO=1.
	dirPlain := t.TempDir()
	plainArgs, _ := selfGoBuildArgsAndEnv("/fake/bashy", dirPlain, "/out/bashy", "-s -w", baseEnv)
	if len(plainArgs) < 3 || plainArgs[0] != "go" || plainArgs[1] != "build" || plainArgs[2] != "-trimpath" {
		t.Fatalf("expected fallback go build -trimpath, got %v", plainArgs)
	}

	skipArgs, _ := selfGoBuildArgsAndEnv("/fake/bashy", dirWithOverlay, "/out/bashy", "-s -w", append(baseEnv, "BASHY_SKIP_PRODUCTGO=1"))
	if len(skipArgs) < 2 || skipArgs[1] != "build" {
		t.Fatalf("expected BASHY_SKIP_PRODUCTGO=1 fallback to go build, got %v", skipArgs)
	}
}

func TestSelfSealedNetworkGuardsDenyUncachedReleaseAndSource(t *testing.T) {
	homeDir, artRoot, _ := isolateOfflineAgentOSEnv(t, t.TempDir())

	if err := artifact.SaveSealState(artRoot, artifact.SealStateV1{
		SchemaVersion:    artifact.SealStateSchemaV1,
		Sealed:           true,
		ActiveGeneration: 1,
	}); err != nil {
		t.Fatalf("SaveSealState: %v", err)
	}

	if _, err := ensureBashyRelease(context.Background(), "v99.99.99-uncached"); !errors.Is(err, artifact.ErrSealedNetworkDenied) {
		t.Fatalf("ensureBashyRelease uncached in sealed mode = %v, want ErrSealedNetworkDenied", err)
	}

	if err := adapterSourceFetch(homeDir); !errors.Is(err, artifact.ErrSealedNetworkDenied) {
		t.Fatalf("adapterSourceFetch uncached in sealed mode = %v, want ErrSealedNetworkDenied", err)
	}
}

func runSelfCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := selfCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestSelfBootstrapStatusSealVerifyExportImportPruneRoundTrip(t *testing.T) {
	hostGo, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	gorootOut, err := exec.Command(hostGo, "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	goVerOut, err := exec.Command(hostGo, "env", "GOVERSION").Output()
	if err != nil {
		t.Fatalf("go env GOVERSION: %v", err)
	}
	realGoroot := strings.TrimSpace(string(gorootOut))
	hostGoVer := strings.TrimPrefix(strings.TrimSpace(string(goVerOut)), "go")
	minSDK := makeMinimalGoSDKCopy(t, realGoroot)

	base1 := t.TempDir()
	_, artRoot1, binCache1 := isolateOfflineAgentOSEnv(t, base1)
	t.Setenv("BASHY_BOOTSTRAP_GOROOT", minSDK)

	// Seed an extra managed tool in binCache1 to verify binmgr/seed round-trip too.
	extraToolDir := filepath.Join(binCache1, "helper-cli", "v1.0.0")
	if err := os.MkdirAll(extraToolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extraToolDir, binmgr.BinaryName("helper-cli")), []byte("helper-binary"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Create a local sibling git module + an external module in seed GOMODCACHE.
	sibDir := filepath.Join(t.TempDir(), "sib-greeting")
	const sibMod = "example.com/bashy/sib"
	if _, err := ygit.Init(ygit.InitOptions{Path: sibDir, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibDir, "go.mod"), []byte("module "+sibMod+"\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sibDir, "sib.go"), []byte("package sib\n\nfunc Banner() string { return \"self-host-ok\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ygit.Add(ygit.AddOptions{RepoPath: sibDir, All: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := ygit.Commit(ygit.CommitOptions{RepoPath: sibDir, Message: "init", AuthorName: "test", AuthorEmail: "test@local.invalid"}); err != nil {
		t.Fatal(err)
	}

	// Check `self status --json` BEFORE any bootstrap or catalog creation.
	stOutBefore, err := runSelfCLI(t, "status", "--artifact-root", artRoot1, "--json")
	if err != nil {
		t.Fatalf("self status before bootstrap: %v\n%s", err, stOutBefore)
	}
	var statusBefore artifact.SystemStatus
	if err := json.Unmarshal([]byte(stOutBefore), &statusBefore); err != nil {
		t.Fatalf("decode statusBefore: %v", err)
	}
	if statusBefore.ClosureComplete || statusBefore.Sealed || len(statusBefore.MissingArtifacts) == 0 {
		t.Fatalf("expected unsealed incomplete status before bootstrap, got %+v", statusBefore)
	}

	// Pre-publish sibDir into artRoot1 Loom + CAS to learn its exact pseudo-version & hashes,
	// so `--publish-module` during `self bootstrap` exercises idempotent Loom publication AND
	// `srcCheckout` requires BOTH the Loom-published module and the external cached module!
	sibPub, err := gomod.PublishArtifact(context.Background(), gomod.PublishOptions{
		Dir:          sibDir,
		ArtifactRoot: artRoot1,
	})
	if err != nil {
		t.Fatalf("PublishArtifact sib: %v", err)
	}

	extSrc := t.TempDir()
	const extMod = "example.com/bashy/ext"
	const extVer = "v1.0.0"
	extGoMod := []byte("module " + extMod + "\n\ngo 1.22\n")
	_ = os.WriteFile(filepath.Join(extSrc, "go.mod"), extGoMod, 0o644)
	_ = os.WriteFile(filepath.Join(extSrc, "ext.go"), []byte("package ext\n\nfunc Suffix() string { return \"verified\" }\n"), 0o644)
	var extZip bytes.Buffer
	if err := modzip.CreateFromDir(&extZip, module.Version{Path: extMod, Version: extVer}, extSrc); err != nil {
		t.Fatal(err)
	}
	seedModCache := filepath.Join(t.TempDir(), "seed-gomodcache")
	t.Cleanup(func() { _ = removeGoModCacheAll(seedModCache) })
	escExt, _ := module.EscapePath(extMod)
	extVDir := filepath.Join(seedModCache, "cache", "download", filepath.FromSlash(escExt), "@v")
	if err := os.MkdirAll(extVDir, 0o755); err != nil {
		t.Fatal(err)
	}
	extInfo, _ := json.Marshal(struct{ Version, Time string }{extVer, time.Unix(0, 0).UTC().Format(time.RFC3339)})
	_ = os.WriteFile(filepath.Join(extVDir, extVer+".info"), extInfo, 0o644)
	_ = os.WriteFile(filepath.Join(extVDir, extVer+".mod"), extGoMod, 0o644)
	_ = os.WriteFile(filepath.Join(extVDir, extVer+".zip"), extZip.Bytes(), 0o644)

	// Start temporary proxy over artRoot1 + file:// fallback? Even simpler: compute ext hashes via gomod.HashGoModBytes + dirhash, or run tidy against StartProxyServer!
	proxySrv, err := gomod.StartProxyServer(context.Background(), gomod.ProxyServerOptions{
		Addr:         "127.0.0.1:0",
		ArtifactRoot: artRoot1,
		Sealed:       true,
	})
	if err != nil {
		t.Fatalf("StartProxyServer: %v", err)
	}

	srcCheckout := filepath.Join(t.TempDir(), "bashy-src")
	if err := os.MkdirAll(filepath.Join(srcCheckout, "cmd", "bashy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(srcCheckout, "tools", "productgo"), 0o755); err != nil {
		t.Fatal(err)
	}
	driverSrc := `package main

import (
	"os"
	"os/exec"
)

func main() {
	if os.Getenv("BASHY_PRODUCTGO_CACHE") == "" {
		os.Exit(2)
	}
	args := os.Args[4:]
	cmd := exec.Command("go", args...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		os.Exit(1)
	}
}
`
	if err := os.WriteFile(filepath.Join(srcCheckout, "tools", "productgo", "main.go"), []byte(driverSrc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcCheckout, "go.mod"), []byte(fmt.Sprintf(
		"module example.com/bashy/selfhost\n\ngo 1.22\n\nrequire (\n\t%s %s\n\t%s %s\n)\n",
		sibMod, sibPub.Version, extMod, extVer,
	)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcCheckout, "cmd", "bashy", "main.go"), []byte(`package main

import (
	"fmt"
	"example.com/bashy/ext"
	"example.com/bashy/sib"
)

func main() {
	fmt.Println(sib.Banner() + ":" + ext.Suffix())
}
`), 0o644); err != nil {
		t.Fatal(err)
	}

	tidy := exec.Command(hostGo, "mod", "tidy")
	tidy.Dir = srcCheckout
	tidy.Env = append(os.Environ(),
		"GOWORK=off",
		"GOMODCACHE="+seedModCache,
		"GOPROXY="+proxySrv.URL()+",file://"+filepath.ToSlash(filepath.Join(seedModCache, "cache", "download")),
		"GOSUMDB=off",
		"GOTOOLCHAIN=local",
	)
	if out, err := tidy.CombinedOutput(); err != nil {
		_ = proxySrv.Close()
		t.Fatalf("tidy: %v\n%s", err, out)
	}
	_ = proxySrv.Close()

	// Run CLI `self bootstrap --profile self-host ... --json`!
	t.Setenv("GOMODCACHE", seedModCache)
	// Point GOPROXY to file:// cache/download during online bootstrap's `go mod download -json all`?
	// Wait: sib is published in artRoot1, not in seedModCache/cache/download! Wait: tidy ALREADY downloaded sib into seedModCache/cache/download from proxySrv!!
	// So `seedModCache/cache/download` has BOTH sib (cached by `go mod tidy`) and ext! And `--publish-module sibDir` marks sib as published so CaptureModuleClosure keeps sib as Loom-published and ingests ext!
	t.Setenv("GOPROXY", "file://"+filepath.ToSlash(filepath.Join(seedModCache, "cache", "download")))
	t.Setenv("GOSUMDB", "off")

	bootCLIOut, err := runSelfCLI(t,
		"bootstrap",
		"--profile", "self-host",
		"--artifact-root", artRoot1,
		"--source", srcCheckout,
		"--publish-module", sibDir,
		"--go-version", hostGoVer,
		"--json",
	)
	if err != nil {
		t.Fatalf("CLI self bootstrap gen 1 failed: %v\n%s", err, bootCLIOut)
	}
	var boot1 selfBootstrapResult
	if err := json.Unmarshal([]byte(bootCLIOut), &boot1); err != nil {
		t.Fatalf("decode boot1: %v\n%s", err, bootCLIOut)
	}
	if boot1.Generation != 1 || boot1.PublishedModules != 1 || boot1.IngestedModules != 1 {
		t.Fatalf("unexpected boot1: %+v", boot1)
	}

	// Delete sibDir, extSrc, srcCheckout, AND seedModCache completely from disk, and poison GOPROXY!
	if err := os.RemoveAll(sibDir); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(extSrc); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(srcCheckout); err != nil {
		t.Fatal(err)
	}
	if err := removeGoModCacheAll(seedModCache); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "empty-gomodcache"))
	t.Setenv("GOPROXY", "https://proxy.golang.org,direct")

	// 2. `self seal --profile default` MUST refuse with ErrIncompleteClosure (missing required Ollama models)!
	if out, err := runSelfCLI(t, "seal", "--artifact-root", artRoot1, "--profile", "default"); !errors.Is(err, artifact.ErrIncompleteClosure) {
		t.Fatalf("self seal --profile default = (%v, %s), want ErrIncompleteClosure", err, out)
	}

	// 3. `self seal --profile self-host --json` MUST succeed!
	sealOut, err := runSelfCLI(t, "seal", "--artifact-root", artRoot1, "--profile", "self-host", "--json")
	if err != nil {
		t.Fatalf("self seal --profile self-host failed: %v\n%s", err, sealOut)
	}
	var sealSt1 artifact.SealStateV1
	if err := json.Unmarshal([]byte(sealOut), &sealSt1); err != nil {
		t.Fatal(err)
	}
	if !sealSt1.Sealed || sealSt1.ActiveGeneration != 1 || sealSt1.BaselineGeneration != 1 {
		t.Fatalf("unexpected sealSt1: %+v", sealSt1)
	}

	// 4. Wipe binCache1 and run `self verify --offline --rebuild --json` (restores toolchain from CAS + runs productgo + builds against loopback CAS proxy)!
	if err := os.RemoveAll(binCache1); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(binCache1, 0o755); err != nil {
		t.Fatal(err)
	}

	verifyOut1, err := runSelfCLI(t, "verify", "--offline", "--rebuild", "--artifact-root", artRoot1, "--json")
	if err != nil {
		t.Fatalf("self verify --offline --rebuild failed: %v\n%s", err, verifyOut1)
	}
	var verRes1 selfVerifyResult
	if err := json.Unmarshal([]byte(verifyOut1), &verRes1); err != nil {
		t.Fatal(err)
	}
	if !verRes1.RebuildTested || !verRes1.Report.Complete {
		t.Fatalf("unexpected verify result: %+v", verRes1)
	}

	// 5. `self export <bundle.tar> --json`
	bundleTar := filepath.Join(t.TempDir(), "portable-selfhost.tar")
	expOut, err := runSelfCLI(t, "export", bundleTar, "--artifact-root", artRoot1, "--json")
	if err != nil {
		t.Fatalf("self export failed: %v\n%s", err, expOut)
	}
	var expRes artifact.ExportResult
	if err := json.Unmarshal([]byte(expOut), &expRes); err != nil {
		t.Fatal(err)
	}
	if expRes.Generation != 1 || expRes.LoomRefCount != 1 {
		t.Fatalf("unexpected export result: %+v", expRes)
	}

	// 6. Clean-host `self import --seal --json` in a completely isolated host root!
	base2 := t.TempDir()
	_, artRoot2, _ := isolateOfflineAgentOSEnv(t, base2)
	t.Setenv("GOPROXY", "https://proxy.golang.org,direct")

	impOut, err := runSelfCLI(t, "import", bundleTar, "--seal", "--artifact-root", artRoot2, "--json")
	if err != nil {
		t.Fatalf("clean-host self import --seal failed: %v\n%s", err, impOut)
	}
	var impRes artifact.ImportResult
	if err := json.Unmarshal([]byte(impOut), &impRes); err != nil {
		t.Fatal(err)
	}
	if !impRes.Sealed || impRes.ImportedGeneration != 1 || impRes.BaselineGeneration != 1 {
		t.Fatalf("unexpected import result: %+v", impRes)
	}
	if gotHelper := binmgr.CachedBinary("helper-cli"); gotHelper == "" {
		t.Fatal("expected helper-cli to be materialized into clean host BASHY_BIN_CACHE by self import")
	}

	// 7. Add a disposable scratch record, run `self prune --artifacts --apply --json` (evicts BOTH scratch AND reconstructible sib.zip!),
	// then prove `self verify --offline --rebuild` STILL succeeds on the clean host by regenerating sib.zip from imported Loom!
	store2, err := artifact.OpenStore(context.Background(), artifact.StoreOptions{Root: artRoot2})
	if err != nil {
		t.Fatal(err)
	}
	scratchBytes := []byte("temporary-build-scratch")
	if _, err := store2.PutArtifact(context.Background(), artifact.CatalogRecordV1{
		SchemaVersion:  artifact.CatalogRecordSchemaV1,
		Digest:         artifact.DigestBytes(scratchBytes),
		Size:           int64(len(scratchBytes)),
		MediaType:      "application/octet-stream",
		LogicalName:    "scratch/temp",
		LogicalVersion: "v1",
		Provenance:     "test",
		Retention:      artifact.RetentionScratch,
	}, bytes.NewReader(scratchBytes)); err != nil {
		t.Fatal(err)
	}
	_ = store2.Close()

	pruneOut, err := runSelfCLI(t, "prune", "--artifacts", "--artifact-root", artRoot2, "--apply", "--json")
	if err != nil {
		t.Fatalf("self prune --artifacts --apply failed: %v\n%s", err, pruneOut)
	}
	var pruneRes prune.Result
	if err := json.Unmarshal([]byte(pruneOut), &pruneRes); err != nil {
		t.Fatalf("decode prune result: %v\n%s", err, pruneOut)
	}
	if !pruneRes.Applied || pruneRes.Counts.Removed != 2 {
		t.Fatalf("expected 2 items pruned (scratch + reconstructible sib.zip), got %+v", pruneRes)
	}

	verifyOut2, err := runSelfCLI(t, "verify", "--offline", "--rebuild", "--artifact-root", artRoot2, "--json")
	if err != nil {
		t.Fatalf("post-prune clean-host self verify --offline --rebuild failed: %v\n%s", err, verifyOut2)
	}
}
