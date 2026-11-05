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
	"strings"
	"testing"
)

func TestShapeCLIMapsCallerAndWish(t *testing.T) {
	oldExec, oldActor := shapeCLIExecute, shapeAuthoredActor
	defer func() { shapeCLIExecute = oldExec; shapeAuthoredActor = oldActor }()
	shapeAuthoredActor = func(explicit string) (string, error) {
		if explicit != "caller" {
			t.Fatalf("actor flag=%q", explicit)
		}
		return "checked-caller", nil
	}
	var got shapeCLIRequest
	shapeCLIExecute = func(_ context.Context, r shapeCLIRequest) (any, error) {
		got = r
		return map[string]string{"id": "candidate"}, nil
	}
	cmd := bashyShapeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"wish", "hello-world", "Make it friendly", "--user", "person", "--as", "caller", "--story", "abc", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.App != "hello-world" || got.User != "person" || got.Actor != "checked-caller" || got.Story != "abc" || got.Request != "Make it friendly" {
		t.Fatalf("mapped request=%+v", got)
	}
	if !json.Valid(out.Bytes()) {
		t.Fatalf("output=%q", out.String())
	}
}

func TestShapeCLIValidationPreventsExecution(t *testing.T) {
	oldExec := shapeCLIExecute
	defer func() { shapeCLIExecute = oldExec }()
	shapeCLIExecute = func(context.Context, shapeCLIRequest) (any, error) {
		t.Fatal("invalid request executed")
		return nil, nil
	}
	for _, args := range [][]string{
		{"wish", "hello-world"},
		{"wish", "hello-world", "hello", "--story", "one", "--todo", "two"},
		{"status", "hello-world", "--remote", "https://example.invalid", "--user", "forged"},
		{"status", "../other", "--user", "person"},
	} {
		cmd := bashyShapeCmd()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}

func TestShapeCLIPrintsFailedVerdictAndFails(t *testing.T) {
	oldExec, oldActor := shapeCLIExecute, shapeAuthoredActor
	defer func() { shapeCLIExecute = oldExec; shapeAuthoredActor = oldActor }()
	shapeAuthoredActor = func(string) (string, error) { return "actor", nil }
	shapeCLIExecute = func(context.Context, shapeCLIRequest) (any, error) {
		return map[string]any{"verdict": map[string]bool{"pass": false}}, errors.New("verification failed")
	}
	cmd := bashyShapeCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"verify", "hello-world", "123", "--user", "person", "--json"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("failed verdict was success")
	}
	if !strings.Contains(out.String(), `"pass": false`) {
		t.Fatalf("verdict missing: %s", out.String())
	}
}

func TestShapeRemoteUsesPairedSessionWithoutPrincipalOverride(t *testing.T) {
	var seen bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = true
		c, err := r.Cookie("bashy_console")
		if err != nil || c.Value != "synthetic-session" {
			t.Errorf("cookie missing")
		}
		if r.URL.Path != "/api/shape/hello-world/candidates" || r.Method != "POST" {
			t.Errorf("request=%s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if _, ok := body["principal"]; ok {
			t.Error("principal override sent")
		}
		if _, ok := body["actor"]; ok {
			t.Error("actor override sent")
		}
		if _, ok := body["todo"]; ok {
			t.Error("empty unsupported tracking key sent")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"candidate","status":"running"}`))
	}))
	defer server.Close()
	cookie := filepath.Join(t.TempDir(), "cookie")
	if err := os.WriteFile(cookie, []byte("synthetic-session"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err := executeShapeRemote(context.Background(), shapeCLIRequest{Verb: "wish", App: "hello-world", Request: "hello", Remote: server.URL, CookieFile: cookie})
	if err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("no request")
	}
}

func TestShapeRemoteDoesNotFollowAuthenticationRedirect(t *testing.T) {
	var redirected bool
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirected = true }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer server.Close()
	cookie := filepath.Join(t.TempDir(), "cookie")
	_ = os.WriteFile(cookie, []byte("synthetic-session"), 0600)
	_, err := executeShapeRemote(context.Background(), shapeCLIRequest{Verb: "status", App: "hello-world", Remote: server.URL, CookieFile: cookie})
	if err == nil || redirected {
		t.Fatalf("err=%v followed=%v", err, redirected)
	}
}

func TestShapeRemoteFailedVerdictPreservedWithoutCookieLeak(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"candidate","verdict":{"pass":false,"detail":"failed check"}}`))
	}))
	defer server.Close()
	cookie := filepath.Join(t.TempDir(), "cookie")
	_ = os.WriteFile(cookie, []byte("synthetic-secret"), 0600)
	result, err := executeShapeRemote(context.Background(), shapeCLIRequest{Verb: "verify", App: "hello-world", ID: "candidate", Remote: server.URL, CookieFile: cookie})
	if err == nil || result == nil {
		t.Fatalf("result=%v err=%v", result, err)
	}
	if strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatal("cookie leaked")
	}
}

func TestShapeRemoteVerificationConflictKeepsCandidate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"id":"candidate","verdict":{"pass":false,"detail":"failed check"}}`))
	}))
	defer server.Close()
	cookie := filepath.Join(t.TempDir(), "cookie")
	_ = os.WriteFile(cookie, []byte("synthetic-secret"), 0600)
	result, err := executeShapeRemote(context.Background(), shapeCLIRequest{Verb: "approve", App: "hello-world", ID: "candidate", Remote: server.URL, CookieFile: cookie})
	if err == nil || result == nil {
		t.Fatalf("result=%v err=%v", result, err)
	}
}

func TestShapeRemoteAuthenticationBodyIsNotRendered(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"cookie":"synthetic-secret"}`))
	}))
	defer server.Close()
	cookie := filepath.Join(t.TempDir(), "cookie")
	if err := os.WriteFile(cookie, []byte("synthetic-secret"), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := executeShapeRemote(context.Background(), shapeCLIRequest{Verb: "status", App: "hello-world", Remote: server.URL, CookieFile: cookie})
	if err == nil || result != nil || strings.Contains(err.Error(), "synthetic-secret") {
		t.Fatalf("unsafe auth failure handling: result=%v err=%v", result, err)
	}
}
