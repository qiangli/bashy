package bashybuild_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func parseKeyValues(t *testing.T, text string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("malformed lock line: %q", line)
		}
		out[k] = v
	}
	return out
}

func TestImageLockAndContainerfileConsistency(t *testing.T) {
	lockBytes, err := os.ReadFile("image.lock")
	if err != nil {
		t.Fatalf("read image.lock: %v", err)
	}
	kv := parseKeyValues(t, string(lockBytes))

	if got := kv["go_version"]; got != "1.27.4" {
		t.Fatalf("image.lock go_version = %q, want 1.27.4", got)
	}
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, k := range []string{"go_sha256_amd64", "go_sha256_arm64", "bashy_sha256"} {
		if !hex64.MatchString(kv[k]) {
			t.Fatalf("image.lock %s = %q, want 64-char lowercase hex", k, kv[k])
		}
	}
	// Must not retain the retired go1.27.2 archive digests.
	if kv["go_sha256_amd64"] == "ecbadb99091a3f46e31f5f934b068b1864eafa7995211b39eaddf76996045fe5" {
		t.Fatal("image.lock go_sha256_amd64 still holds the go1.27.2 checksum")
	}
	if kv["go_sha256_arm64"] == "94f3e30b8e374bc285e7dadc11e0865726b9bc6e85b841ccceaabc0214c6b7c8" {
		t.Fatal("image.lock go_sha256_arm64 still holds the go1.27.2 checksum")
	}
	for _, k := range []string{"bashy_version", "previous_version", "built_with_digest", "base_image", "base_digest", "image", "image_digest"} {
		if kv[k] == "" {
			t.Fatalf("image.lock %s is empty", k)
		}
	}
	if !strings.HasPrefix(kv["image_digest"], "sha256:") || len(kv["image_digest"]) != len("sha256:")+64 {
		t.Fatalf("image.lock image_digest = %q, want sha256:<64hex>", kv["image_digest"])
	}
	shortDigest := strings.TrimPrefix(kv["image_digest"], "sha256:")[:12]
	if !strings.HasSuffix(kv["image"], "-"+shortDigest) {
		t.Fatalf("image.lock image %q does not end with digest suffix -%s", kv["image"], shortDigest)
	}

	cfBytes, err := os.ReadFile("Containerfile")
	if err != nil {
		t.Fatalf("read Containerfile: %v", err)
	}
	cf := string(cfBytes)
	for _, want := range []string{
		"ARG GO_VERSION=" + kv["go_version"],
		"ARG GO_SHA256_AMD64=" + kv["go_sha256_amd64"],
		"ARG GO_SHA256_ARM64=" + kv["go_sha256_arm64"],
		"BASHY_GO_VERSION=${GO_VERSION}",
		"BASHY_ADAPTER_CACHE=/var/cache/bashy-build/adapters",
		".binmgr-tree-complete",
	} {
		if !strings.Contains(cf, want) {
			t.Fatalf("Containerfile missing %q", want)
		}
	}
}

func TestRunShVerifiesDigestAndCreatesAdapterCache(t *testing.T) {
	tmp := t.TempDir()
	srcDir := filepath.Join(tmp, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	lockFile := filepath.Join(tmp, "image.lock")
	wantDigest := "sha256:" + strings.Repeat("a", 64)
	lockContent := fmt.Sprintf("image=localhost/bashy-build:v1.0.0-test-go1.27.4-aaaaaaaaaaaa\nimage_digest=%s\n", wantDigest)
	if err := os.WriteFile(lockFile, []byte(lockContent), 0o644); err != nil {
		t.Fatal(err)
	}

	ociLog := filepath.Join(tmp, "oci.log")
	stubOCI := filepath.Join(tmp, "fake-oci.sh")
	stubScript := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$FAKE_OCI_LOG"
case "$1 ${2-}" in
  "image inspect")
    printf '%s\n' "$FAKE_OCI_DIGEST"
    ;;
  "volume exists")
    exit 0
    ;;
  "run --rm")
    exit 0
    ;;
esac
`
	if err := os.WriteFile(stubOCI, []byte(stubScript), 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Mismatched digest must fail closed before running the container.
	badCmd := exec.Command("/bin/sh", "run.sh", srcDir, "--", "echo hi")
	badCmd.Env = append(os.Environ(),
		"BASHY_BUILD_LOCK="+lockFile,
		"BASHY_OCI="+stubOCI,
		"FAKE_OCI_LOG="+ociLog,
		"FAKE_OCI_DIGEST=sha256:"+strings.Repeat("b", 64),
	)
	if out, err := badCmd.CombinedOutput(); err == nil {
		t.Fatalf("expected digest mismatch failure, got success: %s", out)
	} else if !strings.Contains(string(out), "lock says") {
		t.Fatalf("unexpected mismatch error: %s", out)
	}

	// 2. Matching digest runs with --init, BASHY_BUILD_RUN=1, and adapter cache dir creation.
	_ = os.Remove(ociLog)
	okCmd := exec.Command("/bin/sh", "run.sh", srcDir, "--", "echo ok")
	okCmd.Env = append(os.Environ(),
		"BASHY_BUILD_LOCK="+lockFile,
		"BASHY_OCI="+stubOCI,
		"FAKE_OCI_LOG="+ociLog,
		"FAKE_OCI_DIGEST="+wantDigest,
	)
	if out, err := okCmd.CombinedOutput(); err != nil {
		t.Fatalf("run.sh failed: %v\n%s", err, out)
	}
	logged, err := os.ReadFile(ociLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--init", "BASHY_BUILD_RUN=1", "BASHY_ADAPTER_CACHE", "echo ok"} {
		if !strings.Contains(string(logged), want) {
			t.Fatalf("oci log missing %q:\n%s", want, logged)
		}
	}
}

func makeFakeGoTarball(t *testing.T, version string) ([]byte, string) {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	script := []byte("#!/bin/sh\necho \"go version go" + version + " linux/amd64\"\n")
	hdr := &tar.Header{
		Name: "go/bin/go",
		Mode: 0o755,
		Size: int64(len(script)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(script); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	raw := buf.Bytes()
	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:])
}

func TestBuildShRollStagesGoToolchainAcrossVersionBump(t *testing.T) {
	tmp := t.TempDir()
	dlDir := filepath.Join(tmp, "dl")
	if err := os.MkdirAll(dlDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tgz, sumHex := makeFakeGoTarball(t, "1.27.4")
	for _, arch := range []string{"amd64", "arm64"} {
		if err := os.WriteFile(filepath.Join(dlDir, "go1.27.4.linux-"+arch+".tar.gz"), tgz, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	sources := filepath.Join(tmp, "sources")
	for _, repo := range []string{"bashy", "coreutils", "sh", "readline"} {
		dir := filepath.Join(sources, repo)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/"+repo+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	commitsTxt := "bashy_commit=1111111111111111111111111111111111111111\ncoreutils_commit=2222222222222222222222222222222222222222\n"
	if err := os.WriteFile(filepath.Join(sources, "commits.txt"), []byte(commitsTxt), 0o644); err != nil {
		t.Fatal(err)
	}

	prevDigest := "sha256:" + strings.Repeat("1", 64)
	baseDigest := "sha256:" + strings.Repeat("2", 64)
	newDigest := "sha256:333333333333" + strings.Repeat("4", 52)

	writeLock := func(path, amdSum, armSum string) {
		lock := fmt.Sprintf(
			"bashy_version=v1.0.0-rc.37\nprevious_version=v1.0.0-rc.37\napt_snapshot=20260821T190000Z\ngo_version=1.27.4\ngo_sha256_amd64=%s\ngo_sha256_arm64=%s\nimage=localhost/bashy-build:v1.0.0-rc.37-go1.27.2-111111111111\nimage_digest=%s\n",
			amdSum, armSum, prevDigest,
		)
		if err := os.WriteFile(path, []byte(lock), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	binCache := filepath.Join(tmp, "bin-cache")
	stubBashy := filepath.Join(tmp, "fake-container-bashy.sh")
	stubBashyScript := `#!/bin/sh
set -eu
if [ "${1-}" = "-c" ]; then
  shift
  cmd=$1
  exec /bin/sh -c "$cmd"
fi
if [ "${1-}" = "dag" ] && [ "${2-}" = "build" ]; then
  # Verify make is shadowed, GOWORK=off, BASHY_GO_VERSION=1.27.4, and staged go1.27.4 is on PATH.
  if make >/dev/null 2>&1; then
    echo "make was not shadowed" >&2
    exit 1
  fi
  [ "${GOWORK-}" = "off" ] || { echo "GOWORK=${GOWORK-}, want off" >&2; exit 1; }
  [ "${BASHY_GO_VERSION-}" = "1.27.4" ] || { echo "BASHY_GO_VERSION=${BASHY_GO_VERSION-}, want 1.27.4" >&2; exit 1; }
  ver=$(go version)
  case "$ver" in
    *"go1.27.4"*) ;;
    *) echo "unexpected go on PATH: $ver" >&2; exit 1 ;;
  esac
  [ -f "$BASHY_BIN_CACHE/go/1.27.4/.binmgr-tree-complete" ] || { echo "missing .binmgr-tree-complete" >&2; exit 1; }
  mkdir -p bin
  printf '#!/bin/sh\necho built-bashy-%s-%s\n' "${VERSION-}" "${BUILD_ID-}" > bin/bashy
  chmod +x bin/bashy
  exit 0
fi
exit 1
`
	if err := os.WriteFile(stubBashy, []byte(stubBashyScript), 0o755); err != nil {
		t.Fatal(err)
	}

	ociLog := filepath.Join(tmp, "build-oci.log")
	stubOCI := filepath.Join(tmp, "fake-build-oci.sh")
	stubOCIScript := fmt.Sprintf(`#!/bin/sh
set -eu
printf 'OCI: %%s\n' "$*" >> %q
case "$1 ${2-}" in
  "image inspect")
    target=${5-}
    case "$target" in
      *rc.37*) printf '%%s\n' %q ;;
      *bashy-base:*) printf '%%s\n' %q ;;
      *) printf '%%s\n' %q ;;
    esac
    ;;
  "volume exists")
    exit 0
    ;;
  "build -t")
    exit 0
    ;;
  "tag "*)
    exit 0
    ;;
  "run --rm")
    # Distinguish smoke check (-c 'bashy --version ...') from run.sh (--init ... /usr/bin/dash).
    case "$*" in
      *--entrypoint*)
        # Extract the final shell command argument and execute it locally with /work -> sources.
        last=""
        for arg in "$@"; do last=$arg; done
        rewritten=$(printf '%%s' "$last" | sed "s|/bashy|%s|g; s|bashy dag|%s dag|g")
        GOMODCACHE=%q GOCACHE=%q GOTMPDIR=%q BASHY_BIN_CACHE=%q sh -c "cd %q && $rewritten"
        ;;
      *)
        exit 0
        ;;
    esac
    ;;
esac
`, ociLog, prevDigest, baseDigest, newDigest, stubBashy, stubBashy,
		filepath.Join(tmp, "gomod"), filepath.Join(tmp, "gobuild"), filepath.Join(tmp, "gotmp"), binCache, sources)
	if err := os.WriteFile(stubOCI, []byte(stubOCIScript), 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Tampered checksum in image.lock must fail closed during R1 toolchain staging.
	badLock := filepath.Join(tmp, "bad.lock")
	writeLock(badLock, strings.Repeat("f", 64), strings.Repeat("f", 64))
	badCmd := exec.Command("/bin/sh", "build.sh", "roll", "v1.0.0-rc.40", sources)
	badCmd.Env = append(os.Environ(),
		"BASHY_BUILD_LOCK="+badLock,
		"BASHY_OCI="+stubOCI,
		"BASHY_GO_DL_BASE=file://"+dlDir,
	)
	if out, err := badCmd.CombinedOutput(); err == nil {
		t.Fatalf("expected checksum mismatch to fail roll, got success:\n%s", out)
	}

	// 2. Valid checksum stages go1.27.4, runs self-build with GOWORK=off, and prints new lock values.
	goodLock := filepath.Join(tmp, "good.lock")
	writeLock(goodLock, sumHex, sumHex)
	goodCmd := exec.Command("/bin/sh", "build.sh", "roll", "v1.0.0-rc.40", sources)
	goodCmd.Env = append(os.Environ(),
		"BASHY_BUILD_LOCK="+goodLock,
		"BASHY_OCI="+stubOCI,
		"BASHY_GO_DL_BASE=file://"+dlDir,
	)
	out, err := goodCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build.sh roll failed: %v\n%s", err, out)
	}
	kv := parseKeyValues(t, string(out))
	if kv["bashy_version"] != "v1.0.0-rc.40" {
		t.Fatalf("bashy_version = %q, want v1.0.0-rc.40", kv["bashy_version"])
	}
	if kv["previous_version"] != "v1.0.0-rc.37" {
		t.Fatalf("previous_version = %q, want v1.0.0-rc.37", kv["previous_version"])
	}
	if kv["image"] != "localhost/bashy-build:v1.0.0-rc.40-go1.27.4-333333333333" {
		t.Fatalf("image = %q, want localhost/bashy-build:v1.0.0-rc.40-go1.27.4-333333333333", kv["image"])
	}
	if kv["source_bashy_commit"] != "1111111111111111111111111111111111111111" {
		t.Fatalf("source_bashy_commit = %q", kv["source_bashy_commit"])
	}
}
