package dhnt

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	NativeResultSchema           = "dhnt.native-result/v1"
	NativeResultMarker           = "DKS_NATIVE_RESULT:"
	MaxNativeResultMessageBytes  = 4 * 1024
	MaxNativeResultArtifactBytes = 2 * 1024
)

// NativeResult carries one small verified file through vk-native's bounded
// terminal-status channel. Large artifacts still belong in authenticated
// artifact storage; their small commit/result record can use this envelope.
type NativeResult struct {
	Schema        string   `json:"schema"`
	Artifact      Artifact `json:"artifact"`
	Encoding      string   `json:"encoding"`
	ArtifactBytes int      `json:"artifactBytes"`
	Payload       string   `json:"payload"`
	Run           Run      `json:"run"`
}

// NativeResultExpectation is resolved from the trusted pipeline/binding and
// live Kubernetes Pod identity, never from the native payload itself.
type NativeResultExpectation struct {
	Artifact Artifact
	Executor Executor
}

func MarshalNativeResult(result NativeResult) ([]byte, error) {
	if _, err := validateNativeResult(result, NativeResultExpectation{
		Artifact: result.Artifact,
		Executor: result.Run.Executor,
	}); err != nil {
		return nil, err
	}
	return marshalLine(result)
}

// VerifyNativeResultMessage validates exactly one result marker from the
// apiserver-visible, kubelet-sized terminal message and returns its file bytes.
func VerifyNativeResultMessage(message []byte, expected NativeResultExpectation) ([]byte, Run, error) {
	if len(message) > MaxNativeResultMessageBytes {
		return nil, Run{}, fmt.Errorf("native result message exceeds %d bytes", MaxNativeResultMessageBytes)
	}
	var encoded []byte
	for _, line := range bytes.Split(message, []byte{'\n'}) {
		if bytes.HasPrefix(line, []byte(NativeResultMarker)) {
			if encoded != nil {
				return nil, Run{}, errors.New("native result message has multiple result markers")
			}
			encoded = bytes.TrimPrefix(line, []byte(NativeResultMarker))
		}
	}
	if encoded == nil {
		return nil, Run{}, errors.New("native result message has no result marker")
	}
	if len(encoded) == 0 {
		return nil, Run{}, errors.New("native result marker is empty")
	}
	var result NativeResult
	if err := decodeStrict(encoded, &result); err != nil {
		return nil, Run{}, fmt.Errorf("native result: %w", err)
	}
	canonical, err := marshalLine(result)
	if err != nil {
		return nil, Run{}, err
	}
	if !bytes.Equal(append(encoded, '\n'), canonical) {
		return nil, Run{}, errors.New("native result: envelope must use canonical JSON encoding")
	}
	payload, err := validateNativeResult(result, expected)
	if err != nil {
		return nil, Run{}, err
	}
	return payload, result.Run, nil
}

func validateNativeResult(result NativeResult, expected NativeResultExpectation) ([]byte, error) {
	if result.Schema != NativeResultSchema {
		return nil, fmt.Errorf("native result schema: got %q, want %q", result.Schema, NativeResultSchema)
	}
	if result.Artifact.Kind != ArtifactFile ||
		result.Artifact.DigestAlgorithm != DigestSHA256FileV1 {
		return nil, fmt.Errorf("native result artifact: bounded channel supports only %s/%s, got %s/%s",
			ArtifactFile, DigestSHA256FileV1,
			result.Artifact.Kind, result.Artifact.DigestAlgorithm)
	}
	if err := validateArtifacts("native result artifact", []Artifact{result.Artifact}, true, RunSchemaV2); err != nil {
		return nil, err
	}
	if result.Artifact != expected.Artifact {
		return nil, errors.New("native result artifact does not match expected name/kind/digest")
	}
	if result.Encoding != "base64" {
		return nil, fmt.Errorf("native result encoding: got %q, want %q", result.Encoding, "base64")
	}
	if result.ArtifactBytes < 0 || result.ArtifactBytes > MaxNativeResultArtifactBytes {
		return nil, fmt.Errorf("native result artifactBytes: must be between 0 and %d", MaxNativeResultArtifactBytes)
	}
	payload, err := base64.StdEncoding.Strict().DecodeString(result.Payload)
	if err != nil {
		return nil, fmt.Errorf("native result payload: %w", err)
	}
	if len(payload) != result.ArtifactBytes {
		return nil, errors.New("native result artifactBytes does not match decoded payload")
	}
	digest := sha256.Sum256(payload)
	if hex.EncodeToString(digest[:]) != result.Artifact.SHA256 {
		return nil, errors.New("native result payload sha256 does not match expected artifact")
	}
	if err := result.Run.Validate(); err != nil {
		return nil, fmt.Errorf("native result run: %w", err)
	}
	if result.Run.Schema != RunSchemaV2 {
		return nil, fmt.Errorf("native result run schema: got %q, want %q", result.Run.Schema, RunSchemaV2)
	}
	if result.Run.Result.Class != ResultPass {
		return nil, errors.New("native result run: only pass evidence can publish an artifact")
	}
	if result.Run.Executor != expected.Executor {
		return nil, fmt.Errorf("native result executor does not match live executor: got %s/%s/%s/%s",
			result.Run.Executor.Node, result.Run.Executor.Backend,
			result.Run.Executor.OS, result.Run.Executor.Arch)
	}
	if len(result.Run.Outputs) != 1 || result.Run.Outputs[0] != result.Artifact {
		return nil, errors.New("native result run outputs do not bind exactly the carried artifact")
	}
	return payload, nil
}

// NewNativeResult constructs a canonical bounded-channel result around an
// already-valid passing dhnt.run/v2 record.
func NewNativeResult(run Run, artifact Artifact, payload []byte) (NativeResult, error) {
	if len(payload) > MaxNativeResultArtifactBytes {
		return NativeResult{}, fmt.Errorf("native result payload exceeds %d bytes", MaxNativeResultArtifactBytes)
	}
	result := NativeResult{
		Schema:        NativeResultSchema,
		Artifact:      artifact,
		Encoding:      "base64",
		ArtifactBytes: len(payload),
		Payload:       base64.StdEncoding.EncodeToString(payload),
		Run:           run,
	}
	encoded, err := MarshalNativeResult(result)
	if err != nil {
		return NativeResult{}, err
	}
	if len(NativeResultMarker)+len(encoded) > MaxNativeResultMessageBytes {
		return NativeResult{}, errors.New("native result envelope exceeds terminal message budget")
	}
	return result, nil
}
