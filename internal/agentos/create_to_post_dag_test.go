// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

package agentos

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/feed"
)

func TestCreateToPostDAGFixture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("local Git sink and POSIX test-only rclone shim")
	}
	bin, root := synergyBashyBinary(t)
	dag := filepath.Join(root, "examples", "dag", "create-to-post", "dag.md")
	source := filepath.Join(root, "examples", "dag", "create-to-post", "blender-output.png")
	const wantSHA = "91ea15f875c3130683837d4a6538d1fbbcc3be69e22580531addbc06acb898e5"
	fixture, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(fixture); hex.EncodeToString(sum[:]) != wantSHA {
		t.Fatal("pre-created image fixture changed")
	}
	base := t.TempDir()
	home := filepath.Join(base, "home")
	storage := filepath.Join(base, "storage")
	sink := filepath.Join(base, "feed.git")
	for _, dir := range []string{home, storage} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitConfig := filepath.Join(base, "gitconfig")
	if err := os.WriteFile(gitConfig, []byte("[user]\nname = Fixture\nemail = fixture@example.invalid\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	shim := filepath.Join(base, "bashy-fixture")
	if err := os.WriteFile(shim, []byte("#!/bin/sh\nset -eu\nif [ \"${1-}\" = rclone ]; then\n  [ \"${2-}\" = copyto ] && [ \"$#\" -eq 4 ] || exit 2\n  cp \"$3\" \"$4\"\n  exit\nfi\nexec \"$REAL_BASHY\" \"$@\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + os.Getenv("PATH"), "HOME=" + home, "USERPROFILE=" + home,
		"BASHY_HOME=" + filepath.Join(home, "bashy"),
		"XDG_CONFIG_HOME=" + filepath.Join(home, "config"),
		"XDG_DATA_HOME=" + filepath.Join(home, "data"),
		"XDG_CACHE_HOME=" + filepath.Join(home, "cache"),
		"DAG_CACHE_DIR=" + filepath.Join(home, "dagcache"),
		"BASHY_HINTS=off", "BASHY_PRINCIPAL=fixture",
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + gitConfig,
		"REAL_BASHY=" + bin,
	}
	run := func(program string, args ...string) (string, error) {
		t.Helper()
		cmd := exec.Command(program, args...)
		cmd.Dir, cmd.Env = base, env
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("git", "init", "--bare", sink); err != nil {
		t.Fatalf("init local sink: %v\n%s", err, out)
	}
	if out, err := run(bin, "feed", "sink", "add", "github", "fixture", sink); err != nil {
		t.Fatalf("configure local sink: %v\n%s", err, out)
	}
	args := []string{"dag", "-f", dag, "-B", "BASHY_CLI=" + shim, "SOURCE=" + source,
		"STORAGE_ROOT=" + storage, "SINK_ROOT=" + sink}
	preview := append([]string{}, args...)
	preview[3] = "--dry-run"
	if out, err := run(bin, append(preview, "verify")...); err != nil {
		t.Fatalf("preview: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(storage, "blender-output.png")); !os.IsNotExist(err) {
		t.Fatalf("preview transferred artifact: %v", err)
	}
	if out, err := run(bin, append(args, "verify")...); err != nil {
		t.Fatalf("execute fixture: %v\n%s", err, out)
	}
	stored, err := os.ReadFile(filepath.Join(storage, "blender-output.png"))
	if err != nil || !bytes.Equal(stored, fixture) {
		t.Fatalf("rclone copyto storage differs from fixture: %v", err)
	}
	readGit := func(ref string) []byte {
		t.Helper()
		out, err := run("git", "--git-dir="+sink, "show", "main:"+ref)
		if err != nil {
			t.Fatalf("read sink %s: %v\n%s", ref, err, out)
		}
		return []byte(out)
	}
	attachment := readGit("posts/fixture/blender-output.png")
	if !bytes.Equal(attachment, fixture) {
		t.Fatal("feed attachment differs from pre-created Blender stand-in")
	}
	p, err := feed.ParsePost(readGit("posts/fixture.md"))
	if err != nil || feed.VerifyPost(p) != nil {
		t.Fatalf("published post lacks integrity: %v", err)
	}
	if p.Headline != "Blender fixture image" || p.Dek != "A pre-created image fixture transferred to local storage" ||
		!strings.Contains(p.Body, "pre-created Blender output stand-in; sha256 "+wantSHA) ||
		len(p.Attachments) != 1 || p.Attachments[0].Path != "fixture/blender-output.png" ||
		p.Attachments[0].SHA256 != wantSHA {
		t.Fatalf("feed metadata/provenance mismatch: %+v", p)
	}
	// The fixture is immutable input: corrupting it must fail before another post.
	bad := filepath.Join(base, "bad.png")
	if err := os.WriteFile(bad, []byte("not the fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	badArgs := append([]string{}, args...)
	badArgs[5] = "SOURCE=" + bad
	if out, err := run(bin, append(badArgs, "verify")...); err == nil {
		t.Fatalf("corrupt source was accepted: %s", out)
	}
	if got := readGit("posts/fixture/blender-output.png"); !bytes.Equal(got, fixture) {
		t.Fatal("failed run changed published attachment")
	}
}
