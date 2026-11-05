package agentos

import (
	"context"
	"net/http/httptest"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/qiangli/yoke/pkg/shape"
	"github.com/qiangli/yoke/pkg/webconsole"
	"github.com/qiangli/yoke/pkg/websession"
)

func TestShapeLocalImportCannotChangeProtectedParts(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "src", "example")
	if err := os.MkdirAll(filepath.Join(source, "public"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "public", "index.html"), []byte("Hello, World!"), 0600); err != nil {
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
	if _, err := wt.Add("public/index.html"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("base", &gogit.CommitOptions{Author: &object.Signature{Name: "Example", Email: "example@example.invalid", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	input := t.TempDir()
	if err := os.MkdirAll(filepath.Join(input, "public"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, "public", "index.html"), []byte("Unauthorized change"), 0600); err != nil {
		t.Fatal(err)
	}
	oldLookup, oldOptions, oldRoot := shapeAppLookup, shapeServiceOptions, shapeTrackingRoot
	defer func() { shapeAppLookup = oldLookup; shapeServiceOptions = oldOptions; shapeTrackingRoot = oldRoot }()
	shapeAppLookup = func(string) (shape.App, error) {
		return shape.App{Name: "example", SourceDir: source, Protected: []string{"public/index.html"}}, nil
	}
	shapeServiceOptions = func(string) shape.Options { return shape.ProductionOptions(home) }
	shapeTrackingRoot = func(string) (string, error) { return filepath.Join(home, "tracking"), nil }
	r := shapeCLIRequest{Verb: "wish", App: "example", From: input, User: "principal", Actor: "actor"}
	result, err := executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := result.(shape.Candidate)
	if !ok || c.ID == "" {
		t.Fatalf("failed candidate missing: %#v", result)
	}
	verified, err := executeShapeCLI(context.Background(), shapeCLIRequest{Verb: "verify", App: "example", ID: c.ID, User: "principal", Actor: "actor"})
	if err == nil {
		t.Fatal("protected candidate verified")
	}
	v := verified.(shape.Candidate)
	if v.Verdict == nil || v.Verdict.Pass || !strings.Contains(strings.ToLower(v.Verdict.Detail), "protected") {
		t.Fatalf("candidate verdict=%+v", v)
	}
	if _, err = executeShapeCLI(context.Background(), shapeCLIRequest{Verb: "approve", App: "example", ID: c.ID, User: "principal", Actor: "actor"}); err == nil {
		t.Fatal("failed candidate activated")
	}
	raw, err := os.ReadFile(filepath.Join(source, "public", "index.html"))
	if err != nil || string(raw) != "Hello, World!" {
		t.Fatalf("source changed: %q %v", raw, err)
	}
}

func TestShapeConsoleLookupUsesEmbeddedSeedAndProtection(t *testing.T) {
	t.Setenv("BASHY_HOME", t.TempDir())
	app, err := shapeAppLookup("console")
	if err != nil {
		t.Fatal(err)
	}
	if app.Name != "console" || app.Seed == nil || len(app.Protected) == 0 {
		t.Fatalf("console metadata incomplete: %+v", app)
	}
	if err := app.Seed(); err != nil {
		t.Fatal(err)
	}
	if _, err := gogit.PlainOpen(app.SourceDir); err != nil {
		t.Fatalf("console source not seeded: %v", err)
	}
	for _, verb := range []string{"suggest", "incident"} {
		result, err := executeShapeCLI(context.Background(), shapeCLIRequest{Verb: verb, App: "console", User: "synthetic-principal", Actor: "synthetic-actor", Page: "/"})
		if err != nil || result == nil {
			t.Fatalf("%s direct operation failed: %v", verb, err)
		}
	}
	for _, part := range []string{"shape", "term", "accounts"} {
		found := false
		for _, p := range app.Protected {
			if strings.Contains(p, part) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s missing from protected console paths", part)
		}
	}
}

func TestShapeCommandSchemaIncludesLifecycle(t *testing.T) {
	cmd := cobraVerb("shape")
	if cmd == nil {
		t.Fatal("shape missing from schema")
	}
	for _, verb := range []string{"status", "wish", "show", "diff", "verify", "preview", "approve", "reject", "suggest", "incident", "history", "undo", "restore", "release", "offer", "adopt"} {
		child, _, err := cmd.Find([]string{verb})
		if err != nil || child == cmd {
			t.Errorf("%s missing", verb)
		}
	}
}

func TestShapeLocalCandidateSurvivesSeparateAdapterCalls(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "src", "hello-world")
	if err := os.MkdirAll(filepath.Join(source, "public"), 0700); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(source, "public", "index.html"), []byte("Hello, World!"), 0600)
	repo, err := gogit.PlainInit(source, false)
	if err != nil {
		t.Fatal(err)
	}
	wt, _ := repo.Worktree()
	_, _ = wt.Add("public/index.html")
	if _, err := wt.Commit("base", &gogit.CommitOptions{Author: &object.Signature{Name: "Example", Email: "example@example.invalid", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	input := t.TempDir()
	_ = os.MkdirAll(filepath.Join(input, "public"), 0700)
	_ = os.WriteFile(filepath.Join(input, "public", "index.html"), []byte("Hello, Shaping!"), 0600)
	oldLookup, oldOptions, oldRoot := shapeAppLookup, shapeServiceOptions, shapeTrackingRoot
	defer func() { shapeAppLookup = oldLookup; shapeServiceOptions = oldOptions; shapeTrackingRoot = oldRoot }()
	shapeAppLookup = func(string) (shape.App, error) { return shape.App{Name: "hello-world", SourceDir: source}, nil }
	shapeServiceOptions = func(string) shape.Options { return shape.ProductionOptions(home) }
	shapeTrackingRoot = func(string) (string, error) { return filepath.Join(home, "tracking"), nil }
	base := shapeCLIRequest{App: "hello-world", User: "principal", Actor: "actor"}
	r := base
	r.Verb = "wish"
	r.From = input
	result, err := executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	candidate := result.(shape.Candidate)
	if candidate.Status != shape.Ready || candidate.TrackingItem == "" || candidate.Rev == "" {
		t.Fatalf("incomplete candidate: %+v", candidate)
	}
	r = base
	r.Verb = "show"
	r.ID = candidate.ID
	result, err = executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if result.(shape.Candidate).Rev != candidate.Rev {
		t.Fatal("new adapter call lost candidate revision")
	}
	r.Verb = "diff"
	result, err = executeShapeCLI(context.Background(), r)
	if err != nil || !strings.Contains(result.(string), "Hello, Shaping!") {
		t.Fatalf("diff=%v err=%v", result, err)
	}
	r.User = "other"
	r.Verb = "show"
	if _, err = executeShapeCLI(context.Background(), r); err == nil {
		t.Fatal("other principal read candidate")
	}
	r.User = "principal"
	r.Verb = "reject"
	if _, err = executeShapeCLI(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	r.Verb = "show"
	if _, err = executeShapeCLI(context.Background(), r); err == nil {
		t.Fatal("rejected candidate still available")
	}
	raw, _ := os.ReadFile(filepath.Join(source, "public", "index.html"))
	if string(raw) != "Hello, World!" {
		t.Fatalf("active source changed: %q", raw)
	}
	shapeServiceOptions = func(string) shape.Options {
		opts := shape.ProductionOptions(home)
		opts.Delegate = shapeCLITestDelegate{}
		return opts
	}
	r = base
	r.Verb = "wish"
	r.Request = "Change the greeting"
	result, err = executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	generated := result.(shape.Candidate)
	if generated.Status != shape.Ready || generated.Actor != "actor" || generated.TrackingRoot != filepath.Join(home, "tracking") {
		t.Fatalf("local wish did not finish with caller/tracking identity: %+v", generated)
	}
}

func TestShapeRemoteDiffUsesRealConsoleHandler(t *testing.T) {
	home := t.TempDir()
	source := filepath.Join(home, "src", "hello-world")
	if err := os.MkdirAll(filepath.Join(source, "public"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "public", "index.html"), []byte("Hello, World!"), 0600); err != nil {
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
	if _, err := wt.Add("public/index.html"); err != nil {
		t.Fatal(err)
	}
	if _, err := wt.Commit("base", &gogit.CommitOptions{Author: &object.Signature{Name: "Example", Email: "example@example.invalid", When: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	current, err := user.Current()
	if err != nil {
		t.Fatal("cannot resolve current account for operator-session fixture")
	}
	sessions := websession.NewStore(time.Hour, []byte("synthetic-test-key-32-bytes-long!!"))
	cookie, err := sessions.Mint(current.Username)
	if err != nil {
		t.Fatal(err)
	}
	handler, closeHandler, err := webconsole.Handler(webconsole.Options{RequireLogin: true, Sessions: sessions, Panels: []webconsole.Panel{{Name: "hello-world", Label: "Hello World", Path: "/hello-world/", ShapedSource: source, Auth: webconsole.AuthSystem, Available: true}}, Shape: webconsole.ShapeOptions{Home: home}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeHandler()
	server := httptest.NewServer(handler)
	defer server.Close()
	cookieFile := filepath.Join(home, "cookie")
	if err := os.WriteFile(cookieFile, []byte(cookie), 0600); err != nil {
		t.Fatal(err)
	}
	r := shapeCLIRequest{Verb: "status", App: "hello-world", Remote: server.URL, CookieFile: cookieFile}
	result, err := executeShapeRemote(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	principal, ok := result.(map[string]any)["principal"].(string)
	if !ok || principal == "" {
		t.Fatal("missing authenticated principal")
	}
	input := t.TempDir()
	if err := os.MkdirAll(filepath.Join(input, "public"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(input, "public", "index.html"), []byte("Hello, Shaping!"), 0600); err != nil {
		t.Fatal(err)
	}
	svc := shape.Service{App: shape.App{Name: "hello-world", SourceDir: source}, Options: shape.Options{Home: home}}
	c, err := svc.Import(context.Background(), shape.Caller{Principal: principal, Actor: "synthetic", RepresentedUser: principal}, shape.Wish{Request: "Change greeting"}, input)
	if err != nil {
		t.Fatal(err)
	}
	r.Verb = "diff"
	r.ID = c.ID
	result, err = executeShapeRemote(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	patch, ok := result.(string)
	if !ok || !strings.Contains(patch, "Hello, Shaping!") || strings.Contains(patch, "<pre") {
		t.Fatalf("raw patch missing or HTML returned: %T", result)
	}
}

// shapeCLITestDelegate exercises the synchronous CLI generation boundary without
// a model/provider dependency. Real tool invocation belongs to H7.
type shapeCLITestDelegate struct{}

func (shapeCLITestDelegate) Delegate(ctx context.Context, req shape.DelegateRequest) (shape.DelegateResult, error) {
	if err := ctx.Err(); err != nil {
		return shape.DelegateResult{}, err
	}
	err := os.WriteFile(filepath.Join(req.Dir, "public", "index.html"), []byte("Hello, Shaping!"), 0600)
	return shape.DelegateResult{Agent: "synthetic-agent", Output: "COVERED:\n- Changed greeting"}, err
}
