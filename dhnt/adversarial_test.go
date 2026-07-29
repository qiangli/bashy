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
