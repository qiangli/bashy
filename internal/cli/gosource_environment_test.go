package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"mvdan.cc/sh/v3/gosource"
)

// A real default executable is necessary: a package-only test cannot prove
// startup ordering relative to agentos.init or the cold CLI's shell enrichment.
func TestGoSourceExecutableEnvironment(t *testing.T) {
	dir := t.TempDir()
	sdk := filepath.Join(runtime.GOROOT(), "bin", "go")
	bashy := filepath.Join(dir, "bashy")
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, sdk, "build", "-p", "2", "-o", bashy, "github.com/qiangli/bashy/cmd/bashy")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("default CLI build: %v %s", err, output)
	}
	source, err := os.ReadFile("testdata/gosource-environment/environment-variables.go")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "original.go")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	oracle := filepath.Join(dir, "oracle")
	cmd = exec.CommandContext(ctx, sdk, "build", "-p", "2", "-o", oracle, path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("native build: %v %s", err, output)
	}
	cache, err := exec.CommandContext(ctx, sdk, "env", "GOCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	// The source importer needs an available build cache. Configure it equally
	// for both executables; the variables under test remain absent or supplied.
	baseEnv := []string{"HOME=" + dir, "GOCACHE=" + strings.TrimSpace(string(cache))}
	for name, env := range map[string][]string{
		"absent":   {"LAST=last", "BAR=original-bar", "FIRST=first"},
		"explicit": {"LAST=last", "BASH=caller-bash", "SHELL=caller-shell", "SHLVL=17", "UID=caller-uid", "EUID=caller-euid", "IFS=caller-ifs", "OPTIND=caller-optind", "BASH_VERSION=caller-version", "BASHY_AGENT_MANIFEST=caller-manifest", "FIRST=first"},
	} {
		t.Run(name, func(t *testing.T) {
			env = append(append([]string{}, baseEnv...), env...)
			run := func(binary string, args ...string) (string, string) {
				t.Helper()
				runCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				cmd := exec.CommandContext(runCtx, binary, args...)
				cmd.Dir = dir
				cmd.Env = append([]string{}, env...)
				var out, stderr bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("run %s: %v stdout=%q stderr=%q", filepath.Base(binary), err, out.String(), stderr.String())
				}
				return out.String(), stderr.String()
			}
			wantOut, wantErr := run(oracle)
			gotOut, gotErr := run(bashy, "--bashpp", "--source=go", path)
			if gotOut != wantOut || gotErr != wantErr {
				t.Fatalf("CLI stdout=%q stderr=%q; native stdout=%q stderr=%q", gotOut, gotErr, wantOut, wantErr)
			}
		})
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, source) {
		t.Fatal("original source changed")
	}
}

func TestGoSourceWarmSessionEnvironment(t *testing.T) {
	env := []string{"SECOND=two", "BASHY_BASHPP=1", "BASH=caller-bash", "FIRST=one"}
	var out, stderr bytes.Buffer
	runner, err := NewSessionRunnerWithConfig(SessionIO{Env: env, Dir: t.TempDir(), Stdout: &out, Stderr: &stderr}, SessionConfig{})
	if err != nil {
		t.Fatal(err)
	}
	program, err := gosource.Parse(strings.NewReader("package main\nimport \"fmt\"\nimport \"os\"\nfunc main(){fmt.Println(os.Environ())}"), "session.go", gosource.Options{RunMain: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := runner.Run(ctx, program.File); err != nil {
		t.Fatalf("warm Runner: %v %s", err, stderr.String())
	}
	want := "[" + strings.Join(env, " ") + "]\n"
	if out.String() != want || stderr.Len() != 0 {
		t.Fatalf("warm environment %q %q; want %q", out.String(), stderr.String(), want)
	}
}
