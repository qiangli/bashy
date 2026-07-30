//go:build !windows

package dhnt

import (
	"context"
	"testing"
)

func TestExecuteTaskSignalIsCanceled(t *testing.T) {
	workspace, spec, argv := runnerFixture(t, "signal")
	run, err := ExecuteTask(context.Background(), workspace, spec, argv, runnerMetadata())
	if err != nil {
		t.Fatal(err)
	}
	if run.Result.Class != ResultCanceled || run.OutputCommit != nil {
		t.Fatalf("unexpected signal evidence: %+v", run)
	}
	assertRunnerEvidence(t, workspace, spec, ResultCanceled, false)
}
