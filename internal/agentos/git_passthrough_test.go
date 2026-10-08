package agentos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/interp"
	"mvdan.cc/sh/v3/syntax"

	"github.com/qiangli/yoke/external/gitscm"
)

const gitPassthroughHelper = "BASHY_TEST_GIT_PASSTHROUGH"

func TestGitPassthroughHelper(t *testing.T) {
	if os.Getenv(gitPassthroughHelper) == "" {
		return
	}
	verb := os.Getenv("BASHY_TEST_GIT_VERB")
	if verb == "" {
		verb = "git"
	}
	args := []string{os.Args[0], verb}
	if v := os.Getenv("BASHY_TEST_GIT_ARGS"); v != "" {
		args = append(args, strings.Split(v, "\x1f")...)
	} else {
		for i, arg := range os.Args {
			if arg == "--" && i+1 < len(os.Args) {
				args = append([]string{os.Args[0]}, os.Args[i+1:]...)
				break
			}
		}
	}
	os.Args = args
	Dispatch()
	t.Fatal("front-door dispatch returned")
}

func TestGitPassthroughNormalNonzeroExitSilent(t *testing.T) {
	repo := t.TempDir()
	gitBin := gitscm.Path()
	initCmd := exec.Command(gitBin, "init", "-q", repo)
	if err := initCmd.Run(); err != nil {
		t.Skipf("git init failed: %v", err)
	}
	testFile := filepath.Join(repo, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("# agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	addCmd := exec.Command(gitBin, "-C", repo, "add", "test.txt")
	if err := addCmd.Run(); err != nil {
		t.Fatal(err)
	}
	commitCmd := exec.Command(gitBin, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "initial")
	if err := commitCmd.Run(); err != nil {
		t.Fatal(err)
	}

	for _, verb := range []string{"git", "git-scm"} {
		t.Run(verb+"_grep_nomatch", func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := exec.Command(os.Args[0], "-test.run=^TestGitPassthroughHelper$")
			cmd.Dir = repo
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			cmd.Env = append(os.Environ(),
				gitPassthroughHelper+"=1",
				"BASHY_TEST_GIT_VERB="+verb,
				"BASHY_TEST_GIT_ARGS="+strings.Join([]string{"grep", "-n", "zzqqxxnotfound", "--", "*.txt"}, "\x1f"),
			)
			err := cmd.Run()
			if err == nil {
				t.Fatalf("expected git grep with no match to exit non-zero, got exit 0")
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
			}
			if exitErr.ExitCode() != 1 {
				t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
			}
			if strings.Contains(stderr.String(), "Error: exit status") {
				t.Fatalf("stderr should be silent on normal non-zero exit, got: %q", stderr.String())
			}
		})

		t.Run(verb+"_diff_quiet_with_changes", func(t *testing.T) {
			if err := os.WriteFile(testFile, []byte("hello world modified\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = exec.Command(gitBin, "-C", repo, "checkout", "test.txt").Run()
			})
			var stdout, stderr bytes.Buffer
			cmd := exec.Command(os.Args[0], "-test.run=^TestGitPassthroughHelper$")
			cmd.Dir = repo
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			cmd.Env = append(os.Environ(),
				gitPassthroughHelper+"=1",
				"BASHY_TEST_GIT_VERB="+verb,
				"BASHY_TEST_GIT_ARGS="+strings.Join([]string{"diff", "--quiet"}, "\x1f"),
			)
			err := cmd.Run()
			if err == nil {
				t.Fatalf("expected git diff --quiet with changes to exit 1, got 0")
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
			}
			if exitErr.ExitCode() != 1 {
				t.Fatalf("expected exit code 1, got %d", exitErr.ExitCode())
			}
			if strings.Contains(stderr.String(), "Error: exit status") {
				t.Fatalf("stderr should be silent on normal non-zero exit, got: %q", stderr.String())
			}
		})

		t.Run(verb+"_propagates_exit_code_128", func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cmd := exec.Command(os.Args[0], "-test.run=^TestGitPassthroughHelper$")
			cmd.Dir = repo
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			cmd.Env = append(os.Environ(),
				gitPassthroughHelper+"=1",
				"BASHY_TEST_GIT_VERB="+verb,
				"BASHY_TEST_GIT_ARGS="+strings.Join([]string{"show", "deadbeef00000000000000000000000000000000"}, "\x1f"),
			)
			err := cmd.Run()
			if err == nil {
				t.Fatalf("expected git show non-existent sha to exit 128, got 0")
			}
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("expected *exec.ExitError, got %T: %v", err, err)
			}
			if exitErr.ExitCode() != 128 {
				t.Fatalf("expected exit code 128, got %d", exitErr.ExitCode())
			}
			if strings.Contains(stderr.String(), "Error: exit status") {
				t.Fatalf("stderr should not contain 'Error: exit status', got: %q", stderr.String())
			}
		})
	}
}

// TestGitPassthroughPreamblePipeline replicates the exact bug report:
// bashy -c 'git -C <repo> grep -n zzqqxxnotfound -- "*.md" | wc -l'
// must print 0 on stdout and NOTHING on stderr.
func TestGitPassthroughPreamblePipeline(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pipeline test uses Unix wrapper script")
	}

	repo := t.TempDir()
	gitBin := gitscm.Path()
	initCmd := exec.Command(gitBin, "init", "-q", repo)
	if err := initCmd.Run(); err != nil {
		t.Skipf("git init failed: %v", err)
	}
	testFile := filepath.Join(repo, "test.md")
	if err := os.WriteFile(testFile, []byte("hello world\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "AGENTS.md"), []byte("# agent\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command(gitBin, "-C", repo, "add", "test.md", "AGENTS.md").Run(); err != nil {
		t.Fatal(err)
	}
	if err := exec.Command(gitBin, "-C", repo, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-q", "-m", "initial").Run(); err != nil {
		t.Fatal(err)
	}

	// Create a wrapper script that acts as BASHY_SELF and invokes the test helper.
	wrapperDir := t.TempDir()
	wrapperScript := filepath.Join(wrapperDir, "bashy-wrapper.sh")
	wrapperContent := fmt.Sprintf("#!/bin/sh\nexport %s=1\nexec %s -test.run=^TestGitPassthroughHelper$ -- \"$@\"\n",
		gitPassthroughHelper, shellQuote(os.Args[0]))
	if err := os.WriteFile(wrapperScript, []byte(wrapperContent), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("BASHY_SELF", wrapperScript)

	script := fmt.Sprintf("git -C %s grep -n zzqqxxnotfound -- \"*.md\" | wc -l\n", shellQuote(repo))
	prog, err := syntax.NewParser().Parse(strings.NewReader(Preamble()+script), "")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	var stdout, stderr bytes.Buffer
	r, err := interp.New(
		interp.StdIO(nil, &stdout, &stderr),
		interp.Env(expand.ListEnviron(os.Environ()...)),
	)
	if err != nil {
		t.Fatalf("interp.New: %v", err)
	}

	if err := r.Run(context.Background(), prog); err != nil {
		t.Fatalf("run failed: %v, stderr=%q", err, stderr.String())
	}

	gotOut := strings.TrimSpace(stdout.String())
	if gotOut != "0" {
		t.Fatalf("expected stdout '0', got %q", gotOut)
	}
	if strings.Contains(stderr.String(), "Error: exit status") {
		t.Fatalf("expected silent stderr, got: %q", stderr.String())
	}
	if stderr.String() != "" {
		t.Fatalf("expected empty stderr, got: %q", stderr.String())
	}
}
