package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Sprint 245, story 682: type.tests takes $THIS_SH apart as POSIX text —
// SHBASE=${THIS_SH##*/}; hash -p /tmp/$SHBASE $SHBASE; type -p $SHBASE —
// and type.right wants /tmp/bash. A Windows testee is bash.exe, so the
// binary's own path made SHBASE "bash.exe" and the whole block carried the
// suffix. The fixtures are handed the root's extensionless twin instead.
func TestStory682FixtureThisSHIsExtensionless(t *testing.T) {
	old := fixtureThisSH
	defer func() { fixtureThisSH = old }()

	dir := t.TempDir()
	bashPath := filepath.Join(dir, "bin", exeName("bash"))

	// Without a prepared root (every Unix run) the binary's own path is
	// handed over, exactly as before.
	fixtureThisSH = ""
	env := fixtureEnv(dir, filepath.Join(dir, "tests"), bashPath, "type")
	if got := envValue(env, "THIS_SH"); got != posixSpelling(bashPath, runtime.GOOS == "windows") {
		t.Errorf("THIS_SH without a root = %q", got)
	}

	fixtureThisSH = fixtureThisSHPath
	env = fixtureEnv(dir, filepath.Join(dir, "tests"), bashPath, "type")
	for _, name := range []string{"THIS_SH", "_"} {
		got := envValue(env, name)
		if got != fixtureThisSHPath {
			t.Errorf("%s = %q, want %q", name, got, fixtureThisSHPath)
		}
		if base := got[strings.LastIndex(got, "/")+1:]; base != "bash" {
			t.Errorf("${%s##*/} = %q, want bash — type.right's /tmp/bash depends on it", name, base)
		}
	}
}

// The path THIS_SH names is a real extensionless file in the prepared root,
// so `cp $THIS_SH $TDIR` (exec3.sub, posixexp.tests) copies something.
func TestStory682FixtureThisSHNamesARealFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub multicall is a POSIX shell script; the CI leg covers the real binary")
	}
	dir := t.TempDir()
	stub := filepath.Join(dir, "yoke")
	os.WriteFile(stub, []byte("#!/bin/sh\nprintf 'cat\\n'\n"), 0o755)
	testee := filepath.Join(dir, "bash")
	os.WriteFile(testee, []byte("#!/bin/sh\necho testee\n"), 0o755)
	root, _, _, err := prepareUserland(dir, stub, testee)
	if err != nil {
		t.Fatal(err)
	}
	// fixtureThisSHPath is rooted at the virtual root the shell mounts.
	inRoot := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(fixtureThisSHPath, "/")))
	info, err := os.Stat(inRoot)
	if err != nil {
		t.Fatalf("%s: %v", fixtureThisSHPath, err)
	}
	if info.IsDir() {
		t.Errorf("%s is a directory", fixtureThisSHPath)
	}
	if filepath.Ext(inRoot) != "" {
		t.Errorf("%s carries an extension", fixtureThisSHPath)
	}
}

func envValue(env []string, name string) string {
	// The last assignment wins, as it does for a child process.
	val := ""
	for _, kv := range env {
		if n, v, ok := strings.Cut(kv, "="); ok && n == name {
			val = v
		}
	}
	return val
}
