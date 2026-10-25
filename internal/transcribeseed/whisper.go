package transcribeseed

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	c "github.com/qiangli/bashy/internal/transcribecontract"
	"github.com/qiangli/yoke/pkg/binmgr"
)

func modelName(s string) string {
	if s == "" || s == "default" {
		return "base"
	}
	return s
}

// SetupModel is the explicit network-capable setup operation. Inference never
// calls EnsureWhisperModel, so absent/corrupt models fail offline as well.
func SetupModel(ctx context.Context, name string) (string, error) {
	return binmgr.EnsureWhisperModel(ctx, modelName(name))
}

func cachedModel(ctx context.Context, name string) (string, error) {
	info, err := binmgr.WhisperModel(modelName(name))
	if err != nil {
		return "", err
	}
	dir, err := binmgr.WhisperModelDir()
	if err != nil {
		return "", err
	}
	p := filepath.Join(dir, "ggml-"+info.Name+".bin")
	fi, err := os.Stat(p)
	if err != nil {
		return "", failure("missing_model", "model is not cached; run the adapter model setup operation explicitly")
	}
	if !fi.Mode().IsRegular() || fi.Size() != info.Size {
		return "", failure("invalid_model", "cached model size mismatch; run model setup to repair")
	}
	h, err := digest(ctx, p)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(h, info.SHA256) {
		return "", failure("invalid_model", "cached model checksum mismatch; run model setup to repair")
	}
	return p, nil
}

func infer(ctx context.Context, in c.TranscribeStageInput, cfg Config) (c.TranscribeStageResult, error) {
	r := result(in)
	p, err := inputFile(in)
	if err != nil {
		return r, err
	}
	// Validate actual audio, rather than trusting media metadata supplied by an adapter.
	audio, err := decoded(ctx, in, p)
	if err != nil {
		return r, err
	}
	r.Media = audio.Media
	name := modelName(in.Model)
	lang := strings.ToLower(in.Language)
	if lang == "" {
		lang = "auto"
	}
	lang = strings.SplitN(lang, "-", 2)[0]
	info, err := binmgr.WhisperModel(name)
	if err != nil {
		return r, err
	}
	if strings.HasSuffix(info.Name, ".en") && lang != "en" {
		return r, failure("invalid_model", "automatic/non-English language requires a multilingual model")
	}
	model, err := cachedModel(ctx, name)
	if err != nil {
		return r, err
	}
	var bin string
	if cfg.WhisperPath != "" {
		bin, err = verifiedExecutable(ctx, cfg.WhisperPath, cfg.WhisperSHA256)
	} else {
		// Do not download during inference, even when an admitted manifest lands.
		tool := binmgr.WhisperCppTool()
		_, ok := tool.Assets[binmgr.Platform()]
		if !ok {
			return r, failure("missing_tool", "no admitted whisper.cpp artifact for this platform; complete dependency setup first")
		}
		if tool.Sigstore == nil || !tool.RequireCommittedPin {
			return r, failure("unverified_tool", "managed whisper.cpp requires a signed, committed artifact pin")
		}
		var cached bool
		bin, cached = binmgr.Cached(tool)
		if !cached {
			return r, failure("missing_tool", "whisper.cpp is not cached; complete dependency setup first")
		}
		// Archive digests describe an archive, not the extracted executable.
		// binmgr owns verification of managed cache entries; local override pins
		// above describe executable bytes and are verified on every invocation.
	}
	if err != nil {
		return r, err
	}
	dir, err := workspace(in)
	if err != nil {
		return r, err
	}
	defer os.RemoveAll(dir)
	base := filepath.Join(dir, "transcript")
	err = run(ctx, bin, "--model", model, "--file", p, "--language", lang, "--no-gpu", "--output-json", "--output-file", base)
	if err != nil {
		return r, err
	}
	fi, err := os.Stat(base + ".json")
	if err != nil || !fi.Mode().IsRegular() {
		return r, failure("invalid_transcript", "whisper.cpp produced no regular JSON transcript")
	}
	if fi.Size() > 64<<20 {
		return r, failure("invalid_transcript", "whisper.cpp JSON transcript exceeds 64 MiB")
	}
	b, err := os.ReadFile(base + ".json")
	if err != nil {
		return r, fmt.Errorf("whisper.cpp produced no JSON transcript: %w", err)
	}
	if !utf8.Valid(b) {
		return r, failure("invalid_transcript", "whisper.cpp emitted invalid UTF-8")
	}
	var doc struct {
		Result struct {
			Language string `json:"language"`
		} `json:"result"`
		Transcription []struct {
			Offsets struct {
				From int64 `json:"from"`
				To   int64 `json:"to"`
			} `json:"offsets"`
			Text string `json:"text"`
		} `json:"transcription"`
	}
	if err = json.Unmarshal(b, &doc); err != nil {
		return r, err
	}
	if doc.Result.Language == "" || doc.Result.Language == "auto" {
		return r, failure("invalid_transcript", "whisper.cpp did not report detected language")
	}
	r.Media.Language = doc.Result.Language
	for _, s := range doc.Transcription {
		text := strings.TrimSpace(s.Text)
		if text == "" {
			continue
		}
		if s.Offsets.To > r.Media.DurationMS {
			return r, failure("invalid_transcript", "transcript extends beyond audio duration")
		}
		r.Segments = append(r.Segments, c.TranscribeSegment{StartMS: s.Offsets.From, EndMS: s.Offsets.To, Text: text})
	}
	if err = r.Validate(); err != nil {
		return r, failure("invalid_transcript", err.Error())
	}
	if err = ctx.Err(); err != nil {
		return r, err
	}
	return r, nil
}
