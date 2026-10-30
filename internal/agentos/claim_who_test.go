package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	gitcfg "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	gitobj "github.com/go-git/go-git/v5/plumbing/object"

	"github.com/qiangli/yoke/pkg/policy/coord"
	"github.com/qiangli/yoke/pkg/principal"
	"github.com/qiangli/yoke/pkg/room"
)

// isolateClaimWho points every store `claim who` reads at throwaway state, so a
// test never consults — or disturbs — the operator's real board.
func isolateClaimWho(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Setenv("BASHY_COORD_DIR", filepath.Join(home, "coord"))
	t.Setenv("BASHY_ROOM_DIR", filepath.Join(home, "room"))
}

func ownersOfPath(t *testing.T, rep claimWhoReport, want string) []claimWhoOwner {
	t.Helper()
	for _, p := range rep.Paths {
		if p.Path == want || p.Abs == want {
			return p.Owners
		}
	}
	t.Fatalf("report has no entry for %q: %#v", want, rep.Paths)
	return nil
}

func TestClaimWhoReportsCoordLedgerClaim(t *testing.T) {
	isolateClaimWho(t)
	repo := t.TempDir()
	target := filepath.Join(repo, "internal", "cli")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	holder := principal.Ref{Kind: principal.KindAgent, Name: "codex-a", Episode: "ep-1", Host: "h"}
	if _, err := coord.Acquire(coord.DefaultDir(), []string{repo}, holder, "sprint 541 story 2960", false); err != nil {
		t.Fatalf("acquire claim: %v", err)
	}

	rep, err := claimWhoReportFor([]string{target}, claimWhoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owners := ownersOfPath(t, rep, target)
	if len(owners) != 1 {
		t.Fatalf("owners = %#v, want the one ledger claim", owners)
	}
	got := owners[0]
	if got.Holder != "codex-a" || got.Source != claimWhoSourceClaim || got.Intent != "sprint 541 story 2960" {
		t.Fatalf("owner = %#v", got)
	}
	if got.Liveness != "live" || !strings.Contains(got.Evidence, repo) {
		t.Fatalf("owner evidence = %#v", got)
	}
	if got.Reach != `bashy mb send codex-a "..."` {
		t.Fatalf("reach line = %q", got.Reach)
	}
}

func TestClaimWhoReportsRoomCardCwdHolder(t *testing.T) {
	isolateClaimWho(t)
	repo := t.TempDir()
	target := filepath.Join(repo, "docs", "TODO.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	joined := time.Now().UTC().Add(-90 * time.Second).Truncate(time.Second)
	card := room.Card{
		ID: "claude-b-invoke", Principal: "operator", Tool: "claude", Model: "opus5",
		Binding: "claude:opus5", Nick: "claude-b", Mode: "interactive",
		Task: "rewriting the scoreboard", PID: os.Getpid(), Cwd: repo,
		Joined: joined.Format(time.RFC3339),
	}
	if err := room.Join(card); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { room.Leave(card.ID) })

	rep, err := claimWhoReportFor([]string{target}, claimWhoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owners := ownersOfPath(t, rep, target)
	if len(owners) != 1 {
		t.Fatalf("owners = %#v, want the live room card", owners)
	}
	got := owners[0]
	if got.Holder != "claude-b" || got.Source != claimWhoSourceRoom || got.Intent != "rewriting the scoreboard" {
		t.Fatalf("owner = %#v", got)
	}
	if got.PID != os.Getpid() || got.Liveness != "live" || got.Age == "" {
		t.Fatalf("owner liveness evidence = %#v", got)
	}
	if !strings.Contains(got.Evidence, repo) {
		t.Fatalf("owner evidence should name the card cwd: %#v", got)
	}
}

func TestClaimWhoReportsDeeperRoomCwdFirst(t *testing.T) {
	isolateClaimWho(t)
	repo := t.TempDir()
	inner := filepath.Join(repo, "internal")
	target := filepath.Join(inner, "cli", "flags.go")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, card := range []room.Card{
		{ID: "wide-invoke", Tool: "codex", Binding: "codex:gpt", Nick: "wide", PID: os.Getpid(), Cwd: repo, Task: "repo sweep"},
		{ID: "deep-invoke", Tool: "claude", Binding: "claude:opus5", Nick: "deep", PID: os.Getpid(), Cwd: inner, Task: "cli work"},
	} {
		if err := room.Join(card); err != nil {
			t.Fatal(err)
		}
		id := card.ID
		t.Cleanup(func() { room.Leave(id) })
	}

	rep, err := claimWhoReportFor([]string{target}, claimWhoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owners := ownersOfPath(t, rep, target)
	if len(owners) != 2 {
		t.Fatalf("owners = %#v, want both room cards", owners)
	}
	if owners[0].Holder != "deep" || owners[1].Holder != "wide" {
		t.Fatalf("deepest cwd must come first: %#v", owners)
	}
}

func TestClaimWhoReportsWeaveHolder(t *testing.T) {
	isolateClaimWho(t)
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(repo, "internal", "agentos")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	weaveRoot := t.TempDir()
	queueDir := filepath.Join(weaveRoot, "bashy-deadbeef")
	if err := os.MkdirAll(queueDir, 0o755); err != nil {
		t.Fatal(err)
	}
	queue := map[string]any{
		"next_id": 2,
		"root":    repo,
		"items": []map[string]any{
			{"id": 360, "title": "claim who", "state": "working", "tool": "claude", "workspace": filepath.Join(weaveRoot, "workspaces", "issue-360")},
			{"id": 361, "title": "merged already", "state": "done", "tool": "codex"},
		},
	}
	body, err := json.Marshal(queue)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(queueDir, "queue.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := claimWhoReportFor([]string{target}, claimWhoOptions{weaveStateRoot: weaveRoot})
	if err != nil {
		t.Fatal(err)
	}
	owners := ownersOfPath(t, rep, target)
	if len(owners) != 1 {
		t.Fatalf("owners = %#v, want only the working weave run", owners)
	}
	got := owners[0]
	if got.Source != claimWhoSourceWeave || got.Holder != "claude" || !strings.Contains(got.Intent, "claim who") {
		t.Fatalf("owner = %#v", got)
	}
	if !strings.Contains(got.Evidence, "#360") {
		t.Fatalf("owner evidence should name the run: %#v", got)
	}
}

func TestClaimWhoReportsUnpushedCommitTrailers(t *testing.T) {
	isolateClaimWho(t)
	repo := t.TempDir()
	r, err := gogit.PlainInit(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	sig := &gitobj.Signature{Name: "worker", Email: "worker@example.com", When: time.Now()}
	commit := func(rel, body, msg string) plumbing.Hash {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Add(rel); err != nil {
			t.Fatal(err)
		}
		h, err := w.Commit(msg, &gogit.CommitOptions{Author: sig, Committer: sig})
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	base := commit("README.md", "base\n", "chore: base")
	head, err := r.Head()
	if err != nil {
		t.Fatal(err)
	}
	branch := head.Name().Short()
	if err := r.Storer.SetReference(plumbing.NewHashReference(
		plumbing.NewRemoteReferenceName("origin", branch), base)); err != nil {
		t.Fatal(err)
	}
	cfg, err := r.Config()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Branches[branch] = &gitcfg.Branch{Name: branch, Remote: "origin", Merge: plumbing.NewBranchReferenceName(branch)}
	if err := r.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	commit("owned.go", "package owned\n", "feat(who): compose owners\n\nSprint: #541\nStory-ID: d440c5f5a85c\nAgent: codex-a\n")

	target := filepath.Join(repo, "owned.go")
	rep, err := claimWhoReportFor([]string{target}, claimWhoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	owners := ownersOfPath(t, rep, target)
	if len(owners) != 1 {
		t.Fatalf("owners = %#v, want the unpushed commit's author trailer", owners)
	}
	got := owners[0]
	if got.Holder != "codex-a" || got.Source != claimWhoSourceCommit {
		t.Fatalf("owner = %#v", got)
	}
	if got.Sprint != "#541" || got.StoryID != "d440c5f5a85c" {
		t.Fatalf("owner trailers = %#v", got)
	}
	if got.Liveness != "unknown" || !strings.Contains(got.Intent, "compose owners") {
		t.Fatalf("owner evidence = %#v", got)
	}

	// A path the unpushed commit never touched has no owner from this source.
	other, err := claimWhoReportFor([]string{filepath.Join(repo, "README.md")}, claimWhoOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if owners := ownersOfPath(t, other, filepath.Join(repo, "README.md")); len(owners) != 0 {
		t.Fatalf("pushed-only path has owners: %#v", owners)
	}
}

func TestClaimWhoReportsNoOwnerHonestly(t *testing.T) {
	isolateClaimWho(t)
	repo := t.TempDir()
	target := filepath.Join(repo, "docs", "plan.md")

	rep, err := claimWhoReportFor([]string{target}, claimWhoOptions{weaveStateRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	owners := ownersOfPath(t, rep, target)
	if len(owners) != 0 {
		t.Fatalf("owners = %#v, want nothing invented", owners)
	}
	if len(rep.Paths) != 1 || !strings.Contains(rep.Paths[0].Advice, "no owner found") {
		t.Fatalf("advice = %#v", rep.Paths)
	}
	if !strings.Contains(rep.Paths[0].Advice, "mb post --topic announce") {
		t.Fatalf("advice must route to the board: %q", rep.Paths[0].Advice)
	}

	var out bytes.Buffer
	if err := renderClaimWho(&out, rep); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "no owner found") {
		t.Fatalf("human output = %q", out.String())
	}

	var jsonOut bytes.Buffer
	if err := renderClaimWhoJSON(&jsonOut, rep); err != nil {
		t.Fatal(err)
	}
	var envelope claimWhoReport
	if err := json.Unmarshal(jsonOut.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.SchemaVersion != claimWhoSchema {
		t.Fatalf("schema = %q, want %q", envelope.SchemaVersion, claimWhoSchema)
	}
	if len(envelope.Paths) != 1 || envelope.Paths[0].Owners == nil {
		t.Fatalf("envelope must carry an empty owners array, not null: %s", jsonOut.String())
	}
}

func TestClaimWhoDefaultsToDirtyPathsOfThisCheckout(t *testing.T) {
	isolateClaimWho(t)
	repo := t.TempDir()
	r, err := gogit.PlainInit(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	w, err := r.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.md"), []byte("one\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Add("tracked.md"); err != nil {
		t.Fatal(err)
	}
	sig := &gitobj.Signature{Name: "worker", Email: "worker@example.com", When: time.Now()}
	if _, err := w.Commit("chore: base", &gogit.CommitOptions{Author: sig, Committer: sig}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "tracked.md"), []byte("two\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := claimWhoReportFor(nil, claimWhoOptions{cwd: repo, weaveStateRoot: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Paths) != 1 || rep.Paths[0].Path != "tracked.md" {
		t.Fatalf("paths = %#v, want the one dirty path", rep.Paths)
	}
	if rep.Paths[0].Abs != filepath.Join(repo, "tracked.md") {
		t.Fatalf("abs = %q", rep.Paths[0].Abs)
	}
}

// A lapsed claim is history, not a holder: on a busy host hundreds of them
// cover the umbrella, and listing each with its full sibling root set buried the
// one live owner. Hidden by default, counted, and shown with --all; evidence
// names only the root that covers the path.
func TestClaimWhoHidesLapsedClaimsAndNamesOnlyTheMatchingRoot(t *testing.T) {
	isolateClaimWho(t)
	repo := t.TempDir()
	sibling := t.TempDir()
	target := filepath.Join(repo, "internal")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	holder := principal.Ref{Kind: principal.KindAgent, Name: "old-seat", Episode: "ep-old", Host: "h"}
	if _, err := coord.Acquire(coord.DefaultDir(), []string{repo, sibling}, holder, "long gone", false); err != nil {
		t.Fatalf("acquire claim: %v", err)
	}
	later := time.Now().Add(48 * time.Hour)

	rep, err := claimWhoReportFor([]string{target}, claimWhoOptions{now: later})
	if err != nil {
		t.Fatal(err)
	}
	if owners := ownersOfPath(t, rep, target); len(owners) != 0 || rep.HiddenStale != 1 {
		t.Fatalf("default view: owners=%#v hidden=%d, want none listed and 1 hidden", owners, rep.HiddenStale)
	}

	rep, err = claimWhoReportFor([]string{target}, claimWhoOptions{now: later, all: true})
	if err != nil {
		t.Fatal(err)
	}
	owners := ownersOfPath(t, rep, target)
	if len(owners) != 1 || owners[0].Liveness == "live" {
		t.Fatalf("--all view: owners=%#v, want the one lapsed claim", owners)
	}
	if !strings.Contains(owners[0].Evidence, repo) || strings.Contains(owners[0].Evidence, sibling) {
		t.Fatalf("evidence = %q, want only the covering root %q", owners[0].Evidence, repo)
	}
}
