package agentos

import "github.com/qiangli/bashy/internal/transcribecontract"

const TranscribeStageVersion = transcribecontract.TranscribeStageVersion

type TranscribeArtifact = transcribecontract.TranscribeArtifact
type TranscribeMedia = transcribecontract.TranscribeMedia
type TranscribeSegment = transcribecontract.TranscribeSegment
type TranscribeStageError = transcribecontract.TranscribeStageError
type TranscribeStageInput = transcribecontract.TranscribeStageInput
type TranscribeStageResult = transcribecontract.TranscribeStageResult

func validateTranscribeSegments(v []TranscribeSegment) error {
	return transcribecontract.ValidateSegments(v)
}
