package agentos

import (
	"os"
	"path/filepath"
	"testing"

	ygit "github.com/qiangli/yoke/git"
)

// seedSrcRepo creates $BASHY_HOME/src/<repo> with the given files committed,
// plus `pristine` and `local` branches pinned at that commit (the shape
// src.Fetch leaves behind). It returns the checkout dir and base SHA.
func seedSrcRepo(t *testing.T, home, repo string, files map[string]string) (dir, base string) {
	t.Helper()
	dir = filepath.Join(home, "src", repo)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ygit.Init(ygit.InitOptions{Path: dir}); err != nil {
		t.Fatalf("init: %v", err)
	}
	writeSrcFiles(t, dir, files)
	if _, err := ygit.Add(ygit.AddOptions{RepoPath: dir, All: true}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := ygit.Commit(ygit.CommitOptions{RepoPath: dir, Message: "seed", AuthorName: "t", AuthorEmail: "t@t"}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	rev, err := ygit.RevParse(ygit.RevParseOptions{RepoPath: dir})
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	base = rev.Hash
	for _, b := range []string{"pristine", "local"} {
		if _, _, err := ygit.Branch(ygit.BranchOptions{RepoPath: dir, Name: b}); err != nil {
			t.Fatalf("branch %s: %v", b, err)
		}
	}
	if _, err := ygit.Checkout(ygit.CheckoutOptions{RepoPath: dir, Branch: "local"}); err != nil {
		t.Fatalf("checkout local: %v", err)
	}
	return dir, base
}

func writeSrcFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// commitOnLocal appends files to the `local` branch of a seeded checkout.
func commitOnLocal(t *testing.T, dir string, files map[string]string, msg string) string {
	t.Helper()
	if _, err := ygit.Checkout(ygit.CheckoutOptions{RepoPath: dir, Branch: "local"}); err != nil {
		t.Fatalf("checkout local: %v", err)
	}
	writeSrcFiles(t, dir, files)
	if _, err := ygit.Add(ygit.AddOptions{RepoPath: dir, All: true}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := ygit.Commit(ygit.CommitOptions{RepoPath: dir, Message: msg, AuthorName: "t", AuthorEmail: "t@t"}); err != nil {
		t.Fatalf("commit: %v", err)
	}
	rev, err := ygit.RevParse(ygit.RevParseOptions{RepoPath: dir})
	if err != nil {
		t.Fatal(err)
	}
	return rev.Hash
}

func TestFetchedSrcReposSkipsBookkeeping(t *testing.T) {
	home := t.TempDir()
	seedSrcRepo(t, home, "yoke", map[string]string{"f": "x"})
	if err := os.MkdirAll(filepath.Join(home, "src", ".prs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "src", "note.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := fetchedSrcRepos(home)
	if len(got) != 1 || got[0] != "yoke" {
		t.Fatalf("repos = %v", got)
	}
}

func TestSrcProvenanceFreshIsPristine(t *testing.T) {
	home := t.TempDir()
	_, base := seedSrcRepo(t, home, "yoke", map[string]string{"pkg/weave/engine.go": "package weave\n"})
	rows := srcProvenance(home)
	if len(rows) != 1 {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].State != "pristine" || rows[0].Base != base || rows[0].Local != base {
		t.Fatalf("row = %+v, base %s", rows[0], base)
	}
}

func TestSrcProvenanceSkillEditIsModified(t *testing.T) {
	home := t.TempDir()
	dir, base := seedSrcRepo(t, home, "yoke", map[string]string{"pkg/weave/engine.go": "package weave\n"})
	local := commitOnLocal(t, dir, map[string]string{"pkg/weave/engine.go": "package weave\n// fix\n"}, "skill fix")
	rows := srcProvenance(home)
	if len(rows) != 1 || rows[0].State != "modified" {
		t.Fatalf("rows = %+v", rows)
	}
	if rows[0].Base != base || rows[0].Local != local {
		t.Fatalf("pins wrong: %+v base %s local %s", rows[0], base, local)
	}
}

func TestSrcProvenanceProtectedEditIsModifiedProtected(t *testing.T) {
	home := t.TempDir()
	dir, _ := seedSrcRepo(t, home, "yoke", map[string]string{"pkg/ask/store.go": "package ask\n"})
	commitOnLocal(t, dir, map[string]string{"pkg/ask/store.go": "package ask\n// guard-rail edit\n"}, "protected edit")
	rows := srcProvenance(home)
	if len(rows) != 1 || rows[0].State != "modified-protected" {
		t.Fatalf("rows = %+v", rows)
	}
}

func TestSrcProvenanceUncheckableIsUnknown(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, "src", "yoke")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ygit.Init(ygit.InitOptions{Path: dir}); err != nil {
		t.Fatal(err)
	}
	// No pristine branch: State cannot be derived, so the repo must never
	// read pristine.
	rows := srcProvenance(home)
	if len(rows) != 1 || rows[0].State != "unknown" {
		t.Fatalf("rows = %+v (must be unknown, never pristine)", rows)
	}
}

func TestAggregateSrcStateWorstWins(t *testing.T) {
	mk := func(state string) srcRepoProvenance { return srcRepoProvenance{Repo: "r", State: state} }
	if got := aggregateSrcState(nil); got != "pristine" {
		t.Fatalf("empty = %q", got)
	}
	if got := aggregateSrcState([]srcRepoProvenance{mk("pristine"), mk("pristine")}); got != "pristine" {
		t.Fatalf("pristine = %q", got)
	}
	if got := aggregateSrcState([]srcRepoProvenance{mk("pristine"), mk("modified")}); got != "modified" {
		t.Fatalf("modified = %q", got)
	}
	if got := aggregateSrcState([]srcRepoProvenance{mk("modified"), mk("modified-protected")}); got != "modified-protected" {
		t.Fatalf("protected = %q", got)
	}
	if got := aggregateSrcState([]srcRepoProvenance{mk("unknown")}); got != "unknown" {
		t.Fatalf("unknown = %q", got)
	}
	if got := aggregateSrcState([]srcRepoProvenance{mk("unknown"), mk("modified")}); got != "modified" {
		t.Fatalf("unknown must not mask modified, got %q", got)
	}
}
