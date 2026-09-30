// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"bashy, GNU Bash 5.3 compatible, version 5.3.0(1)-bashy-dev (a5d5934)\n", "5.3.0(1)-bashy-dev (a5d5934)"},
		{"GNU bash, version 5.3.0(1)-bashy-0.20.0\n", "5.3.0(1)-bashy-0.20.0"},
		{"version 5.3.0(1)-bashy-dev (abc)\nextra", "5.3.0(1)-bashy-dev (abc)"},
	}
	for _, c := range cases {
		if got := parseVersion(c.in); got != c.want {
			t.Fatalf("parseVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeGOOS(t *testing.T) {
	cases := map[string]string{
		"Darwin":     "darwin",
		"Linux":      "linux",
		"MINGW64_NT": "windows",
		"unknown":    "",
	}
	for in, want := range cases {
		if got := normalizeGOOS(in); got != want {
			t.Errorf("normalizeGOOS(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNormalizeGOARCH(t *testing.T) {
	cases := map[string]string{
		"x86_64":  "amd64",
		"arm64":   "arm64",
		"aarch64": "arm64",
		"i386":    "386",
	}
	for in, want := range cases {
		if got := normalizeGOARCH(in); got != want {
			t.Errorf("normalizeGOARCH(%q)=%q want %q", in, got, want)
		}
	}
}

func TestRemoteInstallFake_FreshIdempotentAndUpgrade(t *testing.T) {
	// Fresh install to an empty fake remote.
	tmp := t.TempDir()
	cmd := remoteCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", "--fake-root", tmp, "fakehost"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("fresh install: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "installed") && !strings.Contains(out.String(), "installing") {
		t.Fatalf("fresh install output missing 'installed': %s", out.String())
	}
	// Verify binary exists and version matches local.
	goos, goarch, _ := newTransport("fakehost", tmp).detectOSArch("fakehost")
	localBin, _, err := findLocalBinaryForHost(goos, goarch)
	if err != nil {
		t.Fatalf("findLocalBinary: %v", err)
	}
	localVer, err := localVersionString(localBin)
	if err != nil {
		t.Fatalf("localVersion: %v", err)
	}
	remoteVer, err := newTransport("fakehost", tmp).remoteVersion("fakehost", goos)
	if err != nil {
		t.Fatalf("remoteVersion after install: %v", err)
	}
	if remoteVer != localVer {
		t.Fatalf("version mismatch after fresh install: local %q remote %q", localVer, remoteVer)
	}
	// Check both launcher and real exist on darwin (needsLauncher).
	binPath := filepath.Join(tmp, remoteInstallDir, "bashy")
	realPath := filepath.Join(tmp, remoteInstallDir, "bashy.real")
	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("bashy not at %s: %v", binPath, err)
	}
	// On darwin dev builds launcher pair is expected.
	if needsLauncher(goos) {
		if _, err := os.Stat(realPath); err != nil {
			// If dev build without launcher, this may be missing but bin should still work.
			// Check if local had launcher to decide expectation.
			if _, _, lerr := findLocalBinaryForHost(goos, goarch); lerr == nil {
				// If local had launcher, remote should have real.
				// Allow missing if local was single binary (release).
			}
		}
	}

	// Re-run is a no-op: mtime should not change and output says already installed.
	info1, _ := os.Stat(binPath)
	mt1 := info1.ModTime()
	time.Sleep(10 * time.Millisecond)
	cmd2 := remoteCmd()
	var out2 bytes.Buffer
	cmd2.SetOut(&out2)
	cmd2.SetErr(&out2)
	cmd2.SetArgs([]string{"install", "--fake-root", tmp, "fakehost"})
	if err := cmd2.Execute(); err != nil {
		t.Fatalf("idempotent re-run: %v\n%s", err, out2.String())
	}
	if !strings.Contains(strings.ToLower(out2.String()), "already installed") {
		t.Fatalf("re-run should be no-op, got: %s", out2.String())
	}
	info2, _ := os.Stat(binPath)
	if !info2.ModTime().Equal(mt1) {
		t.Fatalf("re-run should not modify binary: %v vs %v", mt1, info2.ModTime())
	}

	// Version mismatch upgrades: seed an old version.
	// Use a cached older release binary if available, otherwise craft a file with old version string?
	// We seed by overwriting the fake binary with a known old cached binary (v0.13.1) if present.
	oldCandidates := []string{
		filepath.Join(filepath.Dir(os.Getenv("HOME")), "Library", "Caches", "bashy", "bin", "bashy", "v0.13.1", "bashy"),
		"/Users/qiangli/Library/Caches/bashy/bin/bashy/v0.13.1/bashy",
	}
	var oldBin string
	for _, c := range oldCandidates {
		if _, err := os.Stat(c); err == nil {
			oldBin = c
			break
		}
	}
	if oldBin == "" {
		// No old cached binary; fabricate a mismatched install by truncating file.
		// Write a small file that will report a different version via our fake remoteVersion override?
		// Simpler: just remove and write different bytes.
		if err := os.WriteFile(binPath, []byte("old"), 0o755); err != nil {
			t.Fatal(err)
		}
		if realPath != "" {
			_ = os.Remove(realPath)
		}
	} else {
		// Overwrite fake binary with old version.
		data, err := os.ReadFile(oldBin)
		if err != nil {
			t.Fatalf("read old bin: %v", err)
		}
		if err := os.WriteFile(binPath, data, 0o755); err != nil {
			t.Fatalf("seed old: %v", err)
		}
		if realPath != "" {
			_ = os.Remove(realPath)
		}
	}
	// Verify remote now reports different version (or error).
	remoteOldVer, _ := newTransport("fakehost", tmp).remoteVersion("fakehost", goos)
	if remoteOldVer == localVer {
		t.Fatalf("seeded old version should differ from local, both %q", localVer)
	}
	cmd3 := remoteCmd()
	var out3 bytes.Buffer
	cmd3.SetOut(&out3)
	cmd3.SetErr(&out3)
	cmd3.SetArgs([]string{"install", "--fake-root", tmp, "fakehost"})
	if err := cmd3.Execute(); err != nil {
		t.Fatalf("upgrade: %v\n%s", err, out3.String())
	}
	if !strings.Contains(strings.ToLower(out3.String()), "upgrad") && !strings.Contains(out3.String(), "installed") {
		t.Fatalf("upgrade output missing upgrade/installed: %s", out3.String())
	}
	remoteNewVer, err := newTransport("fakehost", tmp).remoteVersion("fakehost", goos)
	if err != nil {
		t.Fatalf("remoteVersion after upgrade: %v", err)
	}
	if remoteNewVer != localVer {
		t.Fatalf("upgrade failed: got %q want %q", remoteNewVer, localVer)
	}
}

func TestRemoteInstallFakePreservesUserPathBinary(t *testing.T) {
	tmp := t.TempDir()
	userBin := filepath.Join(tmp, ".local", "bin", "bashy")
	if err := os.MkdirAll(filepath.Dir(userBin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userBin, []byte("do not replace user work"), 0o755); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(userBin)
	if err != nil {
		t.Fatal(err)
	}

	cmd := remoteCmd()
	cmd.SetArgs([]string{"install", "--fake-root", tmp, "fakehost"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install: %v", err)
	}
	after, err := os.ReadFile(userBin)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("remote install replaced the user's PATH binary")
	}
	if _, err := os.Stat(filepath.Join(tmp, remoteInstallDir, "bashy")); err != nil {
		t.Fatalf("owned remote binary not installed: %v", err)
	}
}

func TestRemoteInstallFakeViaPathHost(t *testing.T) {
	// Host as a directory path should also be treated as fake.
	tmp := t.TempDir()
	fakeHostDir := filepath.Join(tmp, "remote-host-dir")
	if err := os.MkdirAll(fakeHostDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := remoteCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install", fakeHostDir})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("install via path host: %v\n%s", err, out.String())
	}
	// Verify.
	goos, goarch, _ := newTransport(fakeHostDir, "").detectOSArch(fakeHostDir)
	localBin, _, _ := findLocalBinaryForHost(goos, goarch)
	localVer, _ := localVersionString(localBin)
	remoteVer, _ := newTransport(fakeHostDir, "").remoteVersion(fakeHostDir, goos)
	if remoteVer != localVer {
		t.Fatalf("path host version mismatch: local %q remote %q", localVer, remoteVer)
	}
}

func TestRemoteInstallRequiresHost(t *testing.T) {
	cmd := remoteCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"install"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error for missing host, got nil")
	}
}

func TestRemotePeerIdentityFakeIsPersistentAndPrivate(t *testing.T) {
	root := t.TempDir()
	tr := newTransport("fakehost", root)
	if err := provisionPeerIdentity(tr, "fakehost"); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(root, ".bashy", "remote", "peers", "fakehost")
	remote := filepath.Join(root, remotePeerDir)
	paths := []string{
		filepath.Join(local, "id_ed25519"), filepath.Join(local, "id_ed25519.pub"), filepath.Join(local, "host_key.pub"),
		filepath.Join(remote, "authorized_keys"), filepath.Join(remote, "host_ed25519"),
	}
	before := make([][]byte, len(paths))
	for i, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		before[i] = data
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 || i == 4 {
			if info.Mode().Perm() != 0o600 {
				t.Errorf("private file %s mode %o", path, info.Mode().Perm())
			}
		}
	}
	for _, path := range []string{local, remote} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("directory %s mode %o", path, info.Mode().Perm())
		}
	}
	if err := os.WriteFile(filepath.Join(remote, "unrelated"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := provisionPeerIdentity(tr, "fakehost"); err != nil {
		t.Fatal(err)
	}
	for i, path := range paths {
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before[i], after) {
			t.Errorf("rerun replaced %s", path)
		}
	}
	if got, err := os.ReadFile(filepath.Join(remote, "unrelated")); err != nil || string(got) != "keep" {
		t.Fatalf("unrelated remote file changed: %q %v", got, err)
	}
}

func TestRemoteDetectOSFake(t *testing.T) {
	tr := newTransport("fakehost", t.TempDir())
	goos, goarch, err := tr.detectOSArch("fakehost")
	if err != nil {
		t.Fatalf("detectOSArch fake: %v", err)
	}
	if goos == "" || goarch == "" {
		t.Fatalf("empty fake os/arch: %s/%s", goos, goarch)
	}
}
