package dhnt

import (
	"os"
	"strings"
	"testing"
)

func fixturePipelineV2() Pipeline {
	pipeline := fixturePipeline()
	pipeline.Schema = PipelineSchemaV2
	for i := range pipeline.Matrix {
		pipeline.Matrix[i].Inputs = v2FileArtifacts(pipeline.Matrix[i].Inputs)
		pipeline.Matrix[i].Outputs = v2FileArtifacts(pipeline.Matrix[i].Outputs)
	}
	return pipeline
}

func fixtureRunV2() Run {
	run := fixtureRun()
	run.Schema = RunSchemaV2
	run.Inputs = v2FileArtifacts(run.Inputs)
	run.Outputs = []Artifact{
		{Name: "report", Kind: ArtifactFile, DigestAlgorithm: DigestSHA256FileV1, SHA256: strings.Repeat("e", 64)},
		{Name: "tested-candidate", Kind: ArtifactFile, DigestAlgorithm: DigestSHA256FileV1, SHA256: inputDigest},
	}
	commit, err := NewOutputCommit(run.Outputs)
	if err != nil {
		panic(err)
	}
	run.OutputCommit = &commit
	return run
}

func v2FileArtifacts(artifacts []Artifact) []Artifact {
	result := append([]Artifact(nil), artifacts...)
	for i := range result {
		result[i].Kind = ArtifactFile
		result[i].DigestAlgorithm = DigestSHA256FileV1
	}
	return result
}

func TestV2GoldenEncoding(t *testing.T) {
	tests := []struct {
		name    string
		golden  string
		marshal func() ([]byte, error)
	}{
		{"pipeline", "testdata/pipeline-v2.golden.json", func() ([]byte, error) {
			return MarshalPipeline(fixturePipelineV2())
		}},
		{"run", "testdata/run-v2.golden.json", func() ([]byte, error) {
			return MarshalRun(fixtureRunV2())
		}},
		{"output commit", "testdata/output-commit-v1.golden.json", func() ([]byte, error) {
			return MarshalOutputCommitManifest(fixtureRunV2().Outputs)
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

func TestV2GoldenDecoding(t *testing.T) {
	pipelineJSON, err := os.ReadFile("testdata/pipeline-v2.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := DecodePipeline(pipelineJSON)
	if err != nil {
		t.Fatal(err)
	}
	if pipeline.Schema != PipelineSchemaV2 {
		t.Fatalf("got schema %q", pipeline.Schema)
	}
	runJSON, err := os.ReadFile("testdata/run-v2.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	run, err := DecodeRun(runJSON)
	if err != nil {
		t.Fatal(err)
	}
	if run.Schema != RunSchemaV2 || run.OutputCommit == nil {
		t.Fatalf("decoded incomplete v2 run: %+v", run)
	}
}

func TestV2ArtifactKindAndAlgorithmAreClosed(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Artifact)
	}{
		{"missing kind", func(a *Artifact) { a.Kind = "" }},
		{"unknown kind", func(a *Artifact) { a.Kind = "blob" }},
		{"missing algorithm", func(a *Artifact) { a.DigestAlgorithm = "" }},
		{"file with tree algorithm", func(a *Artifact) { a.DigestAlgorithm = DigestSHA256TreeV1 }},
		{"tree with file algorithm", func(a *Artifact) {
			a.Kind = ArtifactTree
			a.DigestAlgorithm = DigestSHA256FileV1
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := fixturePipelineV2()
			tt.edit(&pipeline.Matrix[0].Inputs[0])
			if err := pipeline.Validate(); err == nil {
				t.Fatal("invalid v2 artifact was accepted")
			}
		})
	}
}

func TestV1AndV2FieldsDoNotCrossSchemas(t *testing.T) {
	t.Run("v1 rejects v2 artifact fields", func(t *testing.T) {
		pipeline := fixturePipeline()
		pipeline.Matrix[0].Inputs[0].Kind = ArtifactFile
		pipeline.Matrix[0].Inputs[0].DigestAlgorithm = DigestSHA256FileV1
		if err := pipeline.Validate(); err == nil || !strings.Contains(err.Error(), "must be absent from v1") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("v2 rejects v1 artifacts", func(t *testing.T) {
		pipeline := fixturePipeline()
		pipeline.Schema = PipelineSchemaV2
		if err := pipeline.Validate(); err == nil {
			t.Fatal("v2 accepted untyped v1 artifacts")
		}
	})
	t.Run("v1 rejects output commit", func(t *testing.T) {
		run := fixtureRun()
		commit, err := NewOutputCommit(v2FileArtifacts(run.Outputs))
		if err != nil {
			t.Fatal(err)
		}
		run.OutputCommit = &commit
		if err := run.Validate(); err == nil {
			t.Fatal("v1 accepted v2 output commit")
		}
	})
	t.Run("aggregate rejects v1 evidence for v2 pipeline", func(t *testing.T) {
		pipeline := fixturePipelineV2()
		run := fixtureRun()
		run.Executor.OS = "darwin"
		run.Executor.Arch = "arm64"
		if _, err := Aggregate(pipeline, []Run{run}); err == nil || !strings.Contains(err.Error(), "schema mismatch") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("aggregate rejects v2 evidence for v1 pipeline", func(t *testing.T) {
		if _, err := Aggregate(fixturePipeline(), []Run{fixtureRunV2()}); err == nil ||
			!strings.Contains(err.Error(), "schema mismatch") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("v1 Argo lowerer rejects v2", func(t *testing.T) {
		pipeline, binding := argoFixture()
		pipeline.Schema = PipelineSchemaV2
		for i := range pipeline.Matrix {
			pipeline.Matrix[i].Inputs = v2FileArtifacts(pipeline.Matrix[i].Inputs)
			pipeline.Matrix[i].Outputs = v2FileArtifacts(pipeline.Matrix[i].Outputs)
		}
		if _, err := LowerArgo(pipeline, binding); err == nil ||
			!strings.Contains(err.Error(), "trusted-runner") {
			t.Fatalf("got %v", err)
		}
	})
}

func TestOutputCommitBindsCompleteCanonicalSet(t *testing.T) {
	run := fixtureRunV2()
	if err := run.Validate(); err != nil {
		t.Fatal(err)
	}
	t.Run("tampered digest", func(t *testing.T) {
		edited := run
		edited.Outputs = append([]Artifact(nil), run.Outputs...)
		edited.Outputs[0].SHA256 = strings.Repeat("f", 64)
		if err := edited.Validate(); err == nil || !strings.Contains(err.Error(), "canonical outputs manifest") {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("partial set", func(t *testing.T) {
		edited := run
		edited.Outputs = append([]Artifact(nil), run.Outputs[:1]...)
		if err := edited.Validate(); err == nil {
			t.Fatal("partial output set retained pass evidence")
		}
	})
	t.Run("order independent", func(t *testing.T) {
		reversed := append([]Artifact(nil), run.Outputs...)
		reversed[0], reversed[1] = reversed[1], reversed[0]
		commit, err := NewOutputCommit(reversed)
		if err != nil {
			t.Fatal(err)
		}
		if commit != *run.OutputCommit {
			t.Fatal("output commit identity depended on declaration order")
		}
	})
	t.Run("missing commit", func(t *testing.T) {
		edited := run
		edited.OutputCommit = nil
		if err := edited.Validate(); err == nil {
			t.Fatal("v2 pass evidence omitted its output commit")
		}
	})
}

func TestAggregateAcceptsCompleteV2Evidence(t *testing.T) {
	pipeline := fixturePipelineV2()
	runs := make([]Run, 0, len(pipeline.Matrix))
	for _, entry := range pipeline.Matrix {
		run := fixtureRunV2()
		run.Run = "run-" + entry.Platform.OS
		run.Inputs = append([]Artifact(nil), entry.Inputs...)
		run.Outputs = append([]Artifact(nil), entry.Outputs...)
		run.Executor = Executor{
			Node:    "node-" + entry.Platform.OS,
			Backend: entry.Platform.Backend,
			OS:      entry.Platform.OS,
			Arch:    entry.Platform.Arch,
		}
		commit, err := NewOutputCommit(run.Outputs)
		if err != nil {
			t.Fatal(err)
		}
		run.OutputCommit = &commit
		runs = append(runs, run)
	}
	result, err := Aggregate(pipeline, runs)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result != "pass" || len(result.Runs) != len(pipeline.Matrix) {
		t.Fatalf("unexpected aggregate: %+v", result)
	}
}

func TestV1GoldenRegressionRemainsByteExact(t *testing.T) {
	pipeline, err := MarshalPipeline(fixturePipeline())
	if err != nil {
		t.Fatal(err)
	}
	run, err := MarshalRun(fixtureRun())
	if err != nil {
		t.Fatal(err)
	}
	wantPipeline, err := os.ReadFile("testdata/pipeline.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	wantRun, err := os.ReadFile("testdata/run.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(pipeline) != string(wantPipeline) || string(run) != string(wantRun) {
		t.Fatal("v1 canonical bytes regressed")
	}
}
