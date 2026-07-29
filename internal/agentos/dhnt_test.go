package agentos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qiangli/bashy/dhnt"
)

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
				Needs:            []string{},
				Argv:             []string{"test"},
				WorkingDirectory: ".",
				Environment:      []dhnt.Environment{},
			},
			{
				ID:               "container",
				Lane:             dhnt.LaneContainer,
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
