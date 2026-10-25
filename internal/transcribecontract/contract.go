package transcribecontract

import "fmt"

// Stage payloads are data exchanged by registered commands. They do not
// replace the existing unit.Attestation receipt used by Bash# skills/scripts.
const TranscribeStageVersion = "bashy-transcribe-stage-v1"

type TranscribeArtifact struct {
	Path      string `json:"path"`
	MediaType string `json:"media_type,omitempty"`
	SHA256    string `json:"sha256,omitempty"`
}

type TranscribeMedia struct {
	DurationMS   int64  `json:"duration_ms,omitempty"`
	SampleRateHz int    `json:"sample_rate_hz,omitempty"`
	Channels     int    `json:"channels,omitempty"`
	Language     string `json:"language,omitempty"`
	Source       string `json:"source,omitempty"`
}

type TranscribeSegment struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

type TranscribeStageError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable,omitempty"`
}

type TranscribeStageInput struct {
	Version   string               `json:"version"`
	Stage     string               `json:"stage"`
	WorkDir   string               `json:"work_dir,omitempty"`
	Input     string               `json:"input,omitempty"`
	Artifacts []TranscribeArtifact `json:"artifacts,omitempty"`
	Media     *TranscribeMedia     `json:"media,omitempty"`
	Segments  []TranscribeSegment  `json:"segments,omitempty"`
	Model     string               `json:"model,omitempty"`
	Language  string               `json:"language,omitempty"`
	Format    string               `json:"format,omitempty"`
	Output    string               `json:"output,omitempty"`
}

type TranscribeStageResult struct {
	Version   string                `json:"version"`
	Stage     string                `json:"stage"`
	Artifacts []TranscribeArtifact  `json:"artifacts,omitempty"`
	Media     *TranscribeMedia      `json:"media,omitempty"`
	Segments  []TranscribeSegment   `json:"segments,omitempty"`
	Error     *TranscribeStageError `json:"error,omitempty"`
}

func (v TranscribeStageInput) Validate() error {
	if v.Version != TranscribeStageVersion || !Stage(v.Stage) {
		return fmt.Errorf("unsupported transcription stage input version or stage: %q/%q", v.Version, v.Stage)
	}
	if v.Stage == "acquire" && v.Input == "" {
		return fmt.Errorf("acquire input requires a file or URL")
	}
	if (v.Stage == "decode" || v.Stage == "infer") && len(v.Artifacts) == 0 {
		return fmt.Errorf("%s input requires an artifact", v.Stage)
	}
	if v.Stage == "export" && len(v.Segments) == 0 {
		return fmt.Errorf("export input requires transcript segments")
	}
	for _, a := range v.Artifacts {
		if a.Path == "" {
			return fmt.Errorf("artifact path is empty")
		}
	}
	return ValidateSegments(v.Segments)
}

func (v TranscribeStageResult) Validate() error {
	if v.Version != TranscribeStageVersion || !Stage(v.Stage) {
		return fmt.Errorf("unsupported transcription stage result version or stage: %q/%q", v.Version, v.Stage)
	}
	if v.Error != nil {
		if v.Error.Code == "" || v.Error.Message == "" || len(v.Artifacts) != 0 || len(v.Segments) != 0 {
			return fmt.Errorf("failed stage requires code/message and no success artifacts or segments")
		}
		return nil
	}
	if v.Stage == "infer" && len(v.Segments) == 0 {
		return fmt.Errorf("infer result requires transcript segments")
	}
	if v.Stage != "infer" && len(v.Artifacts) == 0 {
		return fmt.Errorf("%s result requires an artifact", v.Stage)
	}
	for _, a := range v.Artifacts {
		if a.Path == "" {
			return fmt.Errorf("artifact path is empty")
		}
	}
	return ValidateSegments(v.Segments)
}

func ValidateSegments(segments []TranscribeSegment) error {
	var lastEnd int64
	for i, s := range segments {
		if s.Text == "" || s.StartMS < 0 || s.EndMS <= s.StartMS || (i > 0 && s.StartMS < lastEnd) {
			return fmt.Errorf("invalid or out-of-order transcript segment %d", i)
		}
		lastEnd = s.EndMS
	}
	return nil
}

// Stage reports whether a stage name belongs to the v1 contract.
func Stage(s string) bool {
	switch s {
	case "acquire", "decode", "infer", "export":
		return true
	}
	return false
}
