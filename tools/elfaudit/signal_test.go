package main

import (
	"os"
	"os/exec"
	"path/filepath"
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
