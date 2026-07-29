package dhnt

import (
	"fmt"
	"sort"
)

type AggregateResult struct {
	Schema   string   `json:"schema"`
	Pipeline string   `json:"pipeline"`
	Result   string   `json:"result"`
	Runs     []string `json:"runs"`
}

// Aggregate validates exact one-for-one evidence coverage for the pipeline
// matrix. It fails closed on any invalid, missing, duplicate, extra,
// mismatched, or non-pass run.
func Aggregate(p Pipeline, runs []Run) (AggregateResult, error) {
	if err := p.Validate(); err != nil {
		return AggregateResult{}, fmt.Errorf("pipeline: %w", err)
	}
	expected := make(map[string]MatrixEntry, len(p.Matrix))
	for _, entry := range p.Matrix {
		expected[matrixKey(entry.Task, entry.Platform)] = entry
	}
	seen := make(map[string]bool, len(runs))
	runIDs := make(map[string]bool, len(runs))
	accepted := make([]string, 0, len(runs))
	for i, run := range runs {
		if err := run.Validate(); err != nil {
			return AggregateResult{}, fmt.Errorf("run[%d]: malformed: %w", i, err)
		}
		if run.Pipeline != p.Pipeline {
			return AggregateResult{}, fmt.Errorf("run[%d]: pipeline mismatch: got %q, want %q", i, run.Pipeline, p.Pipeline)
		}
		if run.Source != p.Source {
			return AggregateResult{}, fmt.Errorf("run[%d]: source mismatch", i)
		}
		platform := Platform{
			Backend: run.Executor.Backend,
			OS:      run.Executor.OS,
			Arch:    run.Executor.Arch,
		}
		key := matrixKey(run.Task, platform)
		entry, ok := expected[key]
		if !ok {
			return AggregateResult{}, fmt.Errorf("run[%d]: extra task/platform evidence for %s/%s/%s/%s",
				i, run.Task, platform.Backend, platform.OS, platform.Arch)
		}
		if seen[key] {
			return AggregateResult{}, fmt.Errorf("run[%d]: duplicate task/platform evidence for %s/%s/%s/%s",
				i, run.Task, platform.Backend, platform.OS, platform.Arch)
		}
		if runIDs[run.Run] {
			return AggregateResult{}, fmt.Errorf("run[%d]: duplicate run identity %q", i, run.Run)
		}
		if run.Result.Class != ResultPass {
			return AggregateResult{}, fmt.Errorf("run[%d]: non-pass result %q", i, run.Result.Class)
		}
		if !equalArtifacts(run.Inputs, entry.Inputs) {
			return AggregateResult{}, fmt.Errorf("run[%d]: input digest mismatch", i)
		}
		if !equalArtifacts(run.Outputs, entry.Outputs) {
			return AggregateResult{}, fmt.Errorf("run[%d]: output digest mismatch", i)
		}
		seen[key] = true
		runIDs[run.Run] = true
		accepted = append(accepted, run.Run)
	}
	for key := range expected {
		if !seen[key] {
			entry := expected[key]
			return AggregateResult{}, fmt.Errorf("missing evidence for %s/%s/%s/%s",
				entry.Task, entry.Platform.Backend, entry.Platform.OS, entry.Platform.Arch)
		}
	}
	sort.Strings(accepted)
	return AggregateResult{
		Schema:   "dhnt.aggregate/v1",
		Pipeline: p.Pipeline,
		Result:   "pass",
		Runs:     accepted,
	}, nil
}

func AggregateJSON(pipelineJSON []byte, runJSON [][]byte) (AggregateResult, error) {
	p, err := DecodePipeline(pipelineJSON)
	if err != nil {
		return AggregateResult{}, fmt.Errorf("pipeline: %w", err)
	}
	runs := make([]Run, len(runJSON))
	for i, data := range runJSON {
		runs[i], err = DecodeRun(data)
		if err != nil {
			return AggregateResult{}, fmt.Errorf("run[%d]: malformed: %w", i, err)
		}
	}
	return Aggregate(p, runs)
}

func MarshalAggregate(result AggregateResult) ([]byte, error) {
	result.Runs = sortedStrings(result.Runs)
	return marshalLine(result)
}

func equalArtifacts(a, b []Artifact) bool {
	a = sortedArtifacts(a)
	b = sortedArtifacts(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
