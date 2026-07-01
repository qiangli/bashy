// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBashyArchiveMatch(t *testing.T) {
	tests := []struct {
		name        string
		goos        string
		goarch      string
		want        bool
		description string
	}{
		{"bashy-darwin-arm64.tar.gz", "darwin", "arm64", true, "current goreleaser archive shape"},
		{"bash-darwin-arm64.tar.gz", "darwin", "arm64", false, "lean bash archive is not bashy"},
		{"bashy-linux-amd64.tar.gz", "darwin", "arm64", false, "wrong os"},
		{"bashy-darwin-amd64.tar.gz", "darwin", "arm64", false, "wrong arch"},
		{"checksums.txt", "darwin", "arm64", false, "sidecar"},
	}
	for _, tt := range tests {
		if got := bashyArchiveMatch(tt.name, tt.goos, tt.goarch); got != tt.want {
			t.Fatalf("%s: bashyArchiveMatch(%q, %q, %q) = %v, want %v", tt.description, tt.name, tt.goos, tt.goarch, got, tt.want)
		}
	}
}

func TestReleaseBinaryName(t *testing.T) {
	got := releaseBinaryName()
	if runtime.GOOS == "windows" {
		if got != "bashy.exe" {
			t.Fatalf("releaseBinaryName on windows = %q", got)
		}
		return
	}
	if got != "bashy" {
		t.Fatalf("releaseBinaryName = %q", got)
	}
}

func TestResolveSelfInstallTargetExplicit(t *testing.T) {
	dir := t.TempDir()
	got, err := resolveSelfInstallTarget(filepath.Join(dir, "bashy-new"))
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("target should be absolute: %q", got)
	}
}

func TestInstallExecutable(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	dst := filepath.Join(dir, "nested", "bashy")
	if err := os.WriteFile(src, []byte("binary"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installExecutable(src, dst); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "binary" {
		t.Fatalf("installed body = %q", body)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(dst)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm()&0o111 == 0 {
			t.Fatalf("installed file is not executable: %v", info.Mode())
		}
	}
}
