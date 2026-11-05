package agentos

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/qiangli/yoke/pkg/shape"
)

func TestShapeSubmitCommandMapsApprovedRevisionAndStory(t *testing.T) {
	oldExec, oldActor := shapeCLIExecute, shapeAuthoredActor
	defer func() { shapeCLIExecute = oldExec; shapeAuthoredActor = oldActor }()
	shapeAuthoredActor = func(string) (string, error) { return "checked-actor", nil }
	var got shapeCLIRequest
	shapeCLIExecute = func(_ context.Context, r shapeCLIRequest) (any, error) {
		got = r
		return map[string]bool{"complete": true}, nil
	}
	cmd := bashyShapeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"submit", "hello-world", "approved-revision", "--story", "story-id", "--user", "principal", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Verb != "submit" || got.ID != "approved-revision" || got.Story != "story-id" || got.User != "principal" || got.Actor != "checked-actor" {
		t.Fatalf("mapped request=%+v", got)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatal("submission output is not JSON")
	}
}

func TestShapeSubmitPreservesArchivedStoreUnlessStoryExplicit(t *testing.T) {
	oldLookup, oldRoot, oldSubmit := shapeAppLookup, shapeTrackingRoot, shapeSubmit
	defer func() { shapeAppLookup = oldLookup; shapeTrackingRoot = oldRoot; shapeSubmit = oldSubmit }()
	shapeAppLookup = func(string) (shape.App, error) { return shape.App{Name: "hello-world", SourceDir: t.TempDir()}, nil }
	var resolved bool
	shapeTrackingRoot = func(actor string) (string, error) {
		resolved = true
		if actor != "actor" {
			t.Errorf("actor=%q", actor)
		}
		return "invocation-store", nil
	}
	var gotOpts shape.SubmitOptions
	shapeSubmit = func(_ context.Context, svc shape.Service, caller shape.Caller, revision string, opts shape.SubmitOptions) (shape.Submission, error) {
		gotOpts = opts
		if svc.App.Name != "hello-world" || caller.Actor != "actor" || caller.Principal != "principal" || revision != "approved" {
			t.Fatal("submission identity changed")
		}
		return shape.Submission{Complete: true}, nil
	}
	r := shapeCLIRequest{Verb: "submit", App: "hello-world", ID: "approved", Actor: "actor", User: "principal"}
	if _, err := executeShapeCLI(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if resolved || gotOpts.TrackingRoot != "" {
		t.Fatal("implicit submission replaced archived tracking store")
	}
	r.Story = "story-id"
	if _, err := executeShapeCLI(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if !resolved || gotOpts.Story != "story-id" || gotOpts.TrackingRoot != "invocation-store" {
		t.Fatalf("explicit story options=%+v", gotOpts)
	}
	shapeTrackingRoot = func(string) (string, error) { return "", errors.New("tracking unavailable") }
	if _, err := executeShapeCLI(context.Background(), r); err == nil {
		t.Fatal("tracking resolution failure ignored")
	}
}

func TestShapeRemoteSubmitUsesAuthenticatedEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/api/shape/hello-world/submit" {
			t.Errorf("request=%s %s", r.Method, r.URL.Path)
		}
		c, err := r.Cookie("bashy_console")
		if err != nil || c.Value != "synthetic-session" {
			t.Error("authenticated cookie missing")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["rev"] != "approved" || body["story"] != "story-id" || len(body) != 2 {
			t.Errorf("body=%v", body)
		}
		_, _ = w.Write([]byte(`{"complete":true,"source_revision":"approved"}`))
	}))
	defer server.Close()
	cookie := filepath.Join(t.TempDir(), "cookie")
	if err := os.WriteFile(cookie, []byte("synthetic-session"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := executeShapeRemote(context.Background(), shapeCLIRequest{Verb: "submit", App: "hello-world", ID: "approved", Story: "story-id", Remote: server.URL, CookieFile: cookie})
	if err != nil || result == nil {
		t.Fatalf("result=%v err=%v", result, err)
	}
}
