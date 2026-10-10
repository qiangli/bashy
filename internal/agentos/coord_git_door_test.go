package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/yoke/pkg/handoff"
	"github.com/qiangli/yoke/pkg/policy/coord"
)

// runGitFrontDoor drives the REAL `bashy git` door in-process and returns the
// status it would exit with. dispatchExit panics a private frontDoorExit while
// the front door is observing, which is what makes the door testable without
// re-execing the binary — and therefore without a git binary, a repo, or a
// network.
func runGitFrontDoor(t *testing.T, args []string) (code int, stderr string) {
	t.Helper()
	frontDoorObserving.Store(true)
	defer frontDoorObserving.Store(false)
	stderr = captureStderr(t, func() {
		defer func() {
			v := recover()
			if v == nil {
				t.Error("dispatchGit returned without exiting")
				return
			}
			c, ok := v.(frontDoorExit)
			if !ok {
				panic(v)
			}
			code = int(c)
		}()
		dispatchGit(args)
	})
	return code, stderr
}

// gitDoorProject makes a project directory the claim ledger can key on
// (detectProjectRoot wants a marker) without making it a git repo: the guard
// must refuse before anything git-shaped runs.
func gitDoorProject(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module gitdoor\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestFrontDoorGitWriteRefusedBeforeTheEngine is the sprint 408 regression: the
// sprint 252 one-door refactor dropped coordGuard from `case "git"`, so every
// `bashy git commit` on the path the shim actually uses went unchecked against
// another agent's project claim.
//
// It is the manager's repro, with the second agent's session modelled by a
// second BASHY_EPISODE and a real (isolated) ledger: no stub stands in for the
// verdict. Every door must refuse with exit 9 BEFORE doing engine work — the
// --external door must not provision a git binary, and the -C door must not
// re-dispatch through awd.
func TestFrontDoorGitWriteRefusedBeforeTheEngine(t *testing.T) {
	setupIsolatedCoord(t)
	base := t.TempDir()
	proj := gitDoorProject(t, filepath.Join(base, "proj"))
	other := gitDoorProject(t, filepath.Join(base, "other"))

	// Agent A claims the project (what `bashy claim` in proj does).
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")
	if err := coord.Enforce(handoff.ProjectRoots(projectRootOf(proj)), "git commit -m holder"); err != nil {
		t.Fatalf("holder could not take the claim: %v", err)
	}

	// Agent B is a different session of a different agent.
	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/other-agent")
	t.Setenv("BASHY_EPISODE", "ep-other")

	var awdCalls [][]string
	oldAwd := gitAwdRun
	gitAwdRun = func(args []string) int {
		awdCalls = append(awdCalls, args)
		return 0
	}
	t.Cleanup(func() { gitAwdRun = oldAwd })

	for _, tc := range []struct {
		name string
		cwd  string
		args []string
	}{
		{"plain", proj, []string{"commit", "-m", "x"}},
		{"external", proj, []string{"--external", "commit", "-m", "x"}},
		{"dash_C_absolute", other, []string{"-C", proj, "commit", "-m", "x"}},
		{"dash_C_relative", other, []string{"-C", "../proj", "commit", "-m", "x"}},
		{"reset_hard", proj, []string{"reset", "--hard", "HEAD~1"}},
		{"push_after_globals", proj, []string{"--no-pager", "push", "origin", "HEAD"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(tc.cwd)
			awdCalls = nil
			code, stderr := runGitFrontDoor(t, tc.args)
			if code != coordExitRefused {
				t.Fatalf("exit = %d, want %d (refused); stderr:\n%s", code, coordExitRefused, stderr)
			}
			if !strings.Contains(stderr, "holder-agent") {
				t.Errorf("refusal does not name the holder:\n%s", stderr)
			}
			if !strings.Contains(stderr, "bashy ping holder-agent") {
				t.Errorf("refusal does not carry the contacts:\n%s", stderr)
			}
			if len(awdCalls) != 0 {
				t.Errorf("-C re-dispatched through awd before the refusal: %v", awdCalls)
			}
		})
	}
}

// TestFrontDoorGitDoesNotRefuseWhatItMustNot keeps the guard narrow: a read
// verb, and the holder's own write, reach the engine. Neither is a claim
// verdict, so neither may exit 9 -- a guard that fires on a read is a guard an
// agent switches off.
func TestFrontDoorGitDoesNotRefuseWhatItMustNot(t *testing.T) {
	setupIsolatedCoord(t)
	proj := gitDoorProject(t, filepath.Join(t.TempDir(), "proj"))
	t.Chdir(proj)

	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")
	if err := coord.Enforce(handoff.ProjectRoots(projectRootOf(proj)), "git commit -m holder"); err != nil {
		t.Fatalf("holder could not take the claim: %v", err)
	}

	// The holder itself, and any read verb from the other session, pass the
	// guard and fail (or not) on the engine's own terms -- never with 9.
	for _, tc := range []struct {
		name    string
		episode string
		args    []string
	}{
		{"holder_write", "ep-holder", []string{"commit", "-m", "x"}},
		{"other_read", "ep-other", []string{"status", "--short"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BASHY_EPISODE", tc.episode)
			code, stderr := runGitFrontDoor(t, tc.args)
			if code == coordExitRefused || strings.Contains(stderr, "refusing") {
				t.Fatalf("wrongly refused: exit=%d stderr:\n%s", code, stderr)
			}
		})
	}
}

// TestFrontDoorGitWriteGuardHonoursClaimOff keeps the operator switch working
// on the door, not just in the middleware.
func TestFrontDoorGitWriteGuardHonoursClaimOff(t *testing.T) {
	setupIsolatedCoord(t)
	proj := gitDoorProject(t, filepath.Join(t.TempDir(), "proj"))
	t.Chdir(proj)

	t.Setenv("BASHY_PRINCIPAL", "dhnt:agent/holder-agent")
	t.Setenv("BASHY_EPISODE", "ep-holder")
	if err := coord.Enforce(handoff.ProjectRoots(projectRootOf(proj)), "git commit -m holder"); err != nil {
		t.Fatalf("holder could not take the claim: %v", err)
	}
	t.Setenv("BASHY_EPISODE", "ep-other")
	t.Setenv("BASHY_CLAIM", "0")

	code, stderr := runGitFrontDoor(t, []string{"commit", "-m", "x"})
	if code == coordExitRefused {
		t.Fatalf("BASHY_CLAIM=0 still refused: stderr:\n%s", stderr)
	}
}

func TestGitArgvDir(t *testing.T) {
	cwd := filepath.Join(string(filepath.Separator), "w")
	abs := filepath.Join(string(filepath.Separator), "b")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"commit"}, cwd},
		{[]string{"-C", "sub", "commit"}, filepath.Join(cwd, "sub")},
		{[]string{"-C", abs, "commit"}, abs},
		{[]string{"-C", "a", "-C", "b", "commit"}, filepath.Join(cwd, "a", "b")},
		{[]string{"--no-pager", "-c", "x=y", "-C", "sub", "push"}, filepath.Join(cwd, "sub")},
		{[]string{"log", "-C", "sub"}, cwd}, // past the verb: not a global
		{[]string{"-C"}, cwd},               // malformed
	} {
		if got := gitArgvDir(cwd, tc.args); got != tc.want {
			t.Errorf("gitArgvDir(%q, %v) = %q, want %q", cwd, tc.args, got, tc.want)
		}
	}
}
