package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
