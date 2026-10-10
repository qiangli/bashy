package agentos

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestGitCommandWiring exercises the cobra wiring end-to-end against
// an on-disk tempdir repo. The library logic is covered in
// coreutils/git; this test catches drift between the
// cobra flags / arg shapes and the library API.
func TestGitCommandWiring(t *testing.T) {
	dir := t.TempDir()

	// Construct the top-level command once; reuse for each invocation.
	// We need a fresh tree per invocation because cobra mutates the
	// captured-flag closures, but for these tests we use distinct
	// subtrees so no state leaks.
	run := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		root := gitCmd()
		buf := &bytes.Buffer{}
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs(args)
		err := root.Execute()
		return buf.String(), err
	}

	// init
	out, err := run(t, "init", dir)
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		t.Fatalf(".git missing after init: %v", err)
	}

	// Write a file directly; the `add` subcommand resolves paths
	// relative to cwd, so we cd into the tempdir for the rest.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	// add a.txt
	if out, err := run(t, "add", "a.txt"); err != nil || out != "" {
		t.Fatalf("add: err=%v, output=%q; want silent success", err, out)
	}

	// status — should not be clean
	out, err = run(t, "status")
	if err != nil {
		t.Fatalf("status: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Changes to be committed") {
		t.Errorf("expected staged-changes header, got:\n%s", out)
	}

	// commit (author flags so we don't need host git config)
	if out, err := run(t,
		"commit",
		"-m", "first",
		"--author-name", "T U",
		"--author-email", "tu@example.com",
	); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}

	// status — clean
	out, err = run(t, "status")
	if err != nil {
		t.Fatalf("status post-commit: %v\n%s", err, out)
	}
	if !strings.Contains(out, "clean") {
		t.Errorf("expected clean message, got:\n%s", out)
	}

	// log
	out, err = run(t, "log", "-n", "5")
	if err != nil {
		t.Fatalf("log: %v\n%s", err, out)
	}
	if !strings.Contains(out, "first") {
		t.Errorf("log missing commit message, got:\n%s", out)
	}

	// branch (list)
	out, err = run(t, "branch")
	if err != nil {
		t.Fatalf("branch list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "* ") {
		t.Errorf("branch list missing current marker, got:\n%s", out)
	}

	// checkout -b feature
	if out, err := run(t, "checkout", "-b", "feature"); err != nil {
		t.Fatalf("checkout -b: %v\n%s", err, out)
	}

	// show HEAD
	out, err = run(t, "show")
	if err != nil {
		t.Fatalf("show: %v\n%s", err, out)
	}
	if !strings.Contains(out, "commit ") || !strings.Contains(out, "Author: T U") {
		t.Errorf("show output malformed:\n%s", out)
	}

	// remote (empty)
	out, err = run(t, "remote")
	if err != nil {
		t.Fatalf("remote: %v\n%s", err, out)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected empty remote list, got:\n%s", out)
	}

	// diff (no changes)
	out, err = run(t, "diff")
	if err != nil {
		t.Fatalf("diff: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no changes") {
		t.Errorf("expected 'no changes' for clean tree, got:\n%s", out)
	}
}

// TestGitAddRequiresPathOrAllFlag verifies that `bashy git add` with
// no args and no -A errors out cleanly instead of silently doing
// nothing.
func TestGitAddRequiresPathOrAllFlag(t *testing.T) {
	dir := t.TempDir()
	root := gitCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"init", dir})
	if err := root.Execute(); err != nil {
		t.Fatalf("init: %v\n%s", err, buf.String())
	}

	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	root = gitCmd()
	buf.Reset()
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{"add"})
	if err := root.Execute(); err == nil {
		t.Errorf("expected error for bare 'add', got nil")
	}
}

// TestGitParityVerbsWiring exercises the cobra wiring of the parity
// verbs (merge, merge-base, rev-list, config, tag, reset, rm,
// ls-files, blame, grep, diff <rev> <rev>, show --no-patch). The
// behavior itself is covered in coreutils/git; this catches
// flag/arg drift.
func TestGitParityVerbsWiring(t *testing.T) {
	dir := t.TempDir()

	run := func(t *testing.T, args ...string) (string, error) {
		t.Helper()
		root := gitCmd()
		buf := &bytes.Buffer{}
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs(args)
		err := root.Execute()
		return buf.String(), err
	}

	if out, err := run(t, "init", dir); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	commit := func(t *testing.T, file, content, msg string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", file, err)
		}
		if out, err := run(t, "add", file); err != nil {
			t.Fatalf("add: %v\n%s", err, out)
		}
		if out, err := run(t, "commit", "-m", msg, "--author-name", "T", "--author-email", "t@e"); err != nil {
			t.Fatalf("commit: %v\n%s", err, out)
		}
	}
	commit(t, "a.txt", "line1\n", "first")
	commit(t, "a.txt", "line1\nline2\n", "second")

	// config set + get
	if out, err := run(t, "config", "user.name", "Wire Test"); err != nil {
		t.Fatalf("config set: %v\n%s", err, out)
	}
	out, err := run(t, "config", "user.name")
	if err != nil || !strings.Contains(out, "Wire Test") {
		t.Errorf("config get: %v\n%s", err, out)
	}

	// tag create + list + delete
	if out, err := run(t, "tag", "v1.0.0"); err != nil {
		t.Fatalf("tag: %v\n%s", err, out)
	}
	out, err = run(t, "tag")
	if err != nil || !strings.Contains(out, "v1.0.0") {
		t.Errorf("tag list: %v\n%s", err, out)
	}
	if out, err := run(t, "tag", "-d", "v1.0.0"); err != nil {
		t.Fatalf("tag -d: %v\n%s", err, out)
	}

	// merge fast-forward via a feature branch
	if out, err := run(t, "checkout", "-b", "feature"); err != nil {
		t.Fatalf("checkout -b: %v\n%s", err, out)
	}
	commit(t, "feat.txt", "feature\n", "feature work")
	if out, err := run(t, "checkout", "master"); err != nil {
		// go-git default branch is master; fall back to main
		if out2, err2 := run(t, "checkout", "main"); err2 != nil {
			t.Fatalf("checkout default branch: %v %v\n%s%s", err, err2, out, out2)
		}
	}
	out, err = run(t, "merge", "feature")
	if err != nil || !strings.Contains(out, "Fast-forward") {
		t.Fatalf("merge: %v\n%s", err, out)
	}

	// merge-base + rev-list --count
	out, err = run(t, "merge-base", "HEAD", "feature")
	if err != nil || len(strings.TrimSpace(out)) != 40 {
		t.Errorf("merge-base: %v\n%s", err, out)
	}
	out, err = run(t, "rev-list", "--count", "HEAD")
	if err != nil || strings.TrimSpace(out) != "3" {
		t.Errorf("rev-list --count HEAD: %v\n%s", err, out)
	}

	// ls-files
	out, err = run(t, "ls-files")
	if err != nil || !strings.Contains(out, "a.txt") || !strings.Contains(out, "feat.txt") {
		t.Errorf("ls-files: %v\n%s", err, out)
	}

	// blame with -L
	out, err = run(t, "blame", "-L", "2,2", "a.txt")
	if err != nil || !strings.Contains(out, "line2") {
		t.Errorf("blame: %v\n%s", err, out)
	}

	// grep hit and miss (miss exits non-zero, like git grep)
	out, err = run(t, "grep", "line2")
	if err != nil || !strings.Contains(out, "a.txt:2:") {
		t.Errorf("grep: %v\n%s", err, out)
	}
	if _, err = run(t, "grep", "no-such-string-anywhere"); err == nil {
		t.Errorf("grep miss: expected non-zero")
	}

	// diff between revisions emits a real patch
	out, err = run(t, "diff", "HEAD~2", "HEAD")
	if err != nil || !strings.Contains(out, "+line2") {
		t.Errorf("diff revs: %v\n%s", err, out)
	}

	// show includes the patch; --no-patch suppresses it
	out, err = run(t, "show", "HEAD~1")
	if err != nil || !strings.Contains(out, "+line2") {
		t.Errorf("show with patch: %v\n%s", err, out)
	}
	out, err = run(t, "show", "--no-patch", "HEAD~1")
	if err != nil || strings.Contains(out, "+line2") {
		t.Errorf("show --no-patch: %v\n%s", err, out)
	}

	// rm --cached keeps the file on disk
	if out, err := run(t, "rm", "--cached", "feat.txt"); err != nil || out != "" {
		t.Fatalf("rm --cached: err=%v, output=%q; want silent success", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "feat.txt")); err != nil {
		t.Errorf("rm --cached deleted the file: %v", err)
	}

	// reset --hard back to the tip (also re-tracks nothing; just wiring)
	out, err = run(t, "reset", "--hard", "HEAD")
	if err != nil || !strings.Contains(out, "HEAD is now at") {
		t.Errorf("reset --hard: %v\n%s", err, out)
	}
}

// TestGitUnimplementedVerbsError verifies the post-S252.6 dispatch contract:
// verbs the native engine cannot serve fail with a workaround hint (and an
// --external pointer), verbs it serves but rejects for the given form surface
// the engine's own loud refusal, and genuinely unknown verbs get a pointer
// to --help. Native-by-default: no host binary is consulted without the flag.
func TestGitUnimplementedVerbsError(t *testing.T) {
	// Engine dispatch is real: bare `stash` would snapshot a dirty repo,
	// so run everything from an empty non-repo directory (never the
	// checkout under test, never anywhere DetectDotGit can escape to
	// a repo — t.TempDir() has no repo parents).
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	if err := os.Chdir(empty); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(prev) }()
	run := func(args ...string) (string, error) {
		root := gitCmd()
		buf := &bytes.Buffer{}
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs(args)
		err := root.Execute()
		return buf.String(), err
	}

	for verb, wantHint := range map[string]string{
		"rebase": "merge <base>",
		"stash":  "push/pop/list run natively",
	} {
		_, err := run(verb)
		if err == nil {
			t.Fatalf("%s: expected error", verb)
		}
		if !strings.Contains(err.Error(), "not served") || !strings.Contains(err.Error(), wantHint) {
			t.Errorf("%s: error missing not-served note or hint %q:\n%v", verb, wantHint, err)
		}
	}

	// Engine-dispatched but rejected for the form: `stash drop` reaches
	// the engine (no repo touched) and comes back loud with the hint.
	if _, err := run("stash", "drop"); err == nil || !strings.Contains(err.Error(), "push/pop/list run natively") {
		t.Errorf("stash drop: %v", err)
	}

	// `clean` with no mode flag refuses like host git (fatal 128), even
	// with no repository involved.
	if out, err := run("clean"); err == nil || !strings.Contains(err.Error(), "exit status 128") || !strings.Contains(out, "requireForce") {
		t.Errorf("clean: out=%q err=%v", out, err)
	}

	// Flags meant for the unimplemented verb don't derail the message.
	if _, err := run("rebase", "-i", "main"); err == nil || !strings.Contains(err.Error(), "not served") {
		t.Errorf("rebase -i: %v", err)
	}

	// Truly unknown verbs carry the engine's external fallback hint too.
	if _, err := run("frobnicate"); err == nil || !strings.Contains(err.Error(), "--external=true") {
		t.Errorf("frobnicate: %v", err)
	}

	// Bare `bashy git` prints help, no error.
	out, err := run()
	if err != nil || !strings.Contains(out, "self-contained git client") {
		t.Errorf("bare git: err=%v out:\n%s", err, out)
	}
}

func TestGitBootstrapRemoteWorkflow(t *testing.T) {
	rootDir := t.TempDir()
	origin := filepath.Join(rootDir, "origin")
	clone := filepath.Join(rootDir, "clone")

	run := func(t *testing.T, cwd string, args ...string) (string, error) {
		t.Helper()
		prev, err := os.Getwd()
		if err != nil {
			t.Fatalf("getwd: %v", err)
		}
		if cwd != "" {
			if err := os.Chdir(cwd); err != nil {
				t.Fatalf("chdir %s: %v", cwd, err)
			}
		}
		defer func() { _ = os.Chdir(prev) }()

		cmd := gitCmd()
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs(args)
		err = cmd.Execute()
		return buf.String(), err
	}
	commitFile := func(t *testing.T, cwd, name, body, msg string) {
		t.Helper()
		full := filepath.Join(cwd, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
		if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if out, err := run(t, cwd, "add", "-A"); err != nil {
			t.Fatalf("add -A: %v\n%s", err, out)
		}
		if out, err := run(t, cwd, "commit", "-m", msg, "--author-name", "T", "--author-email", "t@e"); err != nil {
			t.Fatalf("commit %q: %v\n%s", msg, err, out)
		}
	}

	if out, err := run(t, "", "init", origin); err != nil {
		t.Fatalf("init origin: %v\n%s", err, out)
	}
	commitFile(t, origin, "README.md", "main\n", "main seed")
	if out, err := run(t, origin, "checkout", "-b", "topic"); err != nil {
		t.Fatalf("checkout -b topic: %v\n%s", err, out)
	}
	commitFile(t, origin, "topic.txt", "topic\n", "topic seed")

	shallowClone := filepath.Join(rootDir, "shallow")
	out, err := run(t, "", "clone", "--quiet", "--depth", "1", origin, shallowClone)
	if err == nil || !strings.Contains(err.Error(), "shallow") {
		t.Fatalf("local shallow clone should report transport shallow limitation, err=%v out=%s", err, out)
	}

	out, err = run(t, "", "clone", "--quiet", "--branch", "topic", "--single-branch", origin, clone)
	if err != nil {
		t.Fatalf("clone topic: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Cloned into") {
		t.Fatalf("clone output missing success message:\n%s", out)
	}
	out, err = run(t, clone, "branch")
	if err != nil {
		t.Fatalf("branch in clone: %v\n%s", err, out)
	}
	if !strings.Contains(out, "* topic") {
		t.Fatalf("clone did not check out topic branch:\n%s", out)
	}
	sha, err := run(t, clone, "rev-parse", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v\n%s", err, sha)
	}
	out, err = run(t, clone, "checkout", strings.TrimSpace(sha))
	if err != nil {
		t.Fatalf("checkout detached SHA: %v\n%s", err, out)
	}
	if !strings.Contains(out, "detached") {
		t.Fatalf("checkout SHA should report detached HEAD:\n%s", out)
	}
	out, err = run(t, clone, "checkout", "-B", "topic-reset")
	if err != nil {
		t.Fatalf("checkout -B: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Reset branch") {
		t.Fatalf("checkout -B output:\n%s", out)
	}

	if out, err := run(t, origin, "checkout", "-B", "later"); err != nil {
		t.Fatalf("checkout -B later: %v\n%s", err, out)
	}
	commitFile(t, origin, "later.txt", "later\n", "later seed")
	out, err = run(t, clone, "fetch", "--no-tags", "origin", "later:later")
	if err != nil {
		t.Fatalf("fetch refspec: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Fetched from origin") && !strings.Contains(out, "Already up to date") {
		t.Fatalf("fetch output missing success message:\n%s", out)
	}
	out, err = run(t, clone, "branch")
	if err != nil {
		t.Fatalf("branch after fetch: %v\n%s", err, out)
	}
	if !strings.Contains(out, "later") {
		t.Fatalf("fetched refspec did not create local later branch:\n%s", out)
	}
	out, err = run(t, clone, "checkout", "later")
	if err != nil {
		t.Fatalf("checkout fetched branch: %v\n%s", err, out)
	}
	if body, err := os.ReadFile(filepath.Join(clone, "later.txt")); err != nil || string(body) != "later\n" {
		t.Fatalf("fetched branch worktree mismatch body=%q err=%v", body, err)
	}
}

func TestSplitGitExternal(t *testing.T) {
	cases := []struct {
		name     string
		argv     []string
		external bool
		rest     []string
		wantErr  string
	}{
		{"no flag defaults internal", []string{"status", "--short"}, false, []string{"status", "--short"}, ""},
		{"bare flag selects external", []string{"--external", "log", "--oneline"}, true, []string{"log", "--oneline"}, ""},
		{"flag after verb", []string{"log", "--external", "--oneline"}, true, []string{"log", "--oneline"}, ""},
		{"explicit true", []string{"--external=true", "status"}, true, []string{"status"}, ""},
		{"explicit false stays internal", []string{"--external=false", "status"}, false, []string{"status"}, ""},
		{"last occurrence wins", []string{"--external", "--external=false", "status"}, false, []string{"status"}, ""},
		{"other dashes untouched", []string{"-C", "/tmp", "status"}, false, []string{"-C", "/tmp", "status"}, ""},
		{"bad value fails loudly", []string{"--external=maybe", "status"}, false, nil, "--external"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			external, rest, err := splitGitExternal(tc.argv)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("argv %q: expected error naming %q, got %v", tc.argv, tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("argv %q: unexpected error: %v", tc.argv, err)
			}
			if external != tc.external {
				t.Fatalf("argv %q: external=%v, want %v", tc.argv, external, tc.external)
			}
			if len(rest) != len(tc.rest) {
				t.Fatalf("argv %q: rest=%q, want %q", tc.argv, rest, tc.rest)
			}
			for i := range rest {
				if rest[i] != tc.rest[i] {
					t.Fatalf("argv %q: rest=%q, want %q", tc.argv, rest, tc.rest)
				}
			}
		})
	}
}

func TestGitStatusShortPassthrough(t *testing.T) {
	// `status --short` must reach the engine's porcelain form, not die
	// on flag parsing: the shell shims bare `git status --short` here.
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(prev) }()
	run := func(args ...string) (string, error) {
		root := gitCmd()
		buf := &bytes.Buffer{}
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs(args)
		err := root.Execute()
		return buf.String(), err
	}
	if _, err := run("init"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"--short", "-s", "--porcelain"} {
		out, err := run("status", flag)
		if err != nil {
			t.Fatalf("status %s: %v", flag, err)
		}
		if !strings.Contains(out, "?? new.txt") {
			t.Fatalf("status %s: expected porcelain untracked line, got %q", flag, out)
		}
	}
}

func TestGitCachedDiffAndExternalFallback(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH (needed to check patch application and host fallback)")
	}
	dir := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	run := func(args ...string) (string, error) {
		root := gitCmd()
		buf := &bytes.Buffer{}
		root.SetOut(buf)
		root.SetErr(buf)
		root.SetArgs(args)
		err := root.Execute()
		return buf.String(), err
	}
	if out, err := run("init"); err != nil {
		t.Fatalf("init: %v, %s", err, out)
	}
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run("add", "a.txt"); err != nil || out != "" {
		t.Fatalf("add: err=%v, output=%q", err, out)
	}
	if out, err := run("commit", "-m", "base", "--author-name", "T", "--author-email", "t@e"); err != nil {
		t.Fatalf("commit: %v, %s", err, out)
	}
	if _, err := run("log", "--author=T"); err == nil || !strings.Contains(err.Error(), "--external=true") {
		t.Fatalf("native log unsupported flag: %v", err)
	}
	if out, err := run("log", "--external=true", "--author=T"); err != nil || !strings.Contains(out, "base") {
		t.Fatalf("host log fallback: err=%v, output=%q", err, out)
	}
	if err := os.WriteFile(file, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := run("add", "a.txt"); err != nil || out != "" {
		t.Fatalf("add changed file: err=%v, output=%q", err, out)
	}
	patch, err := run("diff", "--cached", "--binary")
	if err != nil || !strings.Contains(patch, "diff --git a/a.txt b/a.txt") || !strings.Contains(patch, "+new") {
		t.Fatalf("diff --cached --binary: err=%v, patch=%q", err, patch)
	}
	staged, err := run("diff", "--staged")
	if err != nil || staged != patch {
		t.Fatalf("diff --staged: err=%v, patch=%q; want %q", err, staged, patch)
	}
	if out, err := exec.Command("git", "reset", "--mixed", "HEAD").CombinedOutput(); err != nil {
		t.Fatalf("host git reset: %v, %s", err, out)
	}
	apply := exec.Command("git", "apply", "--cached", "-")
	apply.Stdin = strings.NewReader(patch)
	if out, err := apply.CombinedOutput(); err != nil {
		t.Fatalf("host git could not apply staged patch: %v, %s\n%s", err, out, patch)
	}
	if out, err := exec.Command("git", "diff", "--cached").Output(); err != nil || !strings.Contains(string(out), "+new") {
		t.Fatalf("applied patch missing from index: err=%v, patch=%s", err, out)
	}

	// --stat is supported by host git, while the native engine returns
	// ErrUnsupported. The fallback must preserve that argv and its output.
	if _, err := run("diff", "--cached", "--stat"); err == nil || !strings.Contains(err.Error(), "--external=true") || strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("native unsupported flag: %v", err)
	}
	if out, err := run("diff", "--external=true", "--cached", "--stat"); err != nil || !strings.Contains(out, "a.txt") || strings.Contains(out, "unknown flag") {
		t.Fatalf("host fallback: err=%v, output=%q", err, out)
	}
	if out, err := run("--external=true", "diff", "--cached", "--stat"); err != nil || !strings.Contains(out, "a.txt") {
		t.Fatalf("host fallback with door flag first: err=%v, output=%q", err, out)
	}
}
