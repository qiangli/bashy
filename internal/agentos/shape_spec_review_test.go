package agentos

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// spec-review is a local owner verb: it maps APP and ID like approve, and
// has no remote console route (the console's owner flow is separate).
func TestShapeCLIMapsSpecReview(t *testing.T) {
	oldExec, oldActor := shapeCLIExecute, shapeAuthoredActor
	defer func() { shapeCLIExecute = oldExec; shapeAuthoredActor = oldActor }()
	shapeAuthoredActor = func(string) (string, error) { return "owner-agent", nil }
	var got shapeCLIRequest
	shapeCLIExecute = func(_ context.Context, r shapeCLIRequest) (any, error) {
		got = r
		return map[string]string{"id": r.ID}, nil
	}
	cmd := bashyShapeCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetArgs([]string{"spec-review", "summary", "cand1", "--user", "owner", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Verb != "spec-review" || got.App != "summary" || got.ID != "cand1" || got.User != "owner" || got.Actor != "owner-agent" {
		t.Fatalf("mapped request=%+v", got)
	}
	if _, _, _, err := shapeRemoteOperation(shapeCLIRequest{Verb: "spec-review", ID: "cand1"}); err == nil || !strings.Contains(err.Error(), "no supported remote API mapping") {
		t.Fatalf("remote spec-review err=%v", err)
	}
}
