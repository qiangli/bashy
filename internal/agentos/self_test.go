// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestSelfBuildDefaultTarget(t *testing.T) {
	got := selfBuildDefaultTarget()
	if runtime.GOOS == "windows" {
		if got != filepath.Join("bin", "bashy.exe") {
			t.Fatalf("selfBuildDefaultTarget on windows = %q", got)
		}
		return
	}
	if got != filepath.Join("bin", "bashy") {
		t.Fatalf("selfBuildDefaultTarget = %q", got)
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

func TestSelfCheckCommandPlainAndJSON(t *testing.T) {
	cmd := selfCmd()
	var plain bytes.Buffer
	cmd.SetOut(&plain)
	cmd.SetErr(&plain)
	cmd.SetArgs([]string{"check"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("self check: %v\n%s", err, plain.String())
	}
	if !strings.Contains(plain.String(), "embedded git") || !strings.Contains(plain.String(), "bashy self check") {
		t.Fatalf("self check output missing bootstrap checks:\n%s", plain.String())
	}

	cmd = selfCmd()
	var js bytes.Buffer
	cmd.SetOut(&js)
	cmd.SetErr(&js)
	cmd.SetArgs([]string{"check", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("self check --json: %v\n%s", err, js.String())
	}
	var payload struct {
		SchemaVersion string        `json:"schema_version"`
		Checks        []doctorCheck `json:"checks"`
	}
	if err := json.Unmarshal(js.Bytes(), &payload); err != nil {
		t.Fatalf("invalid self check json: %v\n%s", err, js.String())
	}
	if payload.SchemaVersion != "bashy-self-check-v1" {
		t.Fatalf("schema = %q", payload.SchemaVersion)
	}
	var sawManagedGo bool
	for _, c := range payload.Checks {
		if c.Name == "managed go" && c.Status == "ok" {
			sawManagedGo = true
		}
	}
	if !sawManagedGo {
		t.Fatalf("self check JSON missing managed go check: %#v", payload.Checks)
	}
}

func TestSelfCommandIncludesBuildAndSourceInstall(t *testing.T) {
	cmd := selfCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"build", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("self build --help: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "current source checkout") {
		t.Fatalf("self build help missing source wording:\n%s", out.String())
	}

	cmd = selfCmd()
	out.Reset()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("self install --help: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "--source") {
		t.Fatalf("self install help missing --source:\n%s", out.String())
	}
}
