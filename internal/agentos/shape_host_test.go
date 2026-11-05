package agentos

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/qiangli/yoke/pkg/fleet"
	"github.com/qiangli/yoke/pkg/shape"
	shapeverify "github.com/qiangli/yoke/pkg/shape/verify"
	"github.com/qiangli/yoke/pkg/src"
)

func TestShapeHostAdapterUsesOneHostHistoryAndArtifact(t *testing.T) {
	home := t.TempDir()
	t.Setenv("BASHY_HOME", home)
	t.Chdir(home)
	source := filepath.Join(home, "src", "example")
	if err := os.MkdirAll(source, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "message.txt"), []byte("Hello, World!"), 0600); err != nil {
		t.Fatal(err)
	}
	repo, err := gogit.PlainInit(source, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = wt.Add("message.txt"); err != nil {
		t.Fatal(err)
	}
	baseline, err := wt.Commit("base", &gogit.CommitOptions{Author: &object.Signature{Name: "Example", Email: "example@example.invalid", When: time.Now()}})
	if err != nil {
		t.Fatal(err)
	}
	if err = repo.Storer.SetReference(plumbing.NewHashReference(plumbing.NewBranchReferenceName("pristine"), baseline)); err != nil {
		t.Fatal(err)
	}
	release := filepath.Join(t.TempDir(), "released")
	if err = os.WriteFile(release, []byte("released binary"), 0700); err != nil {
		t.Fatal(err)
	}
	checker := filepath.Join(t.TempDir(), "checker")
	if err = os.WriteFile(checker, []byte("trusted frozen checker"), 0700); err != nil {
		t.Fatal(err)
	}
	oldLookup, oldOptions, oldRoot := shapeAppLookup, shapeServiceOptions, shapeTrackingRoot
	defer func() { shapeAppLookup = oldLookup; shapeServiceOptions = oldOptions; shapeTrackingRoot = oldRoot }()
	shapeAppLookup = func(string) (shape.App, error) {
		return shape.App{Name: "example", SourceDir: source, ReleasePath: release, ReleaseRevision: baseline.String(), Shaping: &fleet.AppShaping{Kind: "cli", Build: []string{"builder", "{output}"}, Test: []string{checker, "{artifact}"}}}, nil
	}
	shapeServiceOptions = func(string) shape.Options {
		opts := shape.ProductionOptions(home)
		opts.ProgramRunner = shapeverify.ProgramRunnerFunc(func(_ context.Context, r shapeverify.ProgramRun) error {
			if r.Argv[0] == "builder" {
				return os.WriteFile(r.Argv[1], []byte("candidate binary"), 0700)
			}
			return nil
		})
		return opts
	}
	shapeTrackingRoot = func(string) (string, error) { return filepath.Join(home, "tracking"), nil }
	input := t.TempDir()
	if err = os.WriteFile(filepath.Join(input, "message.txt"), []byte("Hello, Shaping!"), 0600); err != nil {
		t.Fatal(err)
	}
	r := shapeCLIRequest{Verb: "wish", App: "example", From: input, User: "alice", Actor: "accountable-actor"}
	result, err := executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	candidate := result.(shape.Candidate)
	if candidate.Principal != "alice" || candidate.ActivationPrincipal != shape.HostPrincipal {
		t.Fatal("requester and activation scope conflated")
	}
	r.Verb = "approve"
	r.ID = candidate.ID
	if _, err = executeShapeCLI(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	r.User = "bob"
	r.Verb = "status"
	result, err = executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	status := result.(shapeHostStatusView)
	if status.Principal != "bob" || status.Host.ActivationPrincipal != shape.HostPrincipal || status.Host.SourceRevision != candidate.Rev || status.Host.Artifact == "" {
		t.Fatalf("host selection not shared: %+v", status)
	}
	r.Verb = "history"
	result, err = executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	history := result.(src.VersionHistory)
	if history.Principal != shape.HostPrincipal || history.Active != candidate.Rev {
		t.Fatalf("per-user history used: %+v", history)
	}
	for _, verb := range []string{"offer", "adopt", "incident"} {
		r.Verb = verb
		if _, err = executeShapeCLI(context.Background(), r); err == nil {
			t.Fatalf("unsupported host %s changed per-user state", verb)
		}
	}
	r.Verb = "undo"
	result, err = executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	restored := result.(shape.HostStatus)
	if restored.SourceRevision != baseline.String() || restored.Slot.Active.Digest != restored.Slot.Release.Digest {
		t.Fatalf("undo did not restore host release: %+v", restored)
	}
	r.Verb, r.ID = "release", candidate.Rev
	result, err = executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	released := result.(src.ReleaseResult)
	tag, err := repo.Tag(released.Version)
	if err != nil {
		t.Fatal(err)
	}
	annotated, err := repo.TagObject(tag.Hash())
	if err != nil {
		t.Fatal(err)
	}
	if annotated.Tagger.Name != r.Actor || !strings.Contains(annotated.Message, "Principal: host") || !strings.Contains(annotated.Message, "Actor: "+r.Actor) {
		t.Fatal("host source release lost accountable actor or namespace")
	}
}
