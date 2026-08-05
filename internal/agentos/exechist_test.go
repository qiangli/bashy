// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

package agentos

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"mvdan.cc/sh/v3/interp"

	"github.com/qiangli/coreutils/pkg/execlog"
	"github.com/qiangli/coreutils/pkg/spacegraph"
)

func testRecorder(t *testing.T) (*recorder, string, string) {
	t.Helper()
	logDir, spaceDir := t.TempDir(), t.TempDir()
	return &recorder{
		log:   execlog.Open(logDir),
		space: spacegraph.Open(spaceDir),
	}, logDir, spaceDir
}

// TestExitUnchanged is the invariant every tenant of this seam must hold: a
// command must never fail, or succeed, because of note-taking.
func TestExecHistExitUnchanged(t *testing.T) {
	rec, _, _ := testRecorder(t)
	h := execHistHandler(rec)

	cases := []struct {
		name string
		err  error
	}{
		{"success", nil},
		{"shell exit status", interp.ExitStatus(3)},
		{"dispatched CLI exit", &exec.ExitError{ProcessState: nil}},
		{"unclassifiable", errors.New("boom")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			inner := func(context.Context, []string) error { return tc.err }
			got := h(inner)(context.Background(), []string{"ls", "-l"})
			if !errors.Is(got, tc.err) && got != tc.err {
				t.Errorf("error must pass through unchanged: got %v want %v", got, tc.err)
			}
		})
	}
}

// TestNoPanicWithoutHandlerContext — interp.HandlerCtx panics when the context
// carries no HandlerContext. A recorder must never be the reason a shell dies.
func TestExecHistNoPanicWithoutHandlerContext(t *testing.T) {
	rec, _, _ := testRecorder(t)
	h := execHistHandler(rec)
	inner := func(context.Context, []string) error { return nil }

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recorder panicked without a HandlerContext: %v", r)
		}
	}()
	if err := h(inner)(context.Background(), []string{"ls"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRecordsTheCommand is the basic liveness check: a recorder that records
// nothing is indistinguishable from one that is off, and both read as coverage.
func TestExecHistRecordsTheCommand(t *testing.T) {
	rec, logDir, _ := testRecorder(t)
	h := execHistHandler(rec)
	inner := func(context.Context, []string) error { return nil }

	if err := h(inner)(context.Background(), []string{"go", "build", "./..."}); err != nil {
		t.Fatal(err)
	}
	_ = rec.log.Close()

	recs, cov, err := execlog.Read(logDir, execlog.Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d (coverage %+v)", len(recs), cov)
	}
	if recs[0].Cmd != "go" {
		t.Errorf("wrong command recorded: %q", recs[0].Cmd)
	}
	if recs[0].Template != "go build ./..." {
		t.Errorf("wrong template: %q", recs[0].Template)
	}
	if recs[0].Exit == nil || *recs[0].Exit != 0 {
		t.Errorf("a successful command must record exit 0, got %v", recs[0].Exit)
	}
}

// TestUnobservedExitStaysNull — an error the recorder cannot classify means the
// command may never have run. Recording exit 0 there is the absence-of-evidence
// failure: a success state reached because nothing contradicted it.
func TestExecHistUnobservedExitStaysNull(t *testing.T) {
	rec, logDir, _ := testRecorder(t)
	h := execHistHandler(rec)
	inner := func(context.Context, []string) error { return errors.New("interrupted") }

	_ = h(inner)(context.Background(), []string{"ls"})
	_ = rec.log.Close()

	recs, _, _ := execlog.Read(logDir, execlog.Query{})
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	if recs[0].Exit != nil {
		t.Errorf("unobserved exit must stay null, got %d", *recs[0].Exit)
	}
	if recs[0].Observed {
		t.Error("Observed must be false")
	}
}

// TestDispatchedCLIExitIsSeen guards the regression that made the whole
// learning layer inert: exitStatusOf recognises only interp.ExitStatus, so
// every dispatched CLI — the managed externals and the cobra verbs — was
// silently skipped, which is indistinguishable from having no data.
func TestExecHistDispatchedCLIExitIsSeen(t *testing.T) {
	cmd := exec.Command("sh", "-c", "exit 7")
	runErr := cmd.Run()
	var ee *exec.ExitError
	if !errors.As(runErr, &ee) {
		t.Skipf("could not produce a real *exec.ExitError: %v", runErr)
	}

	rec, logDir, _ := testRecorder(t)
	h := execHistHandler(rec)
	inner := func(context.Context, []string) error { return ee }

	_ = h(inner)(context.Background(), []string{"kubectl", "get", "pods"})
	_ = rec.log.Close()

	recs, _, _ := execlog.Read(logDir, execlog.Query{})
	if len(recs) != 1 {
		t.Fatalf("want 1 record, got %d", len(recs))
	}
	if recs[0].Exit == nil {
		t.Fatal("a dispatched CLI's exit must be observed, not skipped")
	}
	if *recs[0].Exit != 7 {
		t.Errorf("want exit 7, got %d", *recs[0].Exit)
	}
}

// TestTransportFailureTeachesNothing — the graph must not learn from a failure
// it cannot attribute. Three of the five reasons an ssh fails leave the edge
// perfectly correct.
func TestExecHistTransportFailureTeachesNothing(t *testing.T) {
	rec, _, spaceDir := testRecorder(t)
	h := execHistHandler(rec)
	inner := func(context.Context, []string) error { return interp.ExitStatus(255) }

	_ = h(inner)(context.Background(), []string{"ssh", "-p", "2222", "user@remote.host"})

	edges, err := spacegraph.Open(spaceDir).Edges(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range edges {
		if e.Rel == spacegraph.RelReached {
			t.Errorf("a transport failure must not assert reachability: %+v", e)
		}
	}
}

// TestSuccessTeachesTheGraph is the payoff: one successful ssh names the host,
// the endpoint, the account, and the relation between them.
func TestExecHistSuccessTeachesTheGraph(t *testing.T) {
	rec, _, spaceDir := testRecorder(t)
	h := execHistHandler(rec)
	inner := func(context.Context, []string) error { return nil }

	if err := h(inner)(context.Background(),
		[]string{"ssh", "-p", "2222", "user@remote.host"}); err != nil {
		t.Fatal(err)
	}

	edges, err := spacegraph.Open(spaceDir).Edges(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, e := range edges {
		if e.Rel == spacegraph.RelReached && e.Dst == "endpoint:remote.host:2222" {
			found = true
			if e.Via != "account:user@remote.host" {
				t.Errorf("edge must record the account, got %q", e.Via)
			}
		}
	}
	if !found {
		t.Errorf("no reached edge learned; got %+v", edges)
	}
}

func TestSubpathOfIsCoarse(t *testing.T) {
	cases := []struct{ root, dir, want string }{
		{"/w/repo", "/w/repo", "."},
		{"/w/repo", "/w/repo/internal", "internal"},
		{"/w/repo", "/w/repo/internal/agentos", "internal/agentos"},
		{"/w/repo", "/w/repo/internal/agentos/testdata/a/b", "internal/agentos"},
		{"/w/repo", "/elsewhere", ""},
		{"", "/w/repo", ""},
	}
	for _, tc := range cases {
		if got := subpathOf(tc.root, tc.dir); got != tc.want {
			t.Errorf("subpathOf(%q,%q) = %q want %q", tc.root, tc.dir, got, tc.want)
		}
	}
}

// TestBenignExitIsNotAFailure — grep finding nothing is a negative ANSWER, not
// an error. In a real corpus 40% of every exit-1 is exactly this, so counting
// them would promote a pitfall claiming grep is unreliable on this machine.
func TestExecHistBenignExitIsNotAFailure(t *testing.T) {
	rec, logDir, _ := testRecorder(t)
	h := execHistHandler(rec)
	inner := func(context.Context, []string) error { return interp.ExitStatus(1) }

	_ = h(inner)(context.Background(), []string{"grep", "needle", "haystack"})
	_ = h(inner)(context.Background(), []string{"go", "test", "./..."})
	_ = rec.log.Close()

	recs, _, _ := execlog.Read(logDir, execlog.Query{})
	if len(recs) != 2 {
		t.Fatalf("want 2 records, got %d", len(recs))
	}
	byCmd := map[string]execlog.Record{}
	for _, r := range recs {
		byCmd[r.Cmd] = r
	}
	if !byCmd["grep"].Benign {
		t.Error("grep exit 1 must be marked benign")
	}
	if byCmd["go"].Benign {
		t.Error("go test exit 1 is a real failure, not benign")
	}
}

// TestDiagnosisIsHandedOver — the advisor's dimension must reach the record.
// Without it a pitfall says "this exits 1 here", which is noise; with it the
// pitfall says WHY, which is actionable.
func TestExecHistDiagnosisIsHandedOver(t *testing.T) {
	ctx, slot := withDiagSlot(context.Background())
	if diagFrom(ctx) != slot {
		t.Fatal("slot must be retrievable from the context")
	}
	slot.recordDiagnosis(&hint{dimension: "network", retryable: false})
	if slot.dimension != "network" || slot.retryable {
		t.Errorf("diagnosis not carried: %+v", slot)
	}
	// A second, vaguer pass must not overwrite a specific one.
	slot.recordDiagnosis(&hint{dimension: "state", retryable: true})
	if slot.dimension != "network" {
		t.Errorf("first diagnosis must win, got %q", slot.dimension)
	}
}

// TestDiagnosisSlotAbsentIsSafe — when the recorder is off there is no slot,
// and the advisor's hand-off must be a nil check rather than a panic.
func TestExecHistDiagnosisSlotAbsentIsSafe(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("hand-off panicked with no slot: %v", r)
		}
	}()
	diagFrom(context.Background()).recordDiagnosis(&hint{dimension: "disk"})
}

// TestClassifiedFoundNothingIsNotUnclassified — a nil hint means the advisor
// looked and found nothing, which is a different claim from never looking.
func TestExecHistClassifiedFoundNothing(t *testing.T) {
	_, slot := withDiagSlot(context.Background())
	slot.recordDiagnosis(nil)
	if !slot.filled {
		t.Error("a nil diagnosis must still mark the slot as classified")
	}
	if slot.dimension != "" {
		t.Errorf("no dimension should be invented, got %q", slot.dimension)
	}
}
