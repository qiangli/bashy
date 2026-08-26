package agentos

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/coreutils/pkg/weave"
)

// THE BUG THIS TEST EXISTS FOR. `git` is a shell FUNCTION in every bashy
// session (the Preamble), so `git commit` reaches the ExecHandler as
// `bashy git commit` — argv[0] is "bashy", not "git". A matcher keyed on
// argv[0] == "git" catches /usr/bin/git and MISSES the only path an agent
// actually uses, which is the same silent no-op the guard exists to remove.
// The first draft of this guard had exactly that bug; reusing coord's isWrite
// is what fixed it.
func TestWeaveGuard_SeesThroughTheShellFunctionShim(t *testing.T) {
	cases := []struct {
		argv []string
		want bool
	}{
		{[]string{"bashy", "git", "commit", "-m", "x"}, true},
		{[]string{"command", "bashy", "git", "commit"}, true},
		{[]string{"/usr/bin/git", "commit"}, true},
		{[]string{"git", "-C", "/somewhere", "merge", "main"}, true},
		{[]string{"git", "rebase", "main"}, true},

		// Reads never violate anything, and warning on the most frequent
		// commands anyone runs is how a guard gets ignored.
		{[]string{"bashy", "git", "status"}, false},
		{[]string{"git", "log", "--oneline"}, false},
		{[]string{"git", "diff"}, false},
		// A plain reset only unstages; only --hard destroys work.
		{[]string{"git", "reset"}, false},
		{[]string{"git", "reset", "--hard"}, true},
		{[]string{"ls"}, false},
		{[]string{"git"}, false},
	}
	for _, c := range cases {
		if got := isWrite(c.argv); got != c.want {
			t.Errorf("isWrite(%v) = %v, want %v", c.argv, got, c.want)
		}
	}
}

func TestWeaveGuardWarning_NamesTheHoldingRuns(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	seedQueue(t, state, repo, `[{"id":35,"state":"working","title":"job carriers"}]`)
	t.Setenv("HOME", fakeHomeWith(t, state))

	line := weaveGuardWarning(filepath.Join(repo, "internal", "agentos"))
	if line == "" {
		t.Fatal("a running weave run holding this repo must produce a warning")
	}
	for _, want := range []string{"#35", "working", "ISOLATION VIOLATED", "BASHY_WEAVE_GUARD=off"} {
		if !strings.Contains(line, want) {
			t.Errorf("warning is missing %q:\n%s", want, line)
		}
	}
}

// Silence when nothing holds the repo is the whole reason the guard is
// tolerable: a hint that fires when nothing is wrong trains people to ignore it.
func TestWeaveGuardWarning_SilentWhenNothingHolds(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	state := t.TempDir()
	seedQueue(t, state, repo, `[{"id":9,"state":"done"}]`)
	t.Setenv("HOME", fakeHomeWith(t, state))

	if line := weaveGuardWarning(repo); line != "" {
		t.Fatalf("no live holder must mean no warning, got: %s", line)
	}
	// Outside any repo there is nothing to hold.
	if line := weaveGuardWarning(t.TempDir()); line != "" {
		t.Fatalf("outside a repo must be silent, got: %s", line)
	}
}

func TestWeaveGuardEnabled_KillSwitch(t *testing.T) {
	for _, v := range []string{"off", "OFF", "0"} {
		t.Setenv("BASHY_WEAVE_GUARD", v)
		if weaveGuardEnabled() {
			t.Errorf("BASHY_WEAVE_GUARD=%q must disable the guard", v)
		}
	}
	t.Setenv("BASHY_WEAVE_GUARD", "strict")
	if !weaveGuardStrict() {
		t.Error("strict must widen the query")
	}
}

// seedQueue writes a queue.json under a fake ~/.bashy/weave tree.
func seedQueue(t *testing.T, stateRoot, repoRoot, itemsJSON string) {
	t.Helper()
	dir := filepath.Join(stateRoot, "repo-test")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := json.Marshal(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"next_id":99,"root":` + string(root) + `,"items":` + itemsJSON + `}`
	if err := os.WriteFile(filepath.Join(dir, "queue.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// fakeHomeWith builds a HOME whose .bashy/weave IS stateRoot's content, since
// HoldersOf resolves the state root from the home dir when no override is given
// — deliberately, so the guard can never look somewhere weave does not write.
func fakeHomeWith(t *testing.T, stateRoot string) string {
	t.Helper()
	home := t.TempDir()
	dst := filepath.Join(home, ".bashy", "weave")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(stateRoot, dst); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	// Sanity: the guard must actually see through this, or the test proves
	// nothing about the real resolution path.
	if _, err := os.Stat(filepath.Join(dst)); err != nil {
		t.Fatalf("fake home not usable: %v", err)
	}
	t.Setenv("USERPROFILE", home)
	_ = weave.HoldersQuery{}
	return home
}
