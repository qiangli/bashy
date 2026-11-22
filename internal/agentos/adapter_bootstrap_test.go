package agentos

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qiangli/bash/expand"
	"github.com/qiangli/bash/interp"
	"github.com/qiangli/bash/syntax"
)

// Exercise the shipped bootstrap with a tiny, dependency-free adapter payload.
// Count and cap every shell process launch BEFORE exec, so a regression cannot
// fork-bomb the test host. Compiler subprocesses are owned by the single go build.
func runColdAdapterBootstrap(t *testing.T, folder, entry string, prepare func(string)) int32 {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("Go unavailable")
	}
	_, source, _, _ := runtime.Caller(0)
	root, ok := findBashySourceRoot(filepath.Dir(source))
	if !ok {
		t.Fatal("source root unavailable")
	}
	raw, err := os.ReadFile(filepath.Join(root, folder, entry, entry+".bsh"))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "adapter")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, entry+".bsh")
	for name, body := range map[string]string{
		entry + ".bsh": string(raw),
		"go.mod":       "module example.com/bootstrap\n\ngo 1.21\n",
		"main.go":      "package main\nimport \"fmt\"\nfunc main() { fmt.Println(\"adapter ready\") }\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if prepare != nil {
		prepare(dir)
	}
	t.Setenv("BASHY_AGENTIC", "1")
	t.Setenv(adapterExecEnv, "1")
	t.Setenv("BASHY_SELF", filepath.Join(dir, "bashy"))
	cache := t.TempDir()
	t.Setenv("BASHY_ADAPTER_CACHE", cache)
	t.Setenv("GOTOOLCHAIN", "local")
	// The preamble is installed at shell startup, before the adapter unsets env.
	prog, err := syntax.NewParser().Parse(strings.NewReader(PreambleFor(true)+string(raw)), script)
	if err != nil {
		t.Fatal(err)
	}
	var launches atomic.Int32
	var out bytes.Buffer
	handler := interp.DefaultExecHandler(2 * time.Second)
	r, err := interp.New(interp.Env(expand.ListEnviron(os.Environ()...)), interp.StdIO(nil, &out, &out),
		interp.ExecHandler(func(ctx context.Context, args []string) error {
			if launches.Add(1) > 8 {
				return fmt.Errorf("bootstrap exceeded 8 process launches")
			}
			if entry == "dag" && filepath.Base(args[0]) == "go" {
				fmt.Fprintln(interp.HandlerCtx(ctx).Stderr, "go: downloading fixture")
			}
			if args[0] == filepath.Join(dir, "bashy") {
				// Stand in for the in-process Go provisioner, never another adapter.
				if entry != "go" || len(args) < 2 || args[1] != "go" || interp.HandlerCtx(ctx).Env.Get(adapterInprocEnv).String() != "1" {
					return fmt.Errorf("recursive adapter dispatch: %v", args)
				}
				args = append([]string{goBin}, args[2:]...)
			}
			return handler(ctx, args)
		}))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := r.Run(ctx, prog); err != nil {
		t.Fatalf("cold %s/%s: %v\n%s", folder, entry, err, &out)
	}
	if strings.TrimSpace(out.String()) != "adapter ready" {
		t.Fatalf("payload did not execute: %s", &out)
	}
	if entry == "dag" {
		name := "dag"
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		log, err := os.ReadFile(filepath.Join(cache, "yoke", name+".build.log"))
		if err != nil || !strings.Contains(string(log), "go: downloading fixture") {
			t.Fatalf("DAG bootstrap diagnostics were not retained: %q (%v)", log, err)
		}
	}
	return launches.Load()
}

func TestAgentModeColdAdapterBuildBounded(t *testing.T) {
	for _, adapter := range [][2]string{{"yoke", "judge"}, {"toolchain", "go"}, {"yoke", "dag"}} {
		t.Run(adapter[1], func(t *testing.T) {
			n := runColdAdapterBootstrap(t, adapter[0], adapter[1], nil)
			if n < 2 || n > 8 {
				t.Fatalf("process launches = %d, want 2..8", n)
			}
			t.Logf("cold bootstrap completed with %d shell process launches", n)
		})
	}
}

// Git hooks export repository-relative GIT_DIR values. Once the adapter cds
// into its own source tree that path addresses a different (or absent) repo.
func TestColdAdapterBuildWithForeignGitDir(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git unavailable")
	}
	foreign := t.TempDir()
	if out, err := exec.Command(git, "init", filepath.Join(foreign, ".hook-repo")).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	t.Chdir(foreign)
	t.Setenv("GIT_DIR", ".hook-repo/.git")
	t.Setenv("GIT_INDEX_FILE", filepath.Join(foreign, ".hook-repo", ".git", "index"))
	runColdAdapterBootstrap(t, "yoke", "judge", func(dir string) {
		cmd := exec.Command(git, "init", filepath.Dir(dir))
		cmd.Env = stripEnvKey(stripEnvKey(os.Environ(), "GIT_DIR"), "GIT_INDEX_FILE")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("adapter git init: %v: %s", err, out)
		}
	})
}
