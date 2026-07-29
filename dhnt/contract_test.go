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
			Distribution:     DistributionSingle,
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
		Result:     Result{Class: ResultPass, ExitCode: intPtr(0)},
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
			run.Result = Result{Class: tt.class, ExitCode: intPtr(tt.exit)}
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
	t.Run("missing pass exit code", func(t *testing.T) {
		data, err := MarshalRun(fixtureRun())
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), `,"exitCode":0`, "", 1))
		if _, err := DecodeRun(data); err == nil {
			t.Fatal("pass record with absent result.exitCode was accepted as exit code zero")
		}
	})
	t.Run("invalid UTF-8", func(t *testing.T) {
		data, err := MarshalRun(fixtureRun())
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), `"repository":"https://`,
			"\"repository\":\"https://\xff", 1))
		if _, err := DecodeRun(data); err == nil {
			t.Fatal("invalid UTF-8 was silently accepted and normalized")
		}
	})
	t.Run("unpaired UTF-16 surrogate", func(t *testing.T) {
		data, err := MarshalRun(fixtureRun())
		if err != nil {
			t.Fatal(err)
		}
		data = []byte(strings.Replace(string(data), `"repository":"https://`,
			`"repository":"\ud800https://`, 1))
		if _, err := DecodeRun(data); err == nil {
			t.Fatal("unpaired UTF-16 surrogate was silently accepted and normalized")
		}
	})
	t.Run("field names are case sensitive", func(t *testing.T) {
		tests := []struct {
			name   string
			data   func() ([]byte, error)
			decode func([]byte) error
		}{
			{
				name: "pipeline",
				data: func() ([]byte, error) {
					return MarshalPipeline(fixturePipeline())
				},
				decode: func(data []byte) error {
					_, err := DecodePipeline(data)
					return err
				},
			},
			{
				name: "run",
				data: func() ([]byte, error) {
					return MarshalRun(fixtureRun())
				},
				decode: func(data []byte) error {
					_, err := DecodeRun(data)
					return err
				},
			},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				data, err := tt.data()
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.Replace(string(data), `"schema":`, `"Schema":`, 1))
				if err := tt.decode(data); err == nil || !strings.Contains(err.Error(), "unknown field") {
					t.Fatalf("got %v, want wrong-case field to be rejected as unknown", err)
				}
			})
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
		{"pass nonzero", func(r *Run) { *r.Result.ExitCode = 1 }, "pass requires"},
		{"test-fail zero", func(r *Run) {
			r.Result = Result{Class: ResultTestFail, ExitCode: intPtr(0)}
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

func TestDecodePipelineRejectsMissingRequiredTaskFields(t *testing.T) {
	data, err := MarshalPipeline(fixturePipeline())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(string) string
	}{
		{
			name: "absent needs",
			edit: func(s string) string {
				return strings.Replace(s, `"needs":[],`, "", 1)
			},
		},
		{
			name: "absent distribution",
			edit: func(s string) string {
				return strings.Replace(s, `,"distribution":"single"`, "", 1)
			},
		},
		{
			name: "absent environment",
			edit: func(s string) string {
				return strings.Replace(s,
					`,"environment":[{"name":"A","value":"first"},{"name":"Z","value":"last"}]`,
					"", 1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodePipeline([]byte(tt.edit(string(data)))); err == nil {
				t.Fatal("strict decoder accepted a missing required task field")
			}
		})
	}
}

func TestDecodePipelineRejectsNullTaskFieldValues(t *testing.T) {
	data, err := MarshalPipeline(fixturePipeline())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		edit func(string) string
	}{
		{
			name: "null needs",
			edit: func(s string) string {
				return strings.Replace(s, `"needs":[]`, `"needs":null`, 1)
			},
		},
		{
			name: "null environment value",
			edit: func(s string) string {
				return strings.Replace(s, `"value":"first"`, `"value":null`, 1)
			},
		},
		{
			name: "null argv argument",
			edit: func(s string) string {
				return strings.Replace(s, `"argv":["bashy","--version"]`, `"argv":["bashy",null]`, 1)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := DecodePipeline([]byte(tt.edit(string(data)))); err == nil {
				t.Fatal("strict decoder accepted null for a non-null task field")
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

func TestPipelineRequiresKnownDistribution(t *testing.T) {
	tests := []struct {
		name         string
		distribution Distribution
		want         string
	}{
		{"missing", "", "distribution"},
		{"unknown", "elastic-magic", "unknown value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := fixturePipeline()
			pipeline.Tasks[0].Distribution = tt.distribution
			if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}

	for _, distribution := range []Distribution{
		DistributionSingle,
		DistributionReplicated,
		DistributionTopologyCoupled,
	} {
		t.Run("accept-"+string(distribution), func(t *testing.T) {
			pipeline := fixturePipeline()
			pipeline.Tasks[0].Distribution = distribution
			if err := pipeline.Validate(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestShardableTaskRequiresStableChunkIdentity(t *testing.T) {
	manifest := strings.Repeat("e", 64)
	valid := Chunk{Index: 2, Count: 4, ManifestSHA256: manifest}
	tests := []struct {
		name string
		edit func(*Pipeline)
		want string
	}{
		{"missing", func(*Pipeline) {}, "requires chunk identity"},
		{"zero count", func(p *Pipeline) {
			p.Matrix[0].Chunk = &Chunk{Index: 1, Count: 0, ManifestSHA256: manifest}
		}, "count: must be positive"},
		{"zero index", func(p *Pipeline) {
			p.Matrix[0].Chunk = &Chunk{Index: 0, Count: 4, ManifestSHA256: manifest}
		}, "index: must be between"},
		{"index above count", func(p *Pipeline) {
			p.Matrix[0].Chunk = &Chunk{Index: 5, Count: 4, ManifestSHA256: manifest}
		}, "index: must be between"},
		{"bad manifest digest", func(p *Pipeline) {
			p.Matrix[0].Chunk = &Chunk{Index: 1, Count: 4, ManifestSHA256: "not-a-digest"}
		}, "lowercase 64-hex"},
		{"inconsistent rows", func(p *Pipeline) {
			for i := range p.Matrix {
				chunk := valid
				p.Matrix[i].Chunk = &chunk
			}
			p.Matrix[1].Chunk = &Chunk{Index: 3, Count: 4, ManifestSHA256: manifest}
		}, "inconsistent chunk identity"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := fixturePipeline()
			pipeline.Tasks[0].Distribution = DistributionShardable
			tt.edit(&pipeline)
			if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("got %v, want error containing %q", err, tt.want)
			}
		})
	}

	pipeline := fixturePipeline()
	pipeline.Tasks[0].Distribution = DistributionShardable
	for i := range pipeline.Matrix {
		chunk := valid
		pipeline.Matrix[i].Chunk = &chunk
	}
	if err := pipeline.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestNonShardableTaskRejectsChunkIdentity(t *testing.T) {
	pipeline := fixturePipeline()
	pipeline.Matrix[0].Chunk = &Chunk{
		Index:          1,
		Count:          1,
		ManifestSHA256: strings.Repeat("e", 64),
	}
	if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), "must not declare chunk") {
		t.Fatalf("got %v, want non-shardable chunk rejection", err)
	}
}

func TestShardCanonicalEncodingIsDeterministicAndNonMutating(t *testing.T) {
	pipeline := fixturePipeline()
	pipeline.Tasks[0].Distribution = DistributionShardable
	chunk := Chunk{Index: 2, Count: 4, ManifestSHA256: strings.Repeat("e", 64)}
	for i := range pipeline.Matrix {
		copy := chunk
		pipeline.Matrix[i].Chunk = &copy
	}
	originalFirstOS := pipeline.Matrix[0].Platform.OS

	first, err := MarshalPipeline(pipeline)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MarshalPipeline(pipeline)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("canonical shard encoding changed:\nfirst: %s\nsecond: %s", first, second)
	}
	if pipeline.Matrix[0].Platform.OS != originalFirstOS {
		t.Fatal("canonical encoding mutated the caller's matrix order")
	}
	wantChunk := `"chunk":{"index":2,"count":4,"manifestSha256":"` + strings.Repeat("e", 64) + `"}`
	if !strings.Contains(string(first), wantChunk) {
		t.Fatalf("canonical encoding lacks pinned chunk object: %s", first)
	}
}

func TestPipelineRejectsWorkingDirectoryTraversal(t *testing.T) {
	tests := []struct {
		name string
		wd   string
	}{
		{"backslash traversal", `..\outside`},
		{"forward-slash traversal", "../outside"},
		{"dot component", "./outside"},
		{"parent-dir only", ".."},
		{"mid-path traversal", "foo/../bar"},
		{"mid-path backslash traversal", `foo\..\bar`},
		{"absolute unix", "/etc/passwd"},
		{"windows drive backslash", `C:\windows`},
		{"windows drive forward-slash", "C:/windows"},
		{"windows drive lowercase", `d:\temp`},
		{"unc backslash", `\\server\share`},
		{"unc forward-slash", "//server/share"},
		{"redundant slashes", "foo//bar"},
		{"redundant backslash-slash mix", `foo\/bar`},
		{"platform-dependent separator", `foo\bar`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := fixturePipeline()
			pipeline.Tasks[0].WorkingDirectory = tt.wd
			if err := pipeline.Validate(); err == nil ||
				!strings.Contains(err.Error(), "workingDirectory") {
				t.Fatalf("got %v, want workingDirectory error", err)
			}
		})
	}
}
