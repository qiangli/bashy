package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ygit "github.com/qiangli/yoke/git"
	"github.com/qiangli/yoke/pkg/src"
)

const localTestUnit = "package main\nfunc main() { println(\"edited\") }\n"

// seedWeaveRepo seeds a yoke checkout whose skill-tier weave package carries
// the files tests edit. "sprint" maps to yoke's pkg/weave (no atlas lookup
// involved), so the dispatcher attributes weave-local edits to it.
func seedWeaveRepo(t *testing.T, home string) (dir, base string) {
	t.Helper()
	return seedSrcRepo(t, home, "yoke", map[string]string{
		"pkg/weave/engine.go": "package weave\n",
	})
}

func TestResolveLocalRunPicksEditedBashUnit(t *testing.T) {
	home := t.TempDir()
	dir, _ := seedWeaveRepo(t, home)
	commitOnLocal(t, dir, map[string]string{"pkg/weave/fix.bsh": localTestUnit}, "skill unit fix")
	res, err := resolveLocalRun(home, "sprint")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != localRunSource {
		t.Fatalf("kind = %q (%+v), want source", res.Kind, res)
	}
	if res.State != src.StateModified {
		t.Fatalf("state = %q", res.State)
	}
	body, err := os.ReadFile(res.Unit)
	if err != nil {
		t.Fatalf("unit %q unreadable: %v", res.Unit, err)
	}
	if !strings.Contains(string(body), "edited") {
		t.Fatalf("unit does not carry the edit: %q", body)
	}
}

func TestResolveLocalRunRestoresReleaseAfterRevert(t *testing.T) {
	home := t.TempDir()
	dir, _ := seedWeaveRepo(t, home)
	commitOnLocal(t, dir, map[string]string{"pkg/weave/fix.bsh": localTestUnit}, "skill unit fix")
	if res, _ := resolveLocalRun(home, "sprint"); res.Kind != localRunSource {
		t.Fatalf("pre-revert kind = %q", res.Kind)
	}
	pristine, err := ygit.ResolveCommit(dir, "pristine")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ygit.Checkout(ygit.CheckoutOptions{RepoPath: dir, Branch: "local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := ygit.Reset(ygit.ResetOpts{RepoPath: dir, Mode: ygit.ResetHard, Commit: pristine}); err != nil {
		t.Fatal(err)
	}
	res, err := resolveLocalRun(home, "sprint")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != localRunRelease || res.State != src.StatePristine {
		t.Fatalf("post-revert = %+v, want release/pristine", res)
	}
}

func TestResolveLocalRunCoreTouchFallsBackToRelease(t *testing.T) {
	home := t.TempDir()
	dir, _ := seedWeaveRepo(t, home)
	// pkg/src is core tier in yoke: a running core never executes a local edit.
	commitOnLocal(t, dir, map[string]string{"pkg/src/engine.go": "package src\n"}, "core touch")
	if core, err := localBuildCorePaths(home, "yoke", mustState(t, home)); err != nil || len(core) == 0 {
		t.Fatalf("core = %v, %v", core, err)
	}
	res, err := resolveLocalRun(home, "sprint")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != localRunRelease {
		t.Fatalf("kind = %q, want release", res.Kind)
	}
	if !strings.Contains(res.Notice, "core") {
		t.Fatalf("notice does not name core: %q", res.Notice)
	}
}

func mustState(t *testing.T, home string) src.RepoState {
	t.Helper()
	st, err := src.State(home, "yoke")
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestResolveLocalRunPrefersFreshCachedBuild(t *testing.T) {
	home := t.TempDir()
	dir, _ := seedWeaveRepo(t, home)
	// Go-only change: no interpretable unit.
	commitOnLocal(t, dir, map[string]string{"pkg/weave/engine.go": "package weave\n// fix\n"}, "go fix")
	st := mustState(t, home)
	if st.State != src.StateModified {
		t.Fatalf("state = %q", st.State)
	}
	res, err := resolveLocalRun(home, "sprint")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != localRunRelease || !strings.Contains(res.Notice, "self build --local") {
		t.Fatalf("without a cached build = %+v (want release pointing at self build)", res)
	}
	bin := filepath.Join(home, "src", ".builds", "yoke", shortSHA12(st.Local), "bashy")
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte("fake"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalBuildManifest(home, "yoke", localBuildManifest{
		Repo: "yoke", Base: st.Base, Local: st.Local, State: st.State, Binary: bin, BuiltAt: nowUTC(),
	}); err != nil {
		t.Fatal(err)
	}
	res, err = resolveLocalRun(home, "sprint")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != localRunBinary || res.Binary != bin {
		t.Fatalf("with a fresh manifest = %+v, want binary %s", res, bin)
	}
	// A stale manifest (another local commit) no longer matches: release.
	commitOnLocal(t, dir, map[string]string{"pkg/weave/other.go": "package weave\n"}, "more")
	res, err = resolveLocalRun(home, "sprint")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != localRunRelease {
		t.Fatalf("stale manifest = %+v, want release", res)
	}
}

func TestResolveLocalRunAmbiguousUnitsError(t *testing.T) {
	home := t.TempDir()
	dir, _ := seedWeaveRepo(t, home)
	commitOnLocal(t, dir, map[string]string{
		"pkg/weave/a.bsh": "a\n",
		"pkg/weave/b.bsh": "b\n",
	}, "two units")
	if _, err := resolveLocalRun(home, "sprint"); err == nil {
		t.Fatal("two edited units resolved without error (must not guess)")
	} else if !strings.Contains(err.Error(), "edited Bash# units") {
		t.Fatalf("wrong error: %v", err)
	}
}

func TestResolveLocalRunUnknownCommandIsRelease(t *testing.T) {
	home := t.TempDir()
	res, err := resolveLocalRun(home, "no-such-command-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != localRunRelease {
		t.Fatalf("kind = %q", res.Kind)
	}
}

func TestMaterializeUnitFromTipWhenWorktreeElsewhere(t *testing.T) {
	home := t.TempDir()
	dir, _ := seedWeaveRepo(t, home)
	commitOnLocal(t, dir, map[string]string{"pkg/weave/fix.bsh": localTestUnit}, "skill unit fix")
	local, err := ygit.ResolveCommit(dir, "local")
	if err != nil {
		t.Fatal(err)
	}
	// Park the worktree on pristine: the run must not move it.
	if _, err := ygit.Checkout(ygit.CheckoutOptions{RepoPath: dir, Branch: "pristine"}); err != nil {
		t.Fatal(err)
	}
	path, err := materializeUnit(home, dir, "yoke", local, "pkg/weave/fix.bsh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(path, filepath.Join(".run", "yoke")) {
		t.Fatalf("path = %q (want the run cache, not the worktree)", path)
	}
	body, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(body), "edited") {
		t.Fatalf("materialized body = %q, %v", body, err)
	}
	if cur, _ := ygit.RevParse(ygit.RevParseOptions{RepoPath: dir}); cur.Hash == local {
		t.Fatal("materialization moved the worktree")
	}
}

func TestServiceSrcGuardRefusesModifiedProtected(t *testing.T) {
	if err := serviceSrcGuard([]srcRepoProvenance{{Repo: "yoke", State: "modified"}}); err != nil {
		t.Fatalf("modified runs may start services: %v", err)
	}
	if err := serviceSrcGuard([]srcRepoProvenance{{Repo: "yoke", State: "modified-protected"}}); err == nil {
		t.Fatal("modified-protected build allowed as a service")
	}
	if err := serviceSrcGuard(nil); err != nil {
		t.Fatalf("empty provenance refused: %v", err)
	}
}

func TestLocalBuildManifestRoundTrip(t *testing.T) {
	home := t.TempDir()
	m := localBuildManifest{Repo: "yoke", Base: "b", Local: "l", State: "modified", Binary: "/bin/x", BuiltAt: nowUTC()}
	if err := writeLocalBuildManifest(home, "yoke", m); err != nil {
		t.Fatal(err)
	}
	got, ok := readLocalBuildManifest(home, "yoke", "l")
	if !ok || got.Binary != "/bin/x" || got.Base != "b" {
		t.Fatalf("got = %+v, %v", got, ok)
	}
	if _, ok := readLocalBuildManifest(home, "yoke", "other"); ok {
		t.Fatal("unknown tip read a manifest")
	}
}
