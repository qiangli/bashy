package dhnt

import (
	"os"
	"strings"
	"testing"
)

const (
	sourceDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	inputDigest  = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func fixturePipeline() Pipeline {
	return Pipeline{
		Schema:   PipelineSchema,
		Pipeline: "bashy-release",
		Source: Source{
			Repository: "https://github.com/qiangli/bashy.git",
			Commit:     "abc123",
			SHA256:     sourceDigest,
		},
		Tasks: []Task{{
			ID:               "native-smoke",
			Lane:             LaneNative,
			Needs:            []string{},
			Argv:             []string{"bashy", "--version"},
			WorkingDirectory: ".",
			Environment: []Environment{
				{Name: "Z", Value: "last"},
				{Name: "A", Value: "first"},
			},
		}},
		Matrix: []MatrixEntry{
			matrixFixture("windows", "amd64", strings.Repeat("d", 64)),
			matrixFixture("linux", "amd64", strings.Repeat("c", 64)),
			matrixFixture("darwin", "arm64", inputDigest),
		},
	}
}

func matrixFixture(goos, arch, digest string) MatrixEntry {
	return MatrixEntry{
		Task:     "native-smoke",
		Platform: Platform{Backend: "vk-native", OS: goos, Arch: arch},
		Inputs:   []Artifact{{Name: "candidate", SHA256: digest}},
		Outputs:  []Artifact{{Name: "tested-candidate", SHA256: digest}},
	}
}

func fixtureRun() Run {
	return Run{
		Schema:   RunSchema,
		Pipeline: "bashy-release",
		Task:     "native-smoke",
		Run:      "run-darwin",
		Source: Source{
			Repository: "https://github.com/qiangli/bashy.git",
			Commit:     "abc123",
			SHA256:     sourceDigest,
		},
		Inputs:     []Artifact{{Name: "candidate", SHA256: inputDigest}},
		Executor:   Executor{Node: "dragon-vk-native", Backend: "vk-native", OS: "darwin", Arch: "arm64"},
		Result:     Result{Class: ResultPass, ExitCode: 0},
		Outputs:    []Artifact{{Name: "tested-candidate", SHA256: inputDigest}},
		StartedAt:  "2026-07-29T12:00:00Z",
		FinishedAt: "2026-07-29T12:00:01.123456789Z",
		TraceID:    "0123456789abcdef0123456789abcdef",
	}
}

func TestGoldenEncoding(t *testing.T) {
	tests := []struct {
		name    string
		golden  string
		marshal func() ([]byte, error)
	}{
		{"pipeline", "testdata/pipeline.golden.json", func() ([]byte, error) {
			return MarshalPipeline(fixturePipeline())
		}},
		{"run", "testdata/run.golden.json", func() ([]byte, error) {
			return MarshalRun(fixtureRun())
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.marshal()
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(tt.golden)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Fatalf("encoding differs from golden\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestEveryResultClass(t *testing.T) {
	tests := []struct {
		class ResultClass
		exit  int
	}{
		{ResultPass, 0},
		{ResultTestFail, 1},
		{ResultInfraFail, 70},
		{ResultIncomplete, 0},
		{ResultCanceled, 130},
	}
	for _, tt := range tests {
		t.Run(string(tt.class), func(t *testing.T) {
			run := fixtureRun()
			run.Result = Result{Class: tt.class, ExitCode: tt.exit}
			if err := run.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestStrictValidation(t *testing.T) {
	t.Run("unknown JSON field", func(t *testing.T) {
		data, err := MarshalRun(fixtureRun())
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), `"traceId":`, `"unknown":true,"traceId":`, 1))
		if _, err := DecodeRun(data); err == nil || !strings.Contains(err.Error(), "unknown field") {
			t.Fatalf("got %v, want unknown field error", err)
		}
	})
	t.Run("duplicate JSON field", func(t *testing.T) {
		data, err := MarshalRun(fixtureRun())
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), `"task":"native-smoke"`,
			`"task":"native-smoke","task":"other"`, 1))
		if _, err := DecodeRun(data); err == nil || !strings.Contains(err.Error(), "duplicate field") {
			t.Fatalf("got %v, want duplicate field error", err)
		}
	})
	tests := []struct {
		name string
		edit func(*Run)
		want string
	}{
		{"unknown class", func(r *Run) { r.Result.Class = "flaky" }, "unknown value"},
		{"uppercase digest", func(r *Run) { r.Source.SHA256 = strings.Repeat("A", 64) }, "lowercase 64-hex"},
		{"non UTC time", func(r *Run) { r.StartedAt = "2026-07-29T05:00:00-07:00" }, "must be UTC"},
		{"reversed time", func(r *Run) { r.FinishedAt = "2026-07-29T11:59:59Z" }, "must not be before"},
		{"unknown backend", func(r *Run) { r.Executor.Backend = "argo" }, "unknown value"},
		{"zero trace", func(r *Run) { r.TraceID = strings.Repeat("0", 32) }, "not all zero"},
		{"empty inputs", func(r *Run) { r.Inputs = nil }, "must not be empty"},
		{"pass nonzero", func(r *Run) { r.Result.ExitCode = 1 }, "pass requires"},
		{"test-fail zero", func(r *Run) {
			r.Result = Result{Class: ResultTestFail, ExitCode: 0}
		}, "test-fail requires"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := fixtureRun()
			tt.edit(&run)
			err := run.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestPipelineRequiresDeclaredPlatformMatrix(t *testing.T) {
	pipeline := fixturePipeline()
	pipeline.Matrix = nil
	if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), "matrix") {
		t.Fatalf("got %v, want matrix error", err)
	}
	pipeline = fixturePipeline()
	pipeline.Matrix[0].Platform.Backend = "vk-podman"
	if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), "requires backend vk-native") {
		t.Fatalf("got %v, want native backend error", err)
	}
}
