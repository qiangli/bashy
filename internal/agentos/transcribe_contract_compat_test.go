package agentos

import (
	"encoding/json"
	c "github.com/qiangli/bashy/internal/transcribecontract"
	"testing"
)

func TestTranscribeSharedContractCompatibility(t *testing.T) {
	var old TranscribeStageInput
	if err := json.Unmarshal([]byte(`{"version":"bashy-transcribe-stage-v1","stage":"infer","artifacts":[{"path":"audio.wav"}],"model":"default","language":"auto","work_dir":"/tmp/run"}`), &old); err != nil {
		t.Fatal(err)
	}
	var shared c.TranscribeStageInput = old // alias assignment must remain source-compatible
	if err := shared.Validate(); err != nil {
		t.Fatal(err)
	}
	if shared.WorkDir != "/tmp/run" {
		t.Fatal("work_dir lost")
	}
	for _, segments := range [][]TranscribeSegment{
		{{StartMS: 0, EndMS: 10, Text: "a"}, {StartMS: 9, EndMS: 20, Text: "b"}},
		{{StartMS: 0, EndMS: 0, Text: "a"}},
		{{StartMS: 0, EndMS: 10, Text: ""}},
	} {
		if validateTranscribeSegments(segments) == nil {
			t.Fatal("original validation weakened")
		}
	}
}
