// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"encoding/json"
	"github.com/qiangli/yoke/pkg/binmgr"
	"io"
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

func TestInstallProductStagesBeforeChangingAnyMember(t *testing.T) {
	source, target := t.TempDir(), t.TempDir()
	members := map[string]string{}
	for _, name := range productMemberNames() {
		members[name] = filepath.Join(source, name)
		if err := os.WriteFile(members[name], []byte("new-"+name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, name), []byte("old-"+name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// Last source is absent: no earlier installed member may have changed.
	if err := os.Remove(members[binmgr.BinaryName("outpost")]); err != nil {
		t.Fatal(err)
	}
	if err := installProductFiles(members, filepath.Join(target, releaseBinaryName())); err == nil {
		t.Fatal("expected missing companion error")
	}
	for _, name := range productMemberNames() {
		data, err := os.ReadFile(filepath.Join(target, name))
		if err != nil || string(data) != "old-"+name {
			t.Fatalf("%s changed before staging completed: %q %v", name, data, err)
		}
	}
}
func TestAdjacentProductAndVersionPair(t *testing.T) {
	dir := t.TempDir()
	for _, name := range productMemberNames() {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if members, err := adjacentProduct(filepath.Join(dir, releaseBinaryName())); err != nil || len(members) != 4 {
		t.Fatalf("members=%v err=%v", members, err)
	}
	if !matchingProductBanner("bashy, version 5.3.0(1)-bashy-v1.2.3-dev", "v1.2.3") {
		t.Fatal("matching product rejected")
	}
	if matchingProductBanner("bashy, version 5.3.0(1)-bashy-v1.2.4", "v1.2.3") {
		t.Fatal("mismatched product accepted")
	}
	os.Remove(filepath.Join(dir, binmgr.BinaryName("sh")))
	if _, err := adjacentProduct(filepath.Join(dir, releaseBinaryName())); err == nil {
		t.Fatal("partial archive accepted")
	}
}

func TestAdjacentProductLauncherPayloadRules(t *testing.T) {
	dir := t.TempDir()
	for _, name := range requiredProductMemberNames() {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	bash := binmgr.BinaryName("bash")
	sh := binmgr.BinaryName("sh")
	if err := os.WriteFile(filepath.Join(dir, bash+".real"), []byte("bash payload"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, sh+".real"), []byte("sh payload"), 0755); err != nil {
		t.Fatal(err)
	}
	members, err := adjacentProduct(filepath.Join(dir, releaseBinaryName()))
	if err != nil {
		t.Fatal(err)
	}
	if members[bash+".real"] == "" || members[sh+".real"] == "" {
		t.Fatalf("optional payloads missing from archive install members: %v", members)
	}
	names := productMemberNames(members)
	for _, name := range []string{bash, sh} {
		launcher, payload := -1, -1
		for i, got := range names {
			if got == name {
				launcher = i
			}
			if got == name+".real" {
				payload = i
			}
		}
		if payload < 0 || launcher < 0 || payload >= launcher {
			t.Fatalf("payload must be installed before launcher %s: %v", name, names)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, binmgr.BinaryName("bashy")+".real"), []byte("forbidden"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := adjacentProduct(filepath.Join(dir, releaseBinaryName())); err == nil || !strings.Contains(err.Error(), "one-file") {
		t.Fatalf("bashy.real refusal = %v", err)
	}
}

func TestSelfSeedExportAndInstallSeed(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("BASHY_BIN_CACHE", cache)
	toolDir := filepath.Join(cache, "fixture-tool", "v1")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatal(err)
	}
	binPath := filepath.Join(toolDir, binmgr.BinaryName("fixture-tool"))
	if err := os.WriteFile(binPath, []byte("echo fixture"), 0755); err != nil {
		t.Fatal(err)
	}
	seedTar := filepath.Join(t.TempDir(), "seed.tar")
	cmd := selfSeedCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"export", seedTar})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("self seed export failed: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(seedTar); err != nil {
		t.Fatalf("seed tar not created: %v", err)
	}

	cache2 := t.TempDir()
	t.Setenv("BASHY_BIN_CACHE", cache2)
	if err := binmgr.ImportSeed(cmd.Context(), seedTar); err != nil {
		t.Fatalf("import seed failed: %v", err)
	}
	if imported := binmgr.CachedBinary("fixture-tool"); imported == "" {
		t.Fatal("imported tool not found in cache")
	}
}

func TestSelfInstallServiceRefusesRootBeforeWrites(t *testing.T) {
	orig := selfInstallEUID
	selfInstallEUID = func() int { return 0 }
	t.Cleanup(func() { selfInstallEUID = orig })
	dir := filepath.Join(t.TempDir(), "bin")
	for _, flags := range [][]string{{"--service"}, {"--service", "--system"}} {
		cmd := selfInstallCmd()
		cmd.SetArgs(append([]string{"--dir", dir, "--seed", filepath.Join(t.TempDir(), "missing.seed")}, flags...))
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "refusing --service as root") {
			t.Fatalf("%v: err = %v, want root refusal", flags, err)
		}
		if _, statErr := os.Stat(dir); !os.IsNotExist(statErr) {
			t.Fatalf("%v: install dir created before refusal: %v", flags, statErr)
		}
	}
}
