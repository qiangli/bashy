//go:build bashy_core && !windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// This is a non-shipping import-boundary proof: one Bashy inode must serve
// shell, Coreutils, and the minimal registered-command CRUD routes.
func TestCoreRouteOneFileAndCommandCRUD(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a one-file core profile")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "bashy")
	build := exec.Command("go", "build", "-tags", "bashy_core", "-trimpath", "-ldflags=-w", "-o", bin, "./cmd/bashy")
	build.Dir = filepath.Join("..", "..")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Bashy core: %v\n%s", err, out)
	}
	for _, name := range []string{"sh", "kill"} {
		if err := os.Symlink(bin, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
		physical, err := os.Stat(bin)
		if err != nil {
			t.Fatal(err)
		}
		alias, err := os.Stat(filepath.Join(dir, name))
		if err != nil || !os.SameFile(physical, alias) {
			t.Fatalf("%s is not the same executable inode: %v", name, err)
		}
	}
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"shell", []string{filepath.Join(dir, "sh"), "-c", "printf shell-ok"}, "shell-ok"},
		{"utility", []string{filepath.Join(dir, "kill"), "-l", "9"}, "KILL\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := exec.Command(tc.argv[0], tc.argv[1:]...).CombinedOutput()
			if err != nil || string(out) != tc.want {
				t.Fatalf("route %v: %v; output=%q, want %q", tc.argv, err, out, tc.want)
			}
		})
	}
	// The base profile retains Bash# Go-source execution. Its loader belongs
	// to the language, even though optional AgentOS imports stay absent.
	goSource := filepath.Join(dir, "minimal.go")
	if err := os.WriteFile(goSource, []byte("package main\nfunc main() { println(\"go-source-ok\") }\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(bin, "--source=go", "--bashsharp", goSource).CombinedOutput(); err != nil || string(out) != "go-source-ok\n" {
		t.Fatalf("Bash# Go source: %v; output=%q", err, out)
	}
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/s355core\n\ngo 1.27.1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fence := filepath.Join(dir, "fence.bsh")
	if err := os.WriteFile(fence, []byte("~~~go as go\nfunc Greet(name string) string { return \"hello \"+name }\n~~~\nvalue := go.Greet(world)\necho \"$value\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	fenceCmd := exec.Command(bin, "--bashsharp", fence)
	fenceCmd.Env = append(os.Environ(), "BASHPP_GO="+goTool)
	if out, err := fenceCmd.CombinedOutput(); err != nil || string(out) != "hello world\n" {
		t.Fatalf("Bash# Go fence: %v; output=%q", err, out)
	}
	ring := filepath.Join(dir, "ring")
	env := append(os.Environ(), "BASHY_COMMANDS_DIR="+ring)
	run := func(want string, args ...string) {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(out), want) {
			t.Fatalf("bashy %v: %v; output=%q, want %q", args, err, out, want)
		}
	}
	run("s355fixture", "commands", "add", "s355fixture", "--set", "exec.0=/bin/echo")
	run("s355fixture", "commands", "set", "s355fixture", "--set", "synopsis=core route")
	run("ok", "commands", "verify", "s355fixture")
	run("core route", "commands", "show", "s355fixture", "--json")
	run("s355fixture is a bashy registered command", "-c", "type s355fixture")
	run("in-shell", "-c", "s355fixture in-shell")
	run("front-door", "s355fixture", "front-door")
	run("s355script", "commands", "add", "s355script", "--set", "script=printf script-ok", "--set", "effects.0=read")
	run("script-ok", "-c", "s355script")
	cert := exec.Command(bin, "-c", "command -v s355fixture")
	cert.Env = append(env, "VSC_PROFILE=cert")
	if out, err := cert.CombinedOutput(); err == nil || len(out) != 0 {
		t.Fatalf("cert route exposed registered command: err=%v output=%q", err, out)
	}
	run("removed", "commands", "rm", "s355fixture")
	run("removed", "commands", "rm", "s355script")
}
