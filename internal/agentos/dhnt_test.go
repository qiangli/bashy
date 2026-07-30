package agentos

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/dhnt"
)

func TestDhntNativeResultWrapAndVerifyRoundTrip(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("verified native result\n")
	sum := sha256.Sum256(payload)
	artifact := dhnt.Artifact{
		Name: "result", Kind: dhnt.ArtifactFile,
		DigestAlgorithm: dhnt.DigestSHA256FileV1,
		SHA256:          hex.EncodeToString(sum[:]),
	}
	exitCode := 0
	run := dhnt.Run{
		Schema:   dhnt.RunSchemaV2,
		Pipeline: "mixed-v2",
		Task:     "native",
		Run:      "native-run",
		Source: dhnt.Source{
			Repository: "https://example.test/repository.git",
			Commit:     "abc123", SHA256: strings.Repeat("a", 64),
		},
		Inputs: []dhnt.Artifact{{
			Name: "input", Kind: dhnt.ArtifactFile,
			DigestAlgorithm: dhnt.DigestSHA256FileV1,
			SHA256:          strings.Repeat("b", 64),
		}},
		Executor: dhnt.Executor{
			Node: "dragon-vk-native", Backend: "vk-native",
			OS: "darwin", Arch: "arm64",
		},
		Result:     dhnt.Result{Class: dhnt.ResultPass, ExitCode: &exitCode},
		Outputs:    []dhnt.Artifact{artifact},
		StartedAt:  "2026-07-30T00:00:00Z",
		FinishedAt: "2026-07-30T00:00:01Z",
		TraceID:    "0123456789abcdef0123456789abcdef",
	}
	commit, err := dhnt.NewOutputCommit(run.Outputs)
	if err != nil {
		t.Fatal(err)
	}
	run.OutputCommit = &commit
	runJSON, err := dhnt.MarshalRun(run)
	if err != nil {
		t.Fatal(err)
	}
	runPath := filepath.Join(dir, "run.json")
	artifactPath := filepath.Join(dir, "result.json")
	if err := os.WriteFile(runPath, runJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifactPath, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	messagePath := filepath.Join(dir, "message")
	if code := withDhntStdoutFile(t, messagePath, func() int {
		return dhntWrapNativeResult([]string{
			"--run", runPath, "--artifact", artifactPath,
		})
	}); code != 0 {
		t.Fatalf("wrap exit = %d", code)
	}
	verifiedPath := filepath.Join(dir, "verified.json")
	canonicalPath := filepath.Join(dir, "canonical-run.json")
	code := withDhntStdoutFile(t, canonicalPath, func() int {
		return dhntVerifyNativeResult([]string{
			"--expect-name", artifact.Name,
			"--expect-kind", string(artifact.Kind),
			"--expect-sha256", artifact.SHA256,
			"--expect-node", run.Executor.Node,
			"--expect-backend", run.Executor.Backend,
			"--expect-os", run.Executor.OS,
			"--expect-arch", run.Executor.Arch,
			"--artifact-output", verifiedPath,
			messagePath,
		})
	})
	if code != 0 {
		t.Fatalf("verify exit = %d", code)
	}
	if got, err := os.ReadFile(verifiedPath); err != nil || string(got) != string(payload) {
		t.Fatalf("verified payload = %q, %v", got, err)
	}
	if got, err := os.ReadFile(canonicalPath); err != nil || string(got) != string(runJSON) {
		t.Fatalf("canonical run mismatch: %v", err)
	}
}

func withDhntStdoutFile(t *testing.T, path string, run func() int) int {
	t.Helper()
	output, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = output
	code := run()
	os.Stdout = old
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	return code
}

func TestDhntEmitRunRejectsAbsentExitCode(t *testing.T) {
	digest := strings.Repeat("a", 64)
	args := []string{
		"--pipeline", "release",
		"--task", "smoke",
		"--run", "run-1",
		"--source-repository", "https://example.test/repository.git",
		"--source-commit", "abc123",
		"--source-sha256", digest,
		"--input", "candidate=" + digest,
		"--node", "node-1",
		"--backend", "vk-native",
		"--os", "linux",
		"--arch", "amd64",
		"--class", "pass",
		"--output", "tested-candidate=" + digest,
		"--started-at", "2026-07-29T12:00:00Z",
		"--finished-at", "2026-07-29T12:00:01Z",
		"--trace-id", "0123456789abcdef0123456789abcdef",
	}

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	oldStdout := os.Stdout
	os.Stdout = devNull
	code := dhntEmitRun(args)
	os.Stdout = oldStdout

	if code == 0 {
		t.Fatal("emit-run treated an absent --exit-code as observed exit code zero")
	}
}

func TestDhntEmitRunFailsWhenEvidenceCannotBeWritten(t *testing.T) {
	digest := strings.Repeat("a", 64)
	args := []string{
		"--pipeline", "release",
		"--task", "smoke",
		"--run", "run-1",
		"--source-repository", "https://example.test/repository.git",
		"--source-commit", "abc123",
		"--source-sha256", digest,
		"--input", "candidate=" + digest,
		"--node", "node-1",
		"--backend", "vk-native",
		"--os", "linux",
		"--arch", "amd64",
		"--class", "pass",
		"--exit-code", "0",
		"--output", "tested-candidate=" + digest,
		"--started-at", "2026-07-29T12:00:00Z",
		"--finished-at", "2026-07-29T12:00:01Z",
		"--trace-id", "0123456789abcdef0123456789abcdef",
	}

	closed, err := os.CreateTemp(t.TempDir(), "closed-stdout")
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = closed
	t.Cleanup(func() { os.Stdout = oldStdout })

	if code := dhntEmitRun(args); code == 0 {
		t.Fatal("emit-run reported success after stdout rejected the evidence record")
	}
}

func TestDhntAggregateRequireOSCannotBeSatisfiedByNonNativeLane(t *testing.T) {
	digest := strings.Repeat("a", 64)
	source := dhnt.Source{
		Repository: "https://example.test/repository.git",
		Commit:     "abc123",
		SHA256:     digest,
	}
	pipeline := dhnt.Pipeline{
		Schema:   dhnt.PipelineSchema,
		Pipeline: "release",
		Source:   source,
		Tasks: []dhnt.Task{
			{
				ID:               "native",
				Lane:             dhnt.LaneNative,
				Distribution:     dhnt.DistributionSingle,
				Needs:            []string{},
				Argv:             []string{"test"},
				WorkingDirectory: ".",
				Environment:      []dhnt.Environment{},
			},
			{
				ID:               "container",
				Lane:             dhnt.LaneContainer,
				Distribution:     dhnt.DistributionSingle,
				Needs:            []string{},
				Argv:             []string{"test"},
				WorkingDirectory: ".",
				Environment:      []dhnt.Environment{},
			},
		},
		Matrix: []dhnt.MatrixEntry{
			{
				Task:     "native",
				Platform: dhnt.Platform{Backend: "vk-native", OS: "darwin", Arch: "arm64"},
				Inputs:   []dhnt.Artifact{{Name: "candidate", SHA256: digest}},
				Outputs:  []dhnt.Artifact{{Name: "tested-candidate", SHA256: digest}},
			},
			{
				Task:     "container",
				Platform: dhnt.Platform{Backend: "vk-podman", OS: "windows", Arch: "amd64"},
				Inputs:   []dhnt.Artifact{{Name: "candidate", SHA256: digest}},
				Outputs:  []dhnt.Artifact{{Name: "tested-candidate", SHA256: digest}},
			},
		},
	}
	run := func(task, id, backend, goos, arch string) dhnt.Run {
		return dhnt.Run{
			Schema:     dhnt.RunSchema,
			Pipeline:   pipeline.Pipeline,
			Task:       task,
			Run:        id,
			Source:     source,
			Inputs:     []dhnt.Artifact{{Name: "candidate", SHA256: digest}},
			Executor:   dhnt.Executor{Node: "node", Backend: backend, OS: goos, Arch: arch},
			Result:     dhnt.Result{Class: dhnt.ResultPass, ExitCode: testIntPtr(0)},
			Outputs:    []dhnt.Artifact{{Name: "tested-candidate", SHA256: digest}},
			StartedAt:  "2026-07-29T12:00:00Z",
			FinishedAt: "2026-07-29T12:00:01Z",
			TraceID:    "0123456789abcdef0123456789abcdef",
		}
	}

	dir := t.TempDir()
	pipelinePath := filepath.Join(dir, "pipeline.json")
	writeDhntTestJSON(t, pipelinePath, mustMarshalPipeline(t, pipeline))
	runPaths := []string{
		filepath.Join(dir, "native.json"),
		filepath.Join(dir, "container.json"),
	}
	writeDhntTestJSON(t, runPaths[0], mustMarshalRun(t, run("native", "native-run", "vk-native", "darwin", "arm64")))
	writeDhntTestJSON(t, runPaths[1], mustMarshalRun(t, run("container", "container-run", "vk-podman", "windows", "amd64")))

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = devNull, devNull
	code := dhntAggregate([]string{
		"--pipeline", pipelinePath,
		"--require-os", "windows",
		runPaths[0], runPaths[1],
	})
	os.Stdout, os.Stderr = oldStdout, oldStderr

	if code == 0 {
		t.Fatal("--require-os windows was satisfied only by a non-native matrix entry")
	}
}

func TestDhntAggregateRequireOSMustCheckTaskLaneNotOnlyBackend(t *testing.T) {
	digest := strings.Repeat("a", 64)
	source := dhnt.Source{
		Repository: "https://example.test/repository.git",
		Commit:     "abc123",
		SHA256:     digest,
	}
	pipeline := dhnt.Pipeline{
		Schema:   dhnt.PipelineSchema,
		Pipeline: "release",
		Source:   source,
		Tasks: []dhnt.Task{
			{
				ID:               "native",
				Lane:             dhnt.LaneNative,
				Distribution:     dhnt.DistributionSingle,
				Needs:            []string{},
				Argv:             []string{"test"},
				WorkingDirectory: ".",
				Environment:      []dhnt.Environment{},
			},
			{
				ID:               "container",
				Lane:             dhnt.LaneContainer,
				Distribution:     dhnt.DistributionSingle,
				Needs:            []string{},
				Argv:             []string{"test"},
				WorkingDirectory: ".",
				Environment:      []dhnt.Environment{},
			},
		},
		Matrix: []dhnt.MatrixEntry{
			{
				Task:     "native",
				Platform: dhnt.Platform{Backend: "vk-native", OS: "darwin", Arch: "arm64"},
				Inputs:   []dhnt.Artifact{{Name: "candidate", SHA256: digest}},
				Outputs:  []dhnt.Artifact{{Name: "tested-candidate", SHA256: digest}},
			},
			{
				Task:     "container",
				Platform: dhnt.Platform{Backend: "vk-native", OS: "windows", Arch: "amd64"},
				Inputs:   []dhnt.Artifact{{Name: "candidate", SHA256: digest}},
				Outputs:  []dhnt.Artifact{{Name: "tested-candidate", SHA256: digest}},
			},
		},
	}
	run := func(task, id, backend, goos, arch string) dhnt.Run {
		return dhnt.Run{
			Schema:     dhnt.RunSchema,
			Pipeline:   pipeline.Pipeline,
			Task:       task,
			Run:        id,
			Source:     source,
			Inputs:     []dhnt.Artifact{{Name: "candidate", SHA256: digest}},
			Executor:   dhnt.Executor{Node: "node", Backend: backend, OS: goos, Arch: arch},
			Result:     dhnt.Result{Class: dhnt.ResultPass, ExitCode: testIntPtr(0)},
			Outputs:    []dhnt.Artifact{{Name: "tested-candidate", SHA256: digest}},
			StartedAt:  "2026-07-29T12:00:00Z",
			FinishedAt: "2026-07-29T12:00:01Z",
			TraceID:    "0123456789abcdef0123456789abcdef",
		}
	}

	dir := t.TempDir()
	pipelinePath := filepath.Join(dir, "pipeline.json")
	writeDhntTestJSON(t, pipelinePath, mustMarshalPipeline(t, pipeline))
	runPaths := []string{
		filepath.Join(dir, "native.json"),
		filepath.Join(dir, "container.json"),
	}
	writeDhntTestJSON(t, runPaths[0], mustMarshalRun(t, run("native", "native-run", "vk-native", "darwin", "arm64")))
	writeDhntTestJSON(t, runPaths[1], mustMarshalRun(t, run("container", "container-run", "vk-native", "windows", "amd64")))

	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = devNull, devNull
	code := dhntAggregate([]string{
		"--pipeline", pipelinePath,
		"--require-os", "windows",
		runPaths[0], runPaths[1],
	})
	os.Stdout, os.Stderr = oldStdout, oldStderr

	if code == 0 {
		t.Fatal("--require-os windows must not be satisfied by a non-native (LaneContainer) matrix entry even with vk-native backend")
	}
}

func testIntPtr(value int) *int { return &value }

func mustMarshalPipeline(t *testing.T, pipeline dhnt.Pipeline) []byte {
	t.Helper()
	data, err := dhnt.MarshalPipeline(pipeline)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustMarshalRun(t *testing.T, run dhnt.Run) []byte {
	t.Helper()
	data, err := dhnt.MarshalRun(run)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeDhntTestJSON(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
