package dhnt

import (
	"strings"
	"testing"
)

func completeRuns() []Run {
	base := fixtureRun()
	linux := base
	linux.Run = "run-linux"
	linux.Inputs = []Artifact{{Name: "candidate", SHA256: strings.Repeat("c", 64)}}
	linux.Outputs = []Artifact{{Name: "tested-candidate", SHA256: strings.Repeat("c", 64)}}
	linux.Executor = Executor{Node: "lern-vk-native", Backend: "vk-native", OS: "linux", Arch: "amd64"}
	windows := base
	windows.Run = "run-windows"
	windows.Inputs = []Artifact{{Name: "candidate", SHA256: strings.Repeat("d", 64)}}
	windows.Outputs = []Artifact{{Name: "tested-candidate", SHA256: strings.Repeat("d", 64)}}
	windows.Executor = Executor{Node: "puppy-vk-native", Backend: "vk-native", OS: "windows", Arch: "amd64"}
	return []Run{windows, base, linux}
}

func TestAggregatePassesExactMatrix(t *testing.T) {
	got, err := Aggregate(fixturePipeline(), completeRuns())
	if err != nil {
		t.Fatal(err)
	}
	if got.Result != "pass" || strings.Join(got.Runs, ",") != "run-darwin,run-linux,run-windows" {
		t.Fatalf("unexpected aggregate: %+v", got)
	}
}

func TestAggregateRejectsEveryNonPassClass(t *testing.T) {
	for _, class := range []ResultClass{
		ResultTestFail, ResultInfraFail, ResultIncomplete, ResultCanceled,
	} {
		t.Run(string(class), func(t *testing.T) {
			runs := completeRuns()
			runs[0].Result = Result{Class: class, ExitCode: 1}
			if _, err := Aggregate(fixturePipeline(), runs); err == nil ||
				!strings.Contains(err.Error(), "non-pass") {
				t.Fatalf("got %v, want non-pass rejection", err)
			}
		})
	}
}

func TestAggregateAdversarialMatrix(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Pipeline, *[]Run)
		want string
	}{
		{"missing", func(_ *Pipeline, runs *[]Run) { *runs = (*runs)[:2] }, "missing evidence"},
		{"duplicate platform", func(_ *Pipeline, runs *[]Run) { *runs = append(*runs, (*runs)[0]) }, "duplicate task/platform"},
		{"duplicate run identity", func(p *Pipeline, runs *[]Run) {
			(*runs)[0].Run = (*runs)[1].Run
			p.Matrix[0].Platform = Platform{Backend: "vk-native", OS: "windows", Arch: "arm64"}
			(*runs)[0].Executor.Arch = "arm64"
		}, "duplicate run identity"},
		{"extra platform", func(_ *Pipeline, runs *[]Run) { (*runs)[0].Executor.Arch = "arm64" }, "extra task/platform"},
		{"pipeline mismatch", func(_ *Pipeline, runs *[]Run) { (*runs)[0].Pipeline = "other" }, "pipeline mismatch"},
		{"task mismatch", func(_ *Pipeline, runs *[]Run) { (*runs)[0].Task = "other" }, "extra task/platform"},
		{"source digest mismatch", func(_ *Pipeline, runs *[]Run) {
			(*runs)[0].Source.SHA256 = strings.Repeat("e", 64)
		}, "source mismatch"},
		{"input digest mismatch", func(_ *Pipeline, runs *[]Run) {
			(*runs)[0].Inputs[0].SHA256 = strings.Repeat("e", 64)
		}, "input digest mismatch"},
		{"output digest mismatch", func(_ *Pipeline, runs *[]Run) {
			(*runs)[0].Outputs[0].SHA256 = strings.Repeat("e", 64)
		}, "output digest mismatch"},
		{"backend mismatch", func(_ *Pipeline, runs *[]Run) {
			(*runs)[0].Executor.Backend = "k3s"
		}, "extra task/platform"},
		{"os mismatch", func(_ *Pipeline, runs *[]Run) {
			(*runs)[0].Executor.OS = "linux"
			(*runs)[0].Inputs[0].SHA256 = strings.Repeat("c", 64)
			(*runs)[0].Outputs[0].SHA256 = strings.Repeat("c", 64)
		}, "duplicate task/platform"},
		{"non pass", func(_ *Pipeline, runs *[]Run) {
			(*runs)[0].Result = Result{Class: ResultTestFail, ExitCode: 1}
		}, "non-pass"},
		{"malformed record", func(_ *Pipeline, runs *[]Run) {
			(*runs)[0].TraceID = "bad"
		}, "malformed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := fixturePipeline()
			runs := completeRuns()
			tt.edit(&pipeline, &runs)
			_, err := Aggregate(pipeline, runs)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestAggregateJSONRejectsUnknownFields(t *testing.T) {
	pipelineJSON, err := MarshalPipeline(fixturePipeline())
	if err != nil {
		t.Fatal(err)
	}
	runJSON, err := MarshalRun(fixtureRun())
	if err != nil {
		t.Fatal(err)
	}
	runJSON = []byte(strings.Replace(string(runJSON), `"traceId":`, `"extra":1,"traceId":`, 1))
	if _, err := AggregateJSON(pipelineJSON, [][]byte{runJSON}); err == nil ||
		!strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("got %v, want unknown field error", err)
	}
}
