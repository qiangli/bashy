package agentos

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestShapePreviewCommandPreservesExplicitArguments(t *testing.T) {
	oldExec, oldActor := shapeCLIExecute, shapeAuthoredActor
	defer func() { shapeCLIExecute = oldExec; shapeAuthoredActor = oldActor }()
	shapeAuthoredActor = func(string) (string, error) { return "actor", nil }
	var got shapeCLIRequest
	shapeCLIExecute = func(_ context.Context, r shapeCLIRequest) (any, error) { got = r; return map[string]any{}, nil }
	cmd := bashyShapeCmd()
	cmd.SetOut(io.Discard)
	cmd.SetArgs([]string{"preview", "bashy", "candidate", "--user", "principal", "--", "shape", "--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.PreviewArgs, " ") != "shape --help" {
		t.Fatalf("arguments=%v", got.PreviewArgs)
	}
}
