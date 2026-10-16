package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOtelAssetsPresent(t *testing.T) {
	dir := t.TempDir()
	for _, name := range requiredOtelAssets() {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("blob"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkReleaseAssets(dir); err != nil {
		t.Fatalf("checkReleaseAssets = %v, want nil", err)
	}
}

func TestOtelAssetsMissing(t *testing.T) {
	// An empty asset directory is missing every blob.
	if err := checkReleaseAssets(t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "otel-darwin-arm64.gz") {
		t.Fatalf("empty dir checkReleaseAssets = %v, want missing-blob error", err)
	}
	// One absent platform is still flagged, naming the exact asset binmgr
	// would fail to fetch.
	dir := t.TempDir()
	for _, name := range requiredOtelAssets() {
		if name == "otel-windows-amd64.gz" {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), []byte("blob"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := checkReleaseAssets(dir); err == nil ||
		!strings.Contains(err.Error(), "otel-windows-amd64.gz") {
		t.Fatalf("partial dir checkReleaseAssets = %v, want otel-windows-amd64.gz error", err)
	}
}

func TestOtelAssetsNonexistentDir(t *testing.T) {
	if err := checkReleaseAssets(filepath.Join(t.TempDir(), "no-such-dir")); err == nil {
		t.Fatal("checkReleaseAssets = nil for missing directory, want error")
	}
}

func TestOtelPlatformsMatchPublishScript(t *testing.T) {
	// Guards drift between this check and its producer: the platform set must
	// equal the PLATFORMS matrix in scripts/publish-otel-blob.sh.
	raw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "publish-otel-blob.sh"))
	if err != nil {
		t.Fatal(err)
	}
	var script []string
	for _, line := range strings.Split(string(raw), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), `PLATFORMS="`); ok {
			script = strings.Fields(strings.TrimSuffix(v, `"`))
		}
	}
	if len(script) == 0 {
		t.Fatal("PLATFORMS not found in scripts/publish-otel-blob.sh")
	}
	if len(script) != len(otelPlatforms) {
		t.Fatalf("publish script PLATFORMS = %v, want %v", script, otelPlatforms)
	}
	for i := range script {
		if script[i] != otelPlatforms[i] {
			t.Fatalf("publish script PLATFORMS = %v, want %v", script, otelPlatforms)
		}
	}
}

func TestRejectCoreProbeArtifact(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the diagnostic Bashy artifact")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "bashy")
	cmd := exec.Command("go", "build", "-tags", "bashy_core", "-o", artifact, "./cmd/bashy")
	cmd.Dir = root
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build diagnostic Bashy: %v\n%s", err, out)
	}
	if err := audit(artifact); err == nil || !strings.Contains(err.Error(), "diagnostic bashy_core") {
		t.Fatalf("core artifact audit = %v, want diagnostic rejection", err)
	}
}

func TestRejectBaseWithoutCertificationSettings(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the Bashy base artifact")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	artifact := filepath.Join(t.TempDir(), "bashy")
	cmd := exec.Command("go", "build", "-tags", "bashy_cert_base", "-o", artifact, "./cmd/bashy")
	cmd.Dir = root
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build unguarded Bashy base: %v\n%s", err, out)
	}
	if err := audit(artifact); err == nil || !strings.Contains(err.Error(), "without Linux CGO and osusergo certification") {
		t.Fatalf("base artifact audit = %v, want certification-settings rejection", err)
	}
}
