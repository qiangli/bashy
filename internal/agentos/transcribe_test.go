package agentos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTranscribePublicCommandPreservesInputAndDefaults(t *testing.T) {
	var name string
	var got []string
	var out, errb bytes.Buffer
	input := "https://example.test/watch?v=one&name=a b;$(bad)"
	code := runTranscribe([]string{"--format", "SRT", "--source", "file-source", input}, &out, &errb,
		func(n string, args []string) int { name, got = n, args; return 0 })
	if code != 0 || name != transcribePipelineCommand {
		t.Fatalf("code=%d name=%q stderr=%s", code, name, errb.String())
	}
	want := []string{"--model", "default", "--language", "auto", "--format", "srt", "--device", "cpu", "--source", "file-source", input}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args=%q, want %q", got, want)
	}
}

func TestTranscribeRejectsInvalidInvocationBeforeProvider(t *testing.T) {
	runs := 0
	run := func(string, []string) int { runs++; return 0 }
	for _, args := range [][]string{{}, {"a", "b"}, {"--format", "pdf", "a"}, {"stage", "decode", "request.json"}} {
		var out, errb bytes.Buffer
		if code := runTranscribe(args, &out, &errb, run); code != 2 || errb.Len() == 0 {
			t.Errorf("args=%q code=%d stderr=%q", args, code, errb.String())
		}
	}
	if runs != 0 {
		t.Fatalf("provider ran %d times for invalid input", runs)
	}
}

func TestTranscribeStagesReplaceIndependently(t *testing.T) {
	dir := t.TempDir()
	requests := map[string]string{}
	for stage, request := range map[string]TranscribeStageInput{
		"acquire": {Version: TranscribeStageVersion, Stage: "acquire", Input: "media with spaces.wav"},
		"decode":  {Version: TranscribeStageVersion, Stage: "decode", Artifacts: []TranscribeArtifact{{Path: "media with spaces.wav"}}},
	} {
		path := filepath.Join(dir, stage+" request.json")
		data, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		requests[stage] = path
	}
	var calls []string
	run := func(name string, args []string) int {
		calls = append(calls, name+":"+strings.Join(args, ","))
		return 0
	}
	for _, tc := range []struct{ stage, provider string }{
		{"acquire", "fixture-source"}, {"decode", "wav-decoder"},
		{"acquire", "replacement-source"}, {"decode", "replacement-decoder"},
	} {
		var out, errb bytes.Buffer
		if code := runTranscribe([]string{"stage", tc.stage, "--provider", tc.provider, requests[tc.stage]}, &out, &errb, run); code != 0 {
			t.Fatalf("%s/%s code=%d stderr=%s", tc.stage, tc.provider, code, errb.String())
		}
	}
	want := []string{"fixture-source:" + requests["acquire"], "wav-decoder:" + requests["decode"], "replacement-source:" + requests["acquire"], "replacement-decoder:" + requests["decode"]}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%q, want %q", calls, want)
	}
}

func TestTranscribeStageContractJSONAndValidation(t *testing.T) {
	request := TranscribeStageInput{Version: TranscribeStageVersion, Stage: "infer",
		Artifacts: []TranscribeArtifact{{Path: "audio.wav", MediaType: "audio/wav"}},
		Media:     &TranscribeMedia{DurationMS: 1200, SampleRateHz: 16000, Channels: 1}, Language: "auto"}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	var decoded TranscribeStageInput
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.Validate() != nil {
		t.Fatalf("request round trip: %v %+v", err, decoded)
	}
	result := TranscribeStageResult{Version: TranscribeStageVersion, Stage: "infer",
		Segments: []TranscribeSegment{{StartMS: 0, EndMS: 500, Text: "hello"}, {StartMS: 500, EndMS: 1100, Text: "world"}}}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Segments[1].StartMS = 400
	if err := result.Validate(); err == nil {
		t.Fatal("overlapping segments accepted")
	}
	result = TranscribeStageResult{Version: TranscribeStageVersion, Stage: "decode", Error: &TranscribeStageError{Code: "unsupported_media", Message: "no audio"}}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Artifacts = []TranscribeArtifact{{Path: "partial.wav"}}
	if err := result.Validate(); err == nil {
		t.Fatal("failure carrying success artifact accepted")
	}
	result.Version = "bashy-transcribe-stage-v2"
	if err := result.Validate(); err == nil {
		t.Fatal("unknown version accepted")
	}
}

func TestTranscribeStageRejectsMismatchedRequest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.json")
	data := []byte(`{"version":"bashy-transcribe-stage-v1","stage":"acquire","input":"clip.wav"}`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	runs := 0
	var out, errb bytes.Buffer
	code := runTranscribe([]string{"stage", "decode", "--provider", "fixture-decoder", path}, &out, &errb,
		func(string, []string) int { runs++; return 0 })
	if code != 2 || runs != 0 || !strings.Contains(errb.String(), "does not match") {
		t.Fatalf("code=%d runs=%d stderr=%s", code, runs, errb.String())
	}
}
