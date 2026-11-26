// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	ygit "github.com/qiangli/yoke/git"
	"github.com/qiangli/yoke/pkg/artifact"
	"github.com/qiangli/yoke/pkg/gomod"
)

func TestSelfWorkflowHelpersCaptureAndRestore(t *testing.T) {
	base := t.TempDir()
	_, artRoot, _ := isolateOfflineAgentOSEnv(t, base)

	helperSrc := filepath.Join(base, "workflow-src")
	if err := os.MkdirAll(filepath.Join(helperSrc, "actions", "check"), 0o755); err != nil {
		t.Fatal(err)
	}
	scriptBody := []byte("#!/bin/sh\nset -eu\necho staged-runner-helper\n")
	metaBody := []byte("name: local-check\n")
	if err := os.WriteFile(filepath.Join(helperSrc, "actions", "check", "entrypoint.sh"), scriptBody, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperSrc, "actions", "check", "action.yml"), metaBody, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(helperSrc, "commits.txt"), []byte("bashy_commit=1122334455667788990011223344556677889900\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	capOut, err := runSelfCLI(t, "workflow-helpers", "capture", helperSrc, "--name", "local-ci-actions", "--artifact-root", artRoot, "--json")
	if err != nil {
		t.Fatalf("self workflow-helpers capture: %v\n%s", err, capOut)
	}
	var capRes selfWorkflowHelpersCaptureResult
	if err := json.Unmarshal([]byte(capOut), &capRes); err != nil {
		t.Fatalf("decode capture json: %v\n%s", err, capOut)
	}
	if capRes.Capture == nil || capRes.Capture.Root.Kind != artifact.WorkflowHelperKind {
		t.Fatalf("unexpected capture payload: %+v", capRes)
	}

	// Wipe source directory completely and restore under BASHY_OFFLINE=1 (also exercising runner-stage alias).
	if err := os.RemoveAll(helperSrc); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BASHY_OFFLINE", "1")

	restoreDest := filepath.Join(base, "restored-helpers")
	resOut, err := runSelfCLI(t, "runner-stage", "restore", "local-ci-actions", "--dest", restoreDest, "--artifact-root", artRoot, "--json")
	if err != nil {
		t.Fatalf("self runner-stage restore: %v\n%s", err, resOut)
	}
	var matRes artifact.WorkflowHelperMaterializeResult
	if err := json.Unmarshal([]byte(resOut), &matRes); err != nil {
		t.Fatalf("decode restore json: %v\n%s", err, resOut)
	}
	if !matRes.VerifiedArchive || matRes.FileCount != 3 {
		t.Fatalf("unexpected restore result: %+v", matRes)
	}

	restoredScript := filepath.Join(restoreDest, "actions", "check", "entrypoint.sh")
	gotBytes, err := os.ReadFile(restoredScript)
	if err != nil || !bytes.Equal(gotBytes, scriptBody) {
		t.Fatalf("restored entrypoint mismatch: err=%v got=%q", err, string(gotBytes))
	}
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(restoredScript)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm()&0o111 == 0 {
			t.Fatalf("expected executable bit on %s, got %o", restoredScript, fi.Mode().Perm())
		}
	}
}

const agentosMiniFixedPointDriver = `package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"example.com/bashy/gatesib"
)

func main() {
	if len(os.Args) >= 2 && os.Args[1] == "--version" {
		fmt.Printf("bashy-gate-mini %s\n", gatesib.Value())
		return
	}
	if len(os.Args) >= 2 && os.Args[1] == "go" {
		goroot := os.Getenv("BASHY_TEST_FIXEDPOINT_GOROOT")
		goBin := filepath.Join(goroot, "bin", "go")
		if os.PathSeparator == '\\' {
			goBin += ".exe"
		}
		c := exec.Command(goBin, os.Args[2:]...)
		c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
		c.Env = append(os.Environ(), "GOROOT="+goroot)
		if err := c.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				os.Exit(ee.ExitCode())
			}
			os.Exit(1)
		}
		return
	}
	os.Exit(2)
}
`

func TestSelfFixedPointCommandJSON(t *testing.T) {
	hostGo, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go toolchain not on PATH")
	}
	gorootOut, err := exec.Command(hostGo, "env", "GOROOT").Output()
	if err != nil {
		t.Fatalf("go env GOROOT: %v", err)
	}
	t.Setenv("BASHY_TEST_FIXEDPOINT_GOROOT", strings.TrimSpace(string(gorootOut)))

	base := t.TempDir()
	_, artRoot, _ := isolateOfflineAgentOSEnv(t, base)
	loomDir := filepath.Join(base, "loom", "modules.git")

	// Publish a sibling module into Loom + CAS.
	sibDir := filepath.Join(base, "gatesib")
	if _, err := ygit.Init(ygit.InitOptions{Path: sibDir, Quiet: true}); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(sibDir, "go.mod"), []byte("module example.com/bashy/gatesib\n\ngo 1.22\n"), 0o644)
	_ = os.WriteFile(filepath.Join(sibDir, "gatesib.go"), []byte("package gatesib\n\nfunc Value() string { return \"fixedpoint-cli-ok\" }\n"), 0o644)
	_, _ = ygit.Add(ygit.AddOptions{RepoPath: sibDir, All: true})
	_, _ = ygit.Commit(ygit.CommitOptions{RepoPath: sibDir, Message: "v1.0.0", AuthorName: "tester", AuthorEmail: "tester@local.invalid"})

	sibPub, err := gomod.PublishArtifact(context.Background(), gomod.PublishOptions{
		Dir:          sibDir,
		Version:      "v1.0.0",
		ArtifactRoot: artRoot,
		LoomRepoDir:  loomDir,
	})
	if err != nil {
		t.Fatalf("PublishArtifact: %v", err)
	}

	srcDir := filepath.Join(base, "mini-bashy")
	if err := os.MkdirAll(filepath.Join(srcDir, "cmd", "bashy"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte("module example.com/bashy/minicli\n\ngo 1.22\n\nrequire example.com/bashy/gatesib v1.0.0\n"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "go.sum"), []byte("example.com/bashy/gatesib v1.0.0 "+sibPub.Hash+"\nexample.com/bashy/gatesib v1.0.0/go.mod "+sibPub.GoModSum+"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(srcDir, "cmd", "bashy", "main.go"), []byte(agentosMiniFixedPointDriver), 0o644)

	lockPath := filepath.Join(base, "image.lock")
	lockBody := strings.Join([]string{
		"bashy_version=v1.0.0-rc.42",
		"apt_snapshot=20260821T190000Z",
		"go_version=1.27.4",
		"go_sha256_amd64=" + strings.Repeat("a", 64),
		"go_sha256_arm64=" + strings.Repeat("b", 64),
		"base_image=localhost/bashy-base:v1.0.0-rc.42",
		"base_digest=sha256:" + strings.Repeat("c", 64),
		"image=localhost/bashy-build:v1.0.0-rc.42-go1.27.4-1234567890ab",
		"image_digest=sha256:" + strings.Repeat("d", 64),
		"source_bashy_commit=abcdef1234567890abcdef1234567890abcdef12",
		"",
	}, "\n")
	if err := os.WriteFile(lockPath, []byte(lockBody), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build seed binary for --prev-bashy.
	prevBin := filepath.Join(base, "prev", releaseBinaryName())
	if err := os.MkdirAll(filepath.Dir(prevBin), 0o755); err != nil {
		t.Fatal(err)
	}
	{
		srv, err := gomod.StartProxyServer(context.Background(), gomod.ProxyServerOptions{
			Addr:         "127.0.0.1:0",
			ArtifactRoot: artRoot,
			LoomRepoDir:  loomDir,
			Sealed:       true,
		})
		if err != nil {
			t.Fatal(err)
		}
		seedModCache := filepath.Join(base, "seed-modcache")
		c := exec.Command(hostGo, "build", "-trimpath", "-o", prevBin, "./cmd/bashy")
		c.Dir = srcDir
		c.Env = append(os.Environ(),
			"GOPROXY="+srv.URL(),
			"GOSUMDB=off",
			"GOTOOLCHAIN=local",
			"CGO_ENABLED=0",
			"GOWORK=off",
			"GOCACHE="+filepath.Join(base, "seed-gocache"),
			"GOMODCACHE="+seedModCache,
		)
		out, buildErr := c.CombinedOutput()
		_ = srv.Close()
		_ = removeGoModCacheAll(seedModCache)
		if buildErr != nil {
			t.Fatalf("build seed binary: %v\n%s", buildErr, string(out))
		}
	}

	outBin := filepath.Join(base, "verified", releaseBinaryName())
	fpOut, err := runSelfCLI(t,
		"fixed-point",
		"--prev-bashy", prevBin,
		"--source", srcDir,
		"--pkg", "./cmd/bashy",
		"--lock", lockPath,
		"--artifact-root", artRoot,
		"--loom-dir", loomDir,
		"--output-binary", outBin,
		"--json",
	)
	if err != nil {
		t.Fatalf("self fixed-point --json: %v\n%s", err, fpOut)
	}

	var report artifact.FixedPointReportV1
	if err := json.Unmarshal([]byte(fpOut), &report); err != nil {
		t.Fatalf("decode fixed-point report: %v\n%s", err, fpOut)
	}
	if report.SchemaVersion != artifact.FixedPointSchemaV1 || !report.FixedPointMatch {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Candidate1SHA256 != report.Candidate2SHA256 {
		t.Fatalf("candidate_1 (%s) != candidate_2 (%s)", report.Candidate1SHA256, report.Candidate2SHA256)
	}
	if !strings.Contains(report.SmokeOutput, "fixedpoint-cli-ok") {
		t.Fatalf("unexpected SmokeOutput: %q", report.SmokeOutput)
	}
	if _, err := os.Stat(outBin); err != nil {
		t.Fatalf("output binary missing: %v", err)
	}
}

func TestBashyBuildOfflineShellScriptsAndMockOCIPlumbing(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell script execution tested on unix")
	}
	shBin, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not on PATH")
	}

	repoRoot, ok := findBashySourceRoot("../..")
	if !ok {
		t.Fatal("cannot locate bashy repo root from internal/agentos")
	}
	buildSh := filepath.Join(repoRoot, "tools", "bashy-build", "build.sh")
	runSh := filepath.Join(repoRoot, "tools", "bashy-build", "run.sh")
	fpSh := filepath.Join(repoRoot, "tools", "bashy-build", "fixedpoint-offline.sh")
	realLock := filepath.Join(repoRoot, "tools", "bashy-build", "image.lock")

	// 1. POSIX shell syntax check (sh -n) across all three scripts!
	for _, script := range []string{buildSh, runSh, fpSh} {
		out, err := exec.Command(shBin, "-n", script).CombinedOutput()
		if err != nil {
			t.Fatalf("sh -n %s failed: %v\n%s", script, err, string(out))
		}
	}

	lock, err := artifact.ParseBuildImageLock(realLock)
	if err != nil {
		t.Fatalf("ParseBuildImageLock: %v", err)
	}

	tmp := t.TempDir()
	_, _, _ = isolateOfflineAgentOSEnv(t, tmp)
	ociLog := filepath.Join(tmp, "mock-oci.log")
	mockOCI := filepath.Join(tmp, "mock-oci.sh")
	mockOCIBody := "#!/bin/sh\n" +
		"echo \"$*\" >> '" + ociLog + "'\n" +
		"if [ \"$1\" = \"image\" ] && [ \"$2\" = \"inspect\" ]; then\n" +
		"  echo '" + lock.ImageDigest + "'\n" +
		"  exit 0\n" +
		"fi\n" +
		"exit 0\n"
	if err := os.WriteFile(mockOCI, []byte(mockOCIBody), 0o755); err != nil {
		t.Fatal(err)
	}

	srcMount := filepath.Join(tmp, "src-mount")
	artMount := filepath.Join(tmp, "art-mount")
	for _, d := range []string{srcMount, artMount} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	_ = os.WriteFile(filepath.Join(srcMount, "go.mod"), []byte("module example.com/test\n\ngo 1.22\n"), 0o644)

	// 2. Test BASHY_BUILD_OFFLINE=1 + BASHY_ARTIFACTS_MOUNT plumbing in run.sh!
	runCmd := exec.Command(runSh, srcMount, "--", "echo", "offline-check")
	runCmd.Env = append(os.Environ(),
		"BASHY_OCI="+mockOCI,
		"BASHY_BUILD_LOCK="+realLock,
		"BASHY_BUILD_OFFLINE=1",
		"BASHY_ARTIFACTS_MOUNT="+artMount,
	)
	if out, err := runCmd.CombinedOutput(); err != nil {
		t.Fatalf("run.sh offline failed: %v\n%s", err, string(out))
	}
	loggedBytes, err := os.ReadFile(ociLog)
	if err != nil {
		t.Fatal(err)
	}
	logged := string(loggedBytes)
	for _, wantToken := range []string{
		"--network=none",
		"-e BASHY_OFFLINE=1",
		"-e GOTOOLCHAIN=local",
		":/var/lib/bashy/artifacts:ro",
	} {
		if !strings.Contains(logged, wantToken) {
			t.Fatalf("run.sh offline missing %q in OCI calls:\n%s", wantToken, logged)
		}
	}

	// 3. Test BASHY_OFFLINE=1 in build.sh seed -> passes --pull=never --network=none to $oci build!
	_ = os.Remove(ociLog)
	seedBin := filepath.Join(tmp, "seed-bashy")
	if err := os.WriteFile(seedBin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	seedBuildCmd := exec.Command(buildSh, "seed", "v1.0.0-offline-test", seedBin)
	seedBuildCmd.Env = append(os.Environ(),
		"BASHY_OCI="+mockOCI,
		"BASHY_BUILD_LOCK="+realLock,
		"BASHY_OFFLINE=1",
	)
	if out, err := seedBuildCmd.CombinedOutput(); err != nil {
		t.Fatalf("build.sh seed offline failed: %v\n%s", err, string(out))
	}
	buildLoggedBytes, err := os.ReadFile(ociLog)
	if err != nil {
		t.Fatal(err)
	}
	buildLogged := string(buildLoggedBytes)
	if !strings.Contains(buildLogged, "build --pull=never --network=none -t localhost/bashy-base:v1.0.0-offline-test") {
		t.Fatalf("build.sh offline missing --pull=never --network=none:\n%s", buildLogged)
	}

	// 4. Test build.sh roll with corrupt BASHY_GO_TARBALL fails closed BEFORE calling curl or run.sh!
	sourcesDir := filepath.Join(tmp, "roll-sources")
	for _, repo := range []string{"bashy", "coreutils", "sh", "readline"} {
		rdir := filepath.Join(sourcesDir, repo)
		if err := os.MkdirAll(filepath.Join(rdir, "bin"), 0o755); err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(rdir, "go.mod"), []byte("module example.com/"+repo+"\n\ngo 1.22\n"), 0o644)
	}
	_ = os.WriteFile(filepath.Join(sourcesDir, "bashy", "bin", "bashy"), []byte("built-binary"), 0o755)

	badTarball := filepath.Join(tmp, "bad-go.tar.gz")
	_ = os.WriteFile(badTarball, []byte("corrupted-tarball"), 0o644)
	badRollCmd := exec.Command(buildSh, "roll", "v1.0.0-offline-roll", sourcesDir)
	badRollCmd.Env = append(os.Environ(),
		"BASHY_OCI="+mockOCI,
		"BASHY_BUILD_LOCK="+realLock,
		"BASHY_OFFLINE=1",
		"BASHY_GO_TARBALL="+badTarball,
	)
	if out, err := badRollCmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "BASHY_GO_TARBALL sha256") {
		t.Fatalf("expected BASHY_GO_TARBALL sha256 failure, got err=%v out=%s", err, string(out))
	}

	// Verify valid BASHY_GO_TARBALL against custom lock passes SHA-256 verification without calling curl!
	goodTarball := filepath.Join(tmp, "good-go.tar.gz")
	goodPayload := []byte("verified-local-go-archive")
	_ = os.WriteFile(goodTarball, goodPayload, 0o644)
	sum := sha256.Sum256(goodPayload)
	sumHex := hex.EncodeToString(sum[:])
	customLock := filepath.Join(tmp, "custom-image.lock")
	customLockBody := strings.Join([]string{
		"bashy_version=v1.0.0-rc.40",
		"apt_snapshot=20260821T190000Z",
		"go_version=1.27.4",
		"go_sha256_amd64=" + sumHex,
		"go_sha256_arm64=" + sumHex,
		"base_image=localhost/bashy-base:v1.0.0-rc.40",
		"base_digest=" + lock.BaseDigest,
		"image=" + lock.Image,
		"image_digest=" + lock.ImageDigest,
		"",
	}, "\n")
	_ = os.WriteFile(customLock, []byte(customLockBody), 0o644)

	// Shadow curl with an unconditionally failing stub to prove curl is never called.
	noCurlDir := filepath.Join(tmp, "no-curl")
	_ = os.MkdirAll(noCurlDir, 0o755)
	_ = os.WriteFile(filepath.Join(noCurlDir, "curl"), []byte("#!/bin/sh\necho 'curl forbidden in offline mode' >&2\nexit 99\n"), 0o755)

	goodRollCmd := exec.Command(buildSh, "roll", "v1.0.0-offline-roll", sourcesDir)
	goodRollCmd.Env = append(os.Environ(),
		"PATH="+noCurlDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"BASHY_OCI="+mockOCI,
		"BASHY_BUILD_LOCK="+customLock,
		"BASHY_OFFLINE=1",
		"BASHY_GO_TARBALL="+goodTarball,
	)
	if out, err := goodRollCmd.CombinedOutput(); err != nil {
		t.Fatalf("build.sh roll with valid BASHY_GO_TARBALL failed: %v\n%s", err, string(out))
	}

	// 5. Test fixedpoint-offline.sh --container plumbing through run.sh + mock BASHY_OCI!
	_ = os.Remove(ociLog)
	fpContainerCmd := exec.Command(fpSh, "--container", "--lock", customLock, "--source", srcMount, "--artifact-root", artMount)
	fpContainerCmd.Env = append(os.Environ(), "BASHY_OCI="+mockOCI)
	if out, err := fpContainerCmd.CombinedOutput(); err != nil {
		t.Fatalf("fixedpoint-offline.sh --container failed: %v\n%s", err, string(out))
	}
	fpLoggedBytes, err := os.ReadFile(ociLog)
	if err != nil {
		t.Fatal(err)
	}
	fpLogged := string(fpLoggedBytes)
	if !strings.Contains(fpLogged, "--network=none") || !strings.Contains(fpLogged, "bashy self fixed-point --json") {
		t.Fatalf("fixedpoint-offline.sh --container missing expected OCI args:\n%s", fpLogged)
	}
}
