package transcribeseed

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	c "github.com/qiangli/bashy/internal/transcribecontract"
)

func timestamp(ms int64, separator string) string {
	return fmt.Sprintf("%02d:%02d:%02d%s%03d", ms/3600000, ms/60000%60, ms/1000%60, separator, ms%1000)
}

// Render writes text, never subtitle markup from model-generated speech.
func Render(ctx context.Context, w io.Writer, format string, segments []c.TranscribeSegment) error {
	if err := c.ValidateSegments(segments); err != nil {
		return err
	}
	if len(segments) == 0 {
		return fmt.Errorf("empty transcript")
	}
	if format != "txt" && format != "srt" && format != "vtt" {
		return fmt.Errorf("unsupported transcript format %q", format)
	}
	write := func(s string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		_, err := io.WriteString(w, s)
		return err
	}
	if format == "vtt" {
		if err := write("WEBVTT\n\n"); err != nil {
			return err
		}
	}
	for i, s := range segments {
		if !utf8.ValidString(s.Text) || strings.TrimSpace(s.Text) == "" || strings.ContainsRune(s.Text, 0) {
			return fmt.Errorf("invalid transcript text at segment %d", i)
		}
		text := strings.Join(strings.Fields(s.Text), " ")
		var line string
		switch format {
		case "txt":
			line = text + "\n"
		case "srt", "vtt":
			// Encode literal cue text; no injected markup, timing line or blank cue.
			text = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(text)
			sep := "."
			if format == "srt" {
				sep = ","
			}
			line = fmt.Sprintf("%d\n%s --> %s\n%s\n\n", i+1, timestamp(s.StartMS, sep), timestamp(s.EndMS, sep), text)
		}
		if err := write(line); err != nil {
			return err
		}
	}
	return nil
}

func export(ctx context.Context, in c.TranscribeStageInput) (r c.TranscribeStageResult, err error) {
	r = result(in)
	if in.Output == "" {
		return r, failure("invalid_request", "export requires an explicit output path")
	}
	path, err := filepath.Abs(in.Output)
	if err != nil {
		return r, err
	}
	if _, err = os.Lstat(path); err == nil {
		return r, failure("output_exists", "refusing to overwrite existing transcript output")
	} else if !os.IsNotExist(err) {
		return r, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".transcribe-export-*")
	if err != nil {
		return r, err
	}
	defer os.Remove(tmp.Name())
	err = Render(ctx, tmp, strings.ToLower(in.Format), in.Segments)
	if err == nil {
		err = tmp.Sync()
	}
	ce := tmp.Close()
	if err == nil {
		err = ce
	}
	if err != nil {
		return r, err
	}
	h, err := digest(ctx, tmp.Name())
	if err != nil {
		return r, err
	}
	if err = ctx.Err(); err != nil {
		return r, err
	}
	// Atomic, exclusive publication: a concurrent creator cannot be overwritten.
	// Temp and destination are siblings, so the link is on the same filesystem.
	if err = os.Link(tmp.Name(), path); err != nil {
		return r, err
	}
	mediaType := map[string]string{"txt": "text/plain", "srt": "application/x-subrip", "vtt": "text/vtt"}[strings.ToLower(in.Format)]
	r.Artifacts = []c.TranscribeArtifact{{Path: path, MediaType: mediaType, SHA256: h}}
	return r, nil
}
