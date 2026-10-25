// Package transcribeseed implements ordinary, replaceable transcription adapters.
// It is deliberately not imported by the shell's dispatcher or initialization.
package transcribeseed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	c "github.com/qiangli/bashy/internal/transcribecontract"
	"github.com/qiangli/yoke/pkg/binmgr"
)

// Config is deployment configuration, outside the untrusted stage request.
// A configured external executable must have an independently reviewed digest.
// An empty WhisperPath uses binmgr's admitted tool manifest. FFmpeg has no
// automatic discovery or installation: its deployment requires decoder policy.
type Config struct {
	WhisperPath   string `json:"whisper_path"`
	WhisperSHA256 string `json:"whisper_sha256"`
	FFmpegPath    string `json:"ffmpeg_path"`
	FFmpegSHA256  string `json:"ffmpeg_sha256"`
}

type stageFailure struct {
	code string
	err  error
}

func (e *stageFailure) Error() string    { return e.err.Error() }
func (e *stageFailure) Unwrap() error    { return e.err }
func failure(code, message string) error { return &stageFailure{code, errors.New(message)} }

// Execute returns a structured error envelope and nonzero status on failure.
// It never emits subprocess stdout as a stage result.
func Execute(ctx context.Context, adapter string, in c.TranscribeStageInput, cfg Config) (out c.TranscribeStageResult, status int) {
	out = c.TranscribeStageResult{Version: c.TranscribeStageVersion, Stage: in.Stage}
	var err error
	if err = in.Validate(); err == nil {
		err = ctx.Err()
	}
	if err == nil {
		switch {
		case adapter == "local" && in.Stage == "acquire":
			out, err = acquire(ctx, in)
		case adapter == "wav" && in.Stage == "decode":
			out, err = decodeWAV(ctx, in)
		case adapter == "ffmpeg" && in.Stage == "decode":
			out, err = decodeFFmpeg(ctx, in, cfg)
		case adapter == "whisper" && in.Stage == "infer":
			out, err = infer(ctx, in, cfg)
		case adapter == "export" && in.Stage == "export":
			out, err = export(ctx, in)
		default:
			err = failure("invalid_request", "adapter does not implement requested stage")
		}
	}
	if err == nil {
		err = out.Validate()
	}
	if err == nil {
		return out, 0
	}
	code := "stage_failed"
	var se *stageFailure
	if errors.As(err, &se) {
		code = se.code
	}
	status = 1
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		code, status = "cancelled", 130
	}
	return c.TranscribeStageResult{Version: c.TranscribeStageVersion, Stage: in.Stage, Error: &c.TranscribeStageError{Code: code, Message: err.Error()}}, status
}

// ReadRequest accepts unknown optional fields, but rejects trailing JSON.
func ReadRequest(r io.Reader) (c.TranscribeStageInput, error) {
	var in c.TranscribeStageInput
	const limit = 16 << 20
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return in, err
	}
	if len(b) > limit {
		return in, errors.New("stage request exceeds 16 MiB")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	if err := d.Decode(&in); err != nil {
		return in, err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return in, errors.New("request must contain exactly one JSON object")
	}
	return in, in.Validate()
}

func result(in c.TranscribeStageInput) c.TranscribeStageResult {
	r := c.TranscribeStageResult{Version: c.TranscribeStageVersion, Stage: in.Stage}
	if in.Media != nil {
		m := *in.Media
		r.Media = &m
	}
	return r
}

func regular(path string) (string, error) {
	p, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	fi, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if !fi.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", p)
	}
	return p, nil
}

type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

func digest(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, contextReader{ctx, f}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), ctx.Err()
}

func verifiedExecutable(ctx context.Context, path, want string) (string, error) {
	if path == "" {
		return "", failure("missing_tool", "external tool is not configured; install an admitted artifact and configure its path and SHA-256")
	}
	if b, err := hex.DecodeString(want); err != nil || len(b) != sha256.Size {
		return "", failure("unverified_tool", "external tool requires a reviewed SHA-256 digest")
	}
	p, err := regular(path)
	if err != nil {
		return "", err
	}
	got, err := digest(ctx, p)
	if err != nil {
		return "", err
	}
	if !strings.EqualFold(got, want) {
		return "", failure("unverified_tool", "external tool checksum mismatch")
	}
	return p, nil
}

func acquire(ctx context.Context, in c.TranscribeStageInput) (c.TranscribeStageResult, error) {
	r := result(in)
	p, err := regular(in.Input)
	if err != nil {
		return r, err
	}
	h, err := digest(ctx, p)
	if err != nil {
		return r, err
	}
	r.Artifacts = []c.TranscribeArtifact{{Path: p, SHA256: h}}
	r.Media = &c.TranscribeMedia{Source: p}
	return r, nil
}

func workspace(in c.TranscribeStageInput) (string, error) {
	if in.WorkDir == "" {
		return "", failure("invalid_request", "work_dir is required; caller owns cleanup of successful decode artifacts")
	}
	p, err := filepath.Abs(in.WorkDir)
	if err != nil {
		return "", err
	}
	return os.MkdirTemp(p, "transcribe-")
}

// Only one artifact is accepted by these seeds; selecting an audio track or
// interpreting a collection is the responsibility of a different adapter.
func inputFile(in c.TranscribeStageInput) (string, error) {
	if len(in.Artifacts) != 1 {
		return "", failure("invalid_request", "seed adapter expects exactly one artifact")
	}
	return regular(in.Artifacts[0].Path)
}

type cappedLog struct{ data []byte }

func (b *cappedLog) Write(p []byte) (int, error) {
	n := len(p)
	left := 8192 - len(b.data)
	if left > len(p) {
		left = len(p)
	}
	if left > 0 {
		b.data = append(b.data, p[:left]...)
	}
	return n, nil
}
func run(ctx context.Context, bin string, args ...string) error {
	cmd := binmgr.Command(ctx, bin, args...)
	cmd.WaitDelay = 2 * time.Second
	var log cappedLog
	cmd.Stdout, cmd.Stderr = io.Discard, &log
	err := cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("tool failed: %w: %s", err, strings.TrimSpace(string(log.data)))
	}
	return nil
}
