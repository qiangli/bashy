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
	// `git -C DIR` re-enters as `bashy -c 'awd "$@"' bashy awd DIR -- SELF git ...`;
	// the wrapper plays the shell's awd (cd for one command) and re-dispatches.
	wrapperContent := fmt.Sprintf("#!/bin/sh\nexport %s=1\nif [ \"$1\" = -c ]; then shift 3; d=$1; shift 2; cd \"$d\" || exit 1; exec \"$@\"; fi\nexec %s -test.run=^TestGitPassthroughHelper$ -- \"$@\"\n",
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

// Decision reads through the front door: flagged status / ls-files forms
// reach the engine verbatim and match host git byte for byte, and
// diff --quiet signals only through its exit status (no stderr line).
func TestGitDecisionReadsMatchHostGit(t *testing.T) {
	gitBin := gitscm.Path()
	repo := t.TempDir()
	host := func(args ...string) string {
		t.Helper()
		c := exec.Command(gitBin, args...)
		c.Dir = repo
		c.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := c.Output()
		if err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				t.Skipf("host git unavailable: %v", err)
			}
		}
		return string(out)
	}
	host("init", "-q")
	for name, body := range map[string]string{"a.txt": "a\n", "AGENTS.md": "# agent\n"} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	host("add", "-A")
	host("commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git", "info", "exclude"), []byte("secret.json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"secret.json", "new/one.txt"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(repo, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(repo, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	front := func(args ...string) (string, string, int) {
		var stdout, stderr bytes.Buffer
		cmd := exec.Command(os.Args[0], "-test.run=^TestGitPassthroughHelper$")
		cmd.Dir = repo
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		cmd.Env = append(os.Environ(), gitPassthroughHelper+"=1", "BASHY_TEST_GIT_VERB=git",
			"BASHY_TEST_GIT_ARGS="+strings.Join(args, "\x1f"))
		code := 0
		if err := cmd.Run(); err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("%v: %v", args, err)
			}
			code = exitErr.ExitCode()
		}
		return stdout.String(), stderr.String(), code
	}
	for _, args := range [][]string{
		{"status", "--porcelain"},
		{"status", "--short", "-uall"},
		{"ls-files", "-o", "--exclude-standard"},
		{"ls-files", "-m"},
	} {
		got, stderr, code := front(args...)
		if want := host(args...); got != want || code != 0 {
			t.Errorf("bashy git %v = %q (exit %d, stderr %q); host git = %q", args, got, code, stderr, want)
		}
	}
	if out, stderr, code := front("diff", "--quiet"); code != 1 || out != "" || stderr != "" {
		t.Errorf("bashy git diff --quiet = exit %d, stdout %q, stderr %q; want exit 1 and no output", code, out, stderr)
	}
}
