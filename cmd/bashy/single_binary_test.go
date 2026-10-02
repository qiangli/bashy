package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestInvocationRouting(t *testing.T) {
	for _, tc := range []struct {
		argv0   string
		shell   bool
		utility bool
	}{
		{"/bin/bashy", false, false},
		{"/bin/sh", true, false},
		{"/bin/-sh", true, false},
		{"/bin/bash.exe", true, false},
		{"/bin/cat", false, true},
		{"/bin/coreutils", false, true},
		{"/bin/not-a-command", false, false},
	} {
		if got := shellInvocation(tc.argv0); got != tc.shell {
			t.Errorf("shellInvocation(%q) = %v, want %v", tc.argv0, got, tc.shell)
		}
		if got := utilityInvocation(tc.argv0); got != tc.utility {
			t.Errorf("utilityInvocation(%q) = %v, want %v", tc.argv0, got, tc.utility)
		}
	}
}

func TestOneBinaryShellAndUtilityRoutes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix executable symlinks and POSIX PATH")
	}
	if testing.Short() {
		t.Skip("builds the product binary")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bashy")
	build := exec.Command("go", "build", "-o", bin, "./cmd/bashy")
	build.Dir = filepath.Join("..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build bashy: %v\n%s", err, out)
	}
	for _, name := range []string{"sh", "bash", "cat", "coreutils"} {
		alias := filepath.Join(dir, name)
		if err := os.Symlink(bin, alias); err != nil {
			t.Fatal(err)
		}
		payload, err := os.Stat(bin)
		if err != nil {
			t.Fatal(err)
		}
		linked, err := os.Stat(alias)
		if err != nil {
			t.Fatal(err)
		}
		if !os.SameFile(payload, linked) {
			t.Fatalf("%s does not resolve to the bashy payload", name)
		}
	}
	run := func(name string, env []string, input string, args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(filepath.Join(dir, name), args...)
		cmd.Env = env
		cmd.Stdin = strings.NewReader(input)
		out, err := cmd.CombinedOutput()
		if err == nil {
			return string(out), 0
		}
		if exit, ok := err.(*exec.ExitError); ok {
			return string(out), exit.ExitCode()
		}
		t.Fatalf("run %s: %v", name, err)
		return "", -1
	}
	baseEnv := append(os.Environ(), "BASHY_SESSION=", "BASHY_AGENTIC=0")
	if out, code := run("cat", baseEnv, "same file\n"); code != 0 || out != "same file\n" {
		t.Fatalf("cat alias: code=%d output=%q", code, out)
	}
	if out, code := run("coreutils", baseEnv, "multicall\n", "cat"); code != 0 || out != "multicall\n" {
		t.Fatalf("coreutils alias: code=%d output=%q", code, out)
	}
	noPath := append(baseEnv, "PATH=")
	if out, code := run("sh", noPath, "", "-c", "cat /dev/null"); code != 127 {
		t.Fatalf("strict sh bypassed empty PATH: code=%d output=%q", code, out)
	}
	if out, code := run("bash", noPath, "", "-c", "cat /dev/null"); code != 127 {
		t.Fatalf("bash bypassed empty PATH: code=%d output=%q", code, out)
	}
	withPath := append(baseEnv, "PATH="+dir)
	if out, code := run("sh", withPath, "", "-c", "cat <<EOF\nvia PATH\nEOF"); code != 0 || out != "via PATH\n" {
		t.Fatalf("sh to same-file cat via PATH: code=%d output=%q", code, out)
	}
	if out, code := run("bashy", baseEnv, "", "--version"); code != 0 || !strings.Contains(out, "bashy") {
		t.Fatalf("bashy front door: code=%d output=%q", code, out)
	}
}
