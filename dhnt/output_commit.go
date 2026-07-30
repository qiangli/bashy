package dhnt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	OutputCommitSchema         = "dhnt.output-commit/v1"
	DigestSHA256OutputCommitV1 = "sha256-output-commit-v1"
)

// OutputCommit identifies the canonical manifest whose atomic publication is
// the visibility point for a complete v2 output set.
type OutputCommit struct {
	Schema          string `json:"schema"`
	DigestAlgorithm string `json:"digestAlgorithm"`
	SHA256          string `json:"sha256"`
}

type outputCommitManifest struct {
	Schema  string     `json:"schema"`
	Outputs []Artifact `json:"outputs"`
}

// MarshalOutputCommitManifest returns the exact canonical bytes committed by a
// v2 artifact runner. Consumers must not treat output paths alone as committed.
func MarshalOutputCommitManifest(outputs []Artifact) ([]byte, error) {
	if err := validateArtifacts("outputs", outputs, true, RunSchemaV2); err != nil {
		return nil, err
	}
	manifest := outputCommitManifest{
		Schema:  OutputCommitSchema,
		Outputs: sortedArtifacts(outputs),
	}
	return marshalLine(manifest)
}

// NewOutputCommit computes the evidence identity for a canonical output
// commit manifest.
func NewOutputCommit(outputs []Artifact) (OutputCommit, error) {
	manifest, err := MarshalOutputCommitManifest(outputs)
	if err != nil {
		return OutputCommit{}, err
	}
	digest := sha256.Sum256(manifest)
	return OutputCommit{
		Schema:          OutputCommitSchema,
		DigestAlgorithm: DigestSHA256OutputCommitV1,
		SHA256:          hex.EncodeToString(digest[:]),
	}, nil
}

func validateOutputCommit(commit *OutputCommit, outputs []Artifact) error {
	if commit == nil {
		return errors.New("outputCommit: must be present in dhnt.run/v2")
	}
	if commit.Schema != OutputCommitSchema {
		return fmt.Errorf("outputCommit.schema: got %q, want %q", commit.Schema, OutputCommitSchema)
	}
	if commit.DigestAlgorithm != DigestSHA256OutputCommitV1 {
		return fmt.Errorf("outputCommit.digestAlgorithm: got %q, want %q", commit.DigestAlgorithm, DigestSHA256OutputCommitV1)
	}
	if err := validateDigest("outputCommit.sha256", commit.SHA256); err != nil {
		return err
	}
	expected, err := NewOutputCommit(outputs)
	if err != nil {
		return fmt.Errorf("outputCommit: %w", err)
	}
	if *commit != expected {
		return errors.New("outputCommit.sha256: does not identify the canonical outputs manifest")
	}
	return nil
}
