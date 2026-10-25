package transcribeseed

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type noNetwork struct{}

func (noNetwork) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("native inference attempted an HTTP request after setup")
}

// TestNativeTranscription is explicitly opt-in. Run only on a claimed runtime
// host with reviewed executable/model bytes and independently sourced speech.
// This forbids Go HTTP requests but does not disable the host/process network;
// acceptance additionally requires an externally enforced network-off repeat.
func TestNativeTranscription(t *testing.T) {
	path := os.Getenv("BASHY_TRANSCRIBE_NATIVE_MANIFEST")
	if path == "" {
		t.Skip("native evidence requires an explicit fixture/dependency manifest on an authorized runtime host")
	}
	var manifest struct {
		OS       string `json:"os"`
		Arch     string `json:"arch"`
		Config   Config `json:"config"`
		Model    string `json:"model"`
		Fixtures []struct {
			Path     string `json:"path"`
			Kind     string `json:"kind"`
			Decoder  string `json:"decoder"`
			Language string `json:"language"`
			Contains string `json:"contains"`
		} `json:"fixtures"`
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.OS != runtime.GOOS || manifest.Arch != runtime.GOARCH {
		t.Fatalf("manifest target %s/%s != process %s/%s; verify native hardware separately", manifest.OS, manifest.Arch, runtime.GOOS, runtime.GOARCH)
	}
	if manifest.Config.WhisperPath == "" || manifest.Config.WhisperSHA256 == "" {
		t.Fatal("native gate requires explicit reviewed whisper executable and digest")
	}
	kinds, languages := map[string]bool{}, map[string]bool{}
	for _, f := range manifest.Fixtures {
		if f.Kind != "audio" && f.Kind != "video" {
			t.Fatal("fixtures must declare audio/video kind")
		}
		if f.Decoder != "wav" && f.Decoder != "ffmpeg" {
			t.Fatal("fixture decoder must be wav or ffmpeg")
		}
		if f.Language == "" || f.Language == "auto" || strings.TrimSpace(f.Contains) == "" {
			t.Fatal("each fixture requires a reviewed language and reference excerpt")
		}
		kinds[f.Kind], languages[f.Language] = true, true
	}
	if !kinds["audio"] || !kinds["video"] || len(languages) < 2 {
		t.Fatal("native gate requires audio, video and at least two languages")
	}
	oldTransport := http.DefaultTransport
	http.DefaultTransport = noNetwork{}
	defer func() { http.DefaultTransport = oldTransport }()
	for i, f := range manifest.Fixtures {
		t.Run(fmt.Sprintf("%d-%s-%s", i, f.Kind, f.Language), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			work := t.TempDir()
			in := request("acquire")
			in.Input = f.Path
			in.WorkDir = work
			acq, status := Execute(ctx, "local", in, manifest.Config)
			if status != 0 {
				t.Fatal(acq.Error)
			}
			in = request("decode")
			in.Artifacts = acq.Artifacts
			in.Media = acq.Media
			in.WorkDir = work
			decoded, status := Execute(ctx, f.Decoder, in, manifest.Config)
			if status != 0 {
				t.Fatal(decoded.Error)
			}
			in = request("infer")
			in.Artifacts = decoded.Artifacts
			in.Media = decoded.Media
			in.WorkDir = work
			in.Model = manifest.Model
			in.Language = "auto"
			started := time.Now()
			transcript, status := Execute(ctx, "whisper", in, manifest.Config)
			if status != 0 {
				t.Fatal(transcript.Error)
			}
			elapsed := time.Since(started)
			if transcript.Media.Language != f.Language {
				t.Fatalf("detected language %q != reviewed %q", transcript.Media.Language, f.Language)
			}
			var text strings.Builder
			for _, s := range transcript.Segments {
				if s.EndMS > decoded.Media.DurationMS {
					t.Fatal("subtitle exceeds media duration")
				}
				text.WriteString(s.Text)
				text.WriteByte(' ')
			}
			if !strings.Contains(strings.ToLower(text.String()), strings.ToLower(f.Contains)) {
				t.Fatal("transcript does not contain reviewed reference excerpt")
			}
			for _, format := range []string{"txt", "srt", "vtt"} {
				e := request("export")
				e.Segments = transcript.Segments
				e.Media = transcript.Media
				e.Format = format
				e.Output = filepath.Join(t.TempDir(), "transcript."+format)
				r, status := Execute(ctx, "export", e, manifest.Config)
				if status != 0 {
					t.Fatal(r.Error)
				}
				b, err := os.ReadFile(r.Artifacts[0].Path)
				if err != nil || len(b) == 0 {
					t.Fatal("missing/empty export", err)
				}
				t.Logf("format=%s sha256=%s bytes=%d", format, r.Artifacts[0].SHA256, len(b))
			}
			// Caller cleanup owns successful decode artifacts, never the input/model.
			if err := os.RemoveAll(work); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(f.Path); err != nil {
				t.Fatal("original fixture removed", err)
			}
			t.Logf("process=%s/%s media_sha256=%s duration_ms=%d detected=%s segments=%d inference=%s", runtime.GOOS, runtime.GOARCH, acq.Artifacts[0].SHA256, decoded.Media.DurationMS, transcript.Media.Language, len(transcript.Segments), elapsed)
		})
	}
}
