package dhnt

import (
	"strings"
	"testing"
)

func TestDecodePipelineRejectsAbsentEnvironmentValue(t *testing.T) {
	data, err := MarshalPipeline(fixturePipeline())
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(
		string(data),
		`{"name":"A","value":"first"}`,
		`{"name":"A"}`,
		1,
	))

	if _, err := DecodePipeline(data); err == nil {
		t.Fatal("strict decoder accepted an environment entry with an absent required value field")
	}
}

func TestDecodeRunRejectsUnpairedLowSurrogateAfterEscapedLiteral(t *testing.T) {
	data, err := MarshalRun(fixtureRun())
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(
		string(data),
		`"repository":"https://`,
		`"repository":"\\uD800\uDC00https://`,
		1,
	))

	if _, err := DecodeRun(data); err == nil {
		t.Fatal("strict decoder accepted an unpaired low surrogate after an escaped literal that only resembles a high surrogate")
	}
}

func TestPipelineRejectsNULInProcessParameters(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Task)
	}{
		{
			name: "argv",
			edit: func(task *Task) {
				task.Argv = append(task.Argv, "argument\x00suffix")
			},
		},
		{
			name: "environment value",
			edit: func(task *Task) {
				task.Environment[0].Value = "value\x00suffix"
			},
		},
		{
			name: "working directory",
			edit: func(task *Task) {
				task.WorkingDirectory = "directory\x00suffix"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := fixturePipeline()
			tt.edit(&pipeline.Tasks[0])
			if err := pipeline.Validate(); err == nil {
				t.Fatal("pipeline accepted a NUL byte that cannot be represented in an OS process invocation")
			}
		})
	}
}

func TestPipelineRejectsNULInSourceIdentity(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Source)
	}{
		{
			name: "repository",
			edit: func(source *Source) {
				source.Repository = "https://example.test/repo.git\x00suffix"
			},
		},
		{
			name: "commit",
			edit: func(source *Source) {
				source.Commit = "abc123\x00suffix"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pipeline := fixturePipeline()
			tt.edit(&pipeline.Source)
			if err := pipeline.Validate(); err == nil {
				t.Fatal("pipeline accepted a NUL byte in source identity")
			}
		})
	}
}

func TestRunRejectsNULInSourceAndExecutorIdentity(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Run)
	}{
		{
			name: "source repository",
			edit: func(run *Run) {
				run.Source.Repository = "https://example.test/repo.git\x00suffix"
			},
		},
		{
			name: "source commit",
			edit: func(run *Run) {
				run.Source.Commit = "abc123\x00suffix"
			},
		},
		{
			name: "executor node",
			edit: func(run *Run) {
				run.Executor.Node = "node\x00suffix"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			run := fixtureRun()
			tt.edit(&run)
			if err := run.Validate(); err == nil {
				t.Fatal("run accepted a NUL byte in source or executor identity")
			}
		})
	}
}

func TestRunRejectsNegativeExitCodeForNonPassResults(t *testing.T) {
	for _, class := range []ResultClass{
		ResultTestFail, ResultInfraFail, ResultIncomplete, ResultCanceled,
	} {
		t.Run(string(class), func(t *testing.T) {
			run := fixtureRun()
			exitCode := -1
			run.Result = Result{Class: class, ExitCode: &exitCode}

			if err := run.Validate(); err == nil {
				t.Fatal("run accepted a negative exit code that cannot be an observed process exit status")
			}
		})
	}
}

func TestRunRejectsNegativeExitCodeForPassResult(t *testing.T) {
	run := fixtureRun()
	exitCode := -1
	run.Result = Result{Class: ResultPass, ExitCode: &exitCode}

	if err := run.Validate(); err == nil {
		t.Fatal("run accepted a negative exit code for pass that cannot be an observed process exit status")
	}
}
