package dhnt

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func nativeResultFixture(t *testing.T, payload []byte) (NativeResult, NativeResultExpectation) {
	t.Helper()
	sum := sha256.Sum256(payload)
	artifact := Artifact{
		Name:            "result",
		Kind:            ArtifactFile,
		DigestAlgorithm: DigestSHA256FileV1,
		SHA256:          hex.EncodeToString(sum[:]),
	}
	exitCode := 0
	run := Run{
		Schema:   RunSchemaV2,
		Pipeline: "mixed-v2",
		Task:     "native",
		Run:      "native-run",
		Source: Source{
			Repository: "https://example.test/repository.git",
			Commit:     "abc123",
			SHA256:     strings.Repeat("a", 64),
		},
		Inputs: []Artifact{{
			Name:            "input",
			Kind:            ArtifactFile,
			DigestAlgorithm: DigestSHA256FileV1,
			SHA256:          strings.Repeat("b", 64),
		}},
		Executor: Executor{
			Node: "dragon-vk-native", Backend: "vk-native",
			OS: "darwin", Arch: "arm64",
		},
		Result:     Result{Class: ResultPass, ExitCode: &exitCode},
		Outputs:    []Artifact{artifact},
		StartedAt:  "2026-07-30T00:00:00Z",
		FinishedAt: "2026-07-30T00:00:01Z",
		TraceID:    "0123456789abcdef0123456789abcdef",
	}
	commit, err := NewOutputCommit(run.Outputs)
	if err != nil {
		t.Fatal(err)
	}
	run.OutputCommit = &commit
	result, err := NewNativeResult(run, artifact, payload)
	if err != nil {
		t.Fatal(err)
	}
	return result, NativeResultExpectation{
		Artifact: artifact,
		Executor: run.Executor,
	}
}

func nativeResultMessage(t *testing.T, result NativeResult) []byte {
	t.Helper()
	data, err := MarshalNativeResult(result)
	if err != nil {
		t.Fatal(err)
	}
	return append([]byte("bounded diagnostic\n"+NativeResultMarker), data...)
}

func TestVerifyNativeResultMessagePositive(t *testing.T) {
	result, expected := nativeResultFixture(t, []byte("verified result\n"))
	payload, run, err := VerifyNativeResultMessage(
		nativeResultMessage(t, result), expected)
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "verified result\n" {
		t.Fatalf("payload = %q", payload)
	}
	if run.Executor != expected.Executor || run.Outputs[0] != expected.Artifact {
		t.Fatalf("run did not retain verified binding: %+v", run)
	}
}

func TestVerifyNativeResultMessageRejectsTamperMissingAndExecutorMismatch(t *testing.T) {
	result, expected := nativeResultFixture(t, []byte("verified result\n"))
	tests := []struct {
		name    string
		message func() []byte
		expect  NativeResultExpectation
		want    string
	}{
		{
			name: "tampered payload",
			message: func() []byte {
				tampered := result
				tampered.Payload = "dGFtcGVyZWQgcmVzdWx0Cg=="
				data, err := marshalLine(tampered)
				if err != nil {
					t.Fatal(err)
				}
				return append([]byte(NativeResultMarker), data...)
			},
			expect: expected,
			want:   "sha256",
		},
		{
			name:    "missing marker",
			message: func() []byte { return []byte("ordinary terminal output") },
			expect:  expected,
			want:    "no result marker",
		},
		{
			name: "multiple markers",
			message: func() []byte {
				message := nativeResultMessage(t, result)
				return append(message, append([]byte(NativeResultMarker), message...)...)
			},
			expect: expected,
			want:   "multiple",
		},
		{
			name:    "live executor mismatch",
			message: func() []byte { return nativeResultMessage(t, result) },
			expect: NativeResultExpectation{
				Artifact: expected.Artifact,
				Executor: Executor{
					Node: "other-vk-native", Backend: "vk-native",
					OS: "darwin", Arch: "arm64",
				},
			},
			want: "live executor",
		},
		{
			name:    "expected kind mismatch",
			message: func() []byte { return nativeResultMessage(t, result) },
			expect: NativeResultExpectation{
				Artifact: Artifact{
					Name: "result", Kind: ArtifactTree,
					DigestAlgorithm: DigestSHA256TreeV1,
					SHA256:          expected.Artifact.SHA256,
				},
				Executor: expected.Executor,
			},
			want: "expected name/kind/digest",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, err := VerifyNativeResultMessage(test.message(), test.expect)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestVerifyNativeResultMessageIsBoundedAndCanonical(t *testing.T) {
	result, expected := nativeResultFixture(t, []byte("result"))
	message := bytes.Repeat([]byte("x"), MaxNativeResultMessageBytes+1)
	if _, _, err := VerifyNativeResultMessage(message, expected); err == nil ||
		!strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversize error = %v", err)
	}
	data, err := MarshalNativeResult(result)
	if err != nil {
		t.Fatal(err)
	}
	noncanonical := bytes.Replace(data, []byte(`"artifact":`), []byte(`"artifact": `), 1)
	message = append([]byte(NativeResultMarker), noncanonical...)
	if _, _, err := VerifyNativeResultMessage(message, expected); err == nil ||
		!strings.Contains(err.Error(), "canonical") {
		t.Fatalf("noncanonical error = %v", err)
	}
}

func TestNewNativeResultRejectsTreeAndOversize(t *testing.T) {
	result, _ := nativeResultFixture(t, []byte("result"))
	tree := result.Artifact
	tree.Kind = ArtifactTree
	tree.DigestAlgorithm = DigestSHA256TreeV1
	result.Artifact = tree
	result.Run.Outputs = []Artifact{tree}
	commit, err := NewOutputCommit(result.Run.Outputs)
	if err != nil {
		t.Fatal(err)
	}
	result.Run.OutputCommit = &commit
	if _, err := NewNativeResult(result.Run, tree, []byte("result")); err == nil ||
		!strings.Contains(err.Error(), "supports only") {
		t.Fatalf("tree error = %v", err)
	}
	if _, err := NewNativeResult(
		result.Run, result.Artifact,
		bytes.Repeat([]byte("x"), MaxNativeResultArtifactBytes+1)); err == nil ||
		!strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversize error = %v", err)
	}
}
