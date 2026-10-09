package agentos

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/external/gitscm"
)

func gitGlobalsRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	git := gitscm.Path()
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(git, args...).CombinedOutput()
		if err != nil {
			t.Skipf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q", repo)
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("-C", repo, "add", "a.txt")
	run("-C", repo, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "-q", "-m", "init")
	if err := os.WriteFile(filepath.Join(repo, "new.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return repo
}

// runGitDoor drives the real front-door dispatch in a subprocess whose cwd
// is NOT the repo, like an agent shell calling `bashy git -C DIR ...`.
func runGitDoor(t *testing.T, cwd string, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(os.Args[0], "-test.run=^TestGitPassthroughHelper$")
	cmd.Dir = cwd
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.Env = append(os.Environ(),
		gitPassthroughHelper+"=1",
		"BASHY_TEST_GIT_VERB=git",
		"BASHY_TEST_GIT_ARGS="+strings.Join(args, "\x1f"),
	)
	code := 0
	if err := cmd.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

func TestGitDashCLogFromOtherCwd(t *testing.T) {
	repo := gitGlobalsRepo(t)
	want, err := exec.Command(gitscm.Path(), "-C", repo, "log", "-1", "--format=%h").Output()
	if err != nil {
		t.Skip(err)
	}
	out, errOut, code := runGitDoor(t, t.TempDir(), "-C", repo, "log", "-1", "--format=%h")
	if code != 0 || strings.TrimSpace(out) != strings.TrimSpace(string(want)) {
		t.Fatalf("git -C log: code=%d out=%q stderr=%q want %q", code, out, errOut, want)
	}
}

func TestGitDashCStatusShortFromOtherCwd(t *testing.T) {
	repo := gitGlobalsRepo(t)
	out, errOut, code := runGitDoor(t, t.TempDir(), "-C", repo, "status", "--short")
	if code != 0 || !strings.Contains(out, "?? new.txt") {
		t.Fatalf("git -C status --short: code=%d out=%q stderr=%q", code, out, errOut)
	}
}

func TestGitDashCRelativeAndRepeated(t *testing.T) {
	repo := gitGlobalsRepo(t)
	parent := filepath.Dir(repo)
	out, errOut, code := runGitDoor(t, t.TempDir(), "-C", parent, "-C", filepath.Base(repo), "--no-pager", "status", "--short")
	if code != 0 || !strings.Contains(out, "?? new.txt") {
		t.Fatalf("repeated -C: code=%d out=%q stderr=%q", code, out, errOut)
	}
}

func TestGitDashCBadDirFailsLikeGit(t *testing.T) {
	_, errOut, code := runGitDoor(t, t.TempDir(), "-C", "/nonexistent-s404", "status")
	if code != 128 || !strings.Contains(errOut, "cannot change to") {
		t.Fatalf("bad -C dir: code=%d stderr=%q", code, errOut)
	}
}

func TestGitDashCExternalStillWorks(t *testing.T) {
	repo := gitGlobalsRepo(t)
	out, errOut, code := runGitDoor(t, t.TempDir(), "--external", "-C", repo, "status", "--short")
	if code != 0 || !strings.Contains(out, "?? new.txt") {
		t.Fatalf("--external -C: code=%d out=%q stderr=%q", code, out, errOut)
	}
}

func TestGitUnappliableGlobalNamesExternal(t *testing.T) {
	repo := gitGlobalsRepo(t)
	_, errOut, code := runGitDoor(t, repo, "-c", "core.quotepath=off", "status", "--short")
	if code == 0 || !strings.Contains(errOut, "--external") {
		t.Fatalf("-c natively: want refusal naming --external, code=%d stderr=%q", code, errOut)
	}
}

func TestSplitGitGlobals(t *testing.T) {
	cases := []struct {
		in      []string
		rest    []string
		dirs    []string
		wantErr string
	}{
		{[]string{"status"}, []string{"status"}, nil, ""},
		{[]string{"-C", "a", "-C", "b", "log", "-1"}, []string{"log", "-1"}, []string{"a", "b"}, ""},
		{[]string{"--no-pager", "-P", "log"}, []string{"log"}, nil, ""},
		{[]string{"log", "-C", "x"}, []string{"log", "-C", "x"}, nil, ""},
		{[]string{"-C"}, nil, nil, "no directory given"},
		{[]string{"-c", "a=b", "log"}, nil, nil, "--external"},
		{[]string{"--git-dir=/x", "log"}, nil, nil, "--external"},
		{[]string{"--work-tree", "/x", "log"}, nil, nil, "--external"},
		{[]string{"--version"}, []string{"--version"}, nil, ""},
	}
	for _, tc := range cases {
		dirs, rest, err := splitGitGlobals(tc.in)
		if tc.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("%q: want error %q, got %v", tc.in, tc.wantErr, err)
			}
			continue
		}
		if err != nil || strings.Join(rest, "\x00") != strings.Join(tc.rest, "\x00") || strings.Join(dirs, "\x00") != strings.Join(tc.dirs, "\x00") {
			t.Errorf("%q: dirs=%q rest=%q err=%v; want dirs=%q rest=%q", tc.in, dirs, rest, err, tc.dirs, tc.rest)
		}
	}
}
