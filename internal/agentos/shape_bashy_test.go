package agentos

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/qiangli/yoke/pkg/shape"
)

// The reserved bashy target resolves without app registration, records a
// defect report with its reproduction/expected/evidence fields, and refuses
// activation.
func TestShapeBashyTargetRecordsDefectReport(t *testing.T) {
	app, err := shapeAppLookup("bashy")
	if err != nil || app.Name != shape.BashyTarget {
		t.Fatalf("lookup bashy = %+v, %v", app, err)
	}
	home := t.TempDir()
	oldOptions, oldRoot := shapeServiceOptions, shapeTrackingRoot
	defer func() { shapeServiceOptions = oldOptions; shapeTrackingRoot = oldRoot }()
	shapeServiceOptions = func(string) shape.Options { return shape.ProductionOptions(home) }
	shapeTrackingRoot = func(string) (string, error) { return filepath.Join(home, "tracking"), nil }
	r := shapeCLIRequest{Verb: "wish", App: "bashy", User: "principal", Actor: "actor",
		Request: "ollama --help fails", Reproduction: "bashy ollama --help", Expected: "usage, exit 0", Evidence: "exit 1"}
	result, err := executeShapeCLI(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	c := result.(shape.Candidate)
	if c.Status != shape.Reported || c.Reproduction != r.Reproduction || c.Expected != r.Expected || c.Evidence != r.Evidence {
		t.Fatalf("record = %+v", c)
	}
	approve := shapeCLIRequest{Verb: "approve", App: "bashy", ID: c.ID, User: "principal", Actor: "actor"}
	if _, err := executeShapeCLI(context.Background(), approve); !errors.Is(err, shape.ErrBashyActivation) {
		t.Fatalf("approve err = %v, want ErrBashyActivation", err)
	}
}

func TestShapeWishDeclaresDefectReportFlags(t *testing.T) {
	wish, _, err := cobraVerb("shape").Find([]string{"wish"})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"reproduction", "expected", "evidence"} {
		if wish.Flags().Lookup(f) == nil {
			t.Errorf("wish missing --%s", f)
		}
	}
}
