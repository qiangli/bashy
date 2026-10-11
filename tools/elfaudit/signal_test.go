package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLinuxBashySignalSymbolBuildGate(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name    string
		ldflags string
		wantErr string
	}{
		{name: "retained", ldflags: "-w"},
		{name: "stripped", ldflags: "-s -w", wantErr: "no ELF symbol table"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			artifact := filepath.Join(t.TempDir(), "bashy-linux-amd64")
			cmd := exec.Command("go", "build", "-trimpath", "-tags", "bashy_scratch", "-ldflags", tc.ldflags, "-o", artifact, "./cmd/bashy")
			cmd.Dir = root
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=linux", "GOARCH=amd64")
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build Linux Bashy: %v\n%s", err, output)
			}
			err := auditBashySignal(artifact)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("audit stripped artifact: got %v, want %q", err, tc.wantErr)
			}
		})
	}
}

func TestDarwinBashySignalSnapshotBuildGate(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	arch := runtime.GOARCH
	if arch != "amd64" && arch != "arm64" {
		arch = "arm64"
	}
	for _, tc := range []struct {
		name    string
		product bool
		wantErr string
	}{
		{name: "product", product: true},
		{name: "plain", wantErr: "lacks the product runtime"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			artifact := filepath.Join(t.TempDir(), "bashy-darwin")
			cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-w", "-o", artifact, "./cmd/bashy")
			if tc.product {
				cmd = exec.Command("./scripts/go-product.sh", "build", "-trimpath", "-ldflags=-w", "-o", artifact, "./cmd/bashy")
			}
			cmd.Dir = root
			// Cross-builds too: the Darwin release no longer needs cgo.
			cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS=darwin", "GOARCH="+arch)
			if output, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("build Darwin Bashy: %v\n%s", err, output)
			}
			err := auditBashySignal(artifact)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("audit Darwin artifact: got %v, want %q", err, tc.wantErr)
			}
		})
	}
}
