package transcribeseed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	c "github.com/qiangli/bashy/internal/transcribecontract"
	"github.com/qiangli/yoke/pkg/binmgr"
)

// The subprocess fixture implements argv/output failure cases only. It is not
// whisper.cpp and these tests are never native transcription acceptance.
func TestMain(m *testing.M) {
	if mode := os.Getenv("TRANSCRIBE_TEST_TOOL"); mode != "" {
		args := os.Args[1:]
		get := func(flag string) string {
			for i, a := range args {
				if a == flag && i+1 < len(args) {
					return args[i+1]
				}
			}
			return ""
		}
		if p := os.Getenv("TRANSCRIBE_TEST_ARGV"); p != "" {
			b, _ := json.Marshal(args)
			_ = os.WriteFile(p, b, 0600)
		}
		base := get("--output-file")
		if mode == "decoder" {
			data, _ := os.ReadFile(os.Getenv("TRANSCRIBE_TEST_WAV"))
			_ = os.WriteFile(args[len(args)-1], data, 0600)
			os.Exit(0)
		}
		if mode == "decoder-fail" {
			_ = os.WriteFile(args[len(args)-1], []byte("partial"), 0600)
			os.Exit(1)
		}
		if mode == "empty" {
			os.Exit(0)
		}
		_ = os.WriteFile(base+".json", []byte("partial"), 0600)
		if mode == "cancel" {
			_ = os.WriteFile(os.Getenv("TRANSCRIBE_TEST_READY"), nil, 0600)
			time.Sleep(time.Minute)
			os.Exit(0)
		}
		if mode == "fail" {
			os.Exit(1)
		}
		if mode == "invalid-json" {
			os.Exit(0)
		}
		data := `{"result":{"language":"es"},"transcription":[{"offsets":{"from":0,"to":500},"text":" Hola mundo. "},{"offsets":{"from":500,"to":1000},"text":" 你好，世界。"}]}`
		if mode == "invalid-time" {
			data = `{"result":{"language":"en"},"transcription":[{"offsets":{"from":0,"to":2000},"text":"too late"}]}`
		}
		_ = os.WriteFile(base+".json", []byte(data), 0600)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func request(stage string) c.TranscribeStageInput {
	return c.TranscribeStageInput{Version: c.TranscribeStageVersion, Stage: stage}
}
func wavFixture(t *testing.T, dir string, rate, channels int) string {
	t.Helper()
	frames := rate
	b := make([]byte, 44+frames*channels*2)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], uint32(len(b)-8))
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], uint16(channels))
	binary.LittleEndian.PutUint32(b[24:], uint32(rate))
	binary.LittleEndian.PutUint32(b[28:], uint32(rate*channels*2))
	binary.LittleEndian.PutUint16(b[32:], uint16(channels*2))
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], uint32(frames*channels*2))
	for i := 44; i < len(b); i += 2 {
		binary.LittleEndian.PutUint16(b[i:], 1000)
	}
	p := filepath.Join(dir, "audio ; space ' 日本.wav")
	if err := os.WriteFile(p, b, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func failResult(t *testing.T, r c.TranscribeStageResult, status int) {
	t.Helper()
	if status == 0 || r.Error == nil || len(r.Artifacts) != 0 || len(r.Segments) != 0 {
		t.Fatalf("misleading failure: status=%d %+v", status, r)
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}
func emptyDir(t *testing.T, path string) {
	t.Helper()
	items, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("owned artifacts remain in %s: %v", path, items)
	}
}

func TestLocalAndNormalizePaths(t *testing.T) {
	in := request("acquire")
	in.Input = wavFixture(t, t.TempDir(), 44100, 2)
	r, status := Execute(context.Background(), "local", in, Config{})
	if status != 0 {
		t.Fatal(r.Error)
	}
	if r.Artifacts[0].Path != in.Input || len(r.Artifacts[0].SHA256) != 64 {
		t.Fatal(r)
	}
	d := request("decode")
	d.Artifacts = r.Artifacts
	d.Media = r.Media
	d.WorkDir = t.TempDir()
	r, status = Execute(context.Background(), "wav", d, Config{})
	if status != 0 {
		t.Fatal(r.Error)
	}
	if r.Media.SampleRateHz != 16000 || r.Media.Channels != 1 || r.Media.DurationMS != 1000 || r.Media.Source != in.Input {
		t.Fatal(r.Media)
	}
	b, err := os.ReadFile(r.Artifacts[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != 32044 || binary.LittleEndian.Uint16(b[44:]) != 1000 {
		t.Fatalf("normalization did not preserve signal: size=%d", len(b))
	}
	if _, err := os.Stat(in.Input); err != nil {
		t.Fatal("input removed", err)
	}
}

func TestInvalidInputsLeaveNoTemporaryFiles(t *testing.T) {
	for _, kind := range []string{"invalid", "truncated", "no-audio", "missing", "directory"} {
		t.Run(kind, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "input")
			switch kind {
			case "invalid":
				os.WriteFile(p, []byte("not media"), 0600)
			case "truncated":
				p = wavFixture(t, t.TempDir(), 16000, 1)
				os.Truncate(p, 50)
			case "no-audio":
				p = wavFixture(t, t.TempDir(), 16000, 1)
				b, _ := os.ReadFile(p)
				b = b[:44]
				binary.LittleEndian.PutUint32(b[4:], 36)
				binary.LittleEndian.PutUint32(b[40:], 0)
				os.WriteFile(p, b, 0600)
			case "directory":
				os.Mkdir(p, 0700)
			}
			in := request("decode")
			in.Artifacts = []c.TranscribeArtifact{{Path: p}}
			in.WorkDir = t.TempDir()
			r, status := Execute(context.Background(), "wav", in, Config{})
			failResult(t, r, status)
			emptyDir(t, in.WorkDir)
		})
	}
}

func modelFixture(t *testing.T) string {
	t.Helper()
	t.Setenv("BASHY_BIN_CACHE", filepath.Join(t.TempDir(), "bin"))
	data := []byte("implementation test model, not inference weights")
	h := sha256.Sum256(data)
	name := "s503-test-model"
	binmgr.WhisperModels[name] = binmgr.WhisperModelInfo{Name: name, SHA256: hex.EncodeToString(h[:]), Size: int64(len(data))}
	t.Cleanup(func() { delete(binmgr.WhisperModels, name) })
	dir, err := binmgr.WhisperModelDir()
	if err != nil {
		t.Fatal(err)
	}
	os.MkdirAll(dir, 0700)
	os.WriteFile(filepath.Join(dir, "ggml-"+name+".bin"), data, 0600)
	return name
}
func toolFixture(t *testing.T) Config {
	t.Helper()
	p, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	h, err := digest(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return Config{WhisperPath: p, WhisperSHA256: h, FFmpegPath: p, FFmpegSHA256: h}
}
func inferRequest(t *testing.T) c.TranscribeStageInput {
	t.Helper()
	in := request("infer")
	in.Model = modelFixture(t)
	in.Language = "auto"
	in.WorkDir = t.TempDir()
	in.Artifacts = []c.TranscribeArtifact{{Path: wavFixture(t, t.TempDir(), 16000, 1)}}
	return in
}

func TestWhisperCPUAutoAndExports(t *testing.T) {
	in := inferRequest(t)
	cfg := toolFixture(t)
	t.Setenv("TRANSCRIBE_TEST_TOOL", "success")
	argv := filepath.Join(t.TempDir(), "argv.json")
	t.Setenv("TRANSCRIBE_TEST_ARGV", argv)
	r, status := Execute(context.Background(), "whisper", in, cfg)
	if status != 0 {
		t.Fatal(r.Error)
	}
	if r.Media.Language != "es" || len(r.Segments) != 2 || r.Segments[1].Text != "你好，世界。" {
		t.Fatal(r)
	}
	emptyDir(t, in.WorkDir)
	b, _ := os.ReadFile(argv)
	var args []string
	json.Unmarshal(b, &args)
	if !reflect.DeepEqual(args[:7], []string{"--model", func() string { p, _ := cachedModel(context.Background(), in.Model); return p }(), "--file", in.Artifacts[0].Path, "--language", "auto", "--no-gpu"}) {
		t.Fatalf("CPU/auto argv: %v", args)
	}
	for _, format := range []string{"txt", "srt", "vtt"} {
		t.Run(format, func(t *testing.T) {
			e := request("export")
			e.Segments = r.Segments
			e.Format = format
			e.Output = filepath.Join(t.TempDir(), "speech with spaces."+format)
			out, status := Execute(context.Background(), "export", e, Config{})
			if status != 0 {
				t.Fatal(out.Error)
			}
			b, _ := os.ReadFile(e.Output)
			s := string(b)
			if !strings.Contains(s, "Hola mundo.") || !strings.Contains(s, "你好，世界。") {
				t.Fatal(s)
			}
			if format == "srt" && !strings.Contains(s, "00:00:00,000 --> 00:00:00,500") {
				t.Fatal(s)
			}
			if format == "vtt" && (!strings.HasPrefix(s, "WEBVTT\n\n") || !strings.Contains(s, "00:00:00.500 --> 00:00:01.000")) {
				t.Fatal(s)
			}
			items, _ := os.ReadDir(filepath.Dir(e.Output))
			if len(items) != 1 {
				t.Fatal(items)
			}
		})
	}
}

func TestWhisperFailureAndCancellation(t *testing.T) {
	for _, mode := range []string{"fail", "empty", "invalid-json", "invalid-time", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			in := inferRequest(t)
			cfg := toolFixture(t)
			t.Setenv("TRANSCRIBE_TEST_TOOL", mode)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				ready := filepath.Join(t.TempDir(), "ready")
				t.Setenv("TRANSCRIBE_TEST_READY", ready)
				go func() {
					deadline := time.Now().Add(5 * time.Second)
					for time.Now().Before(deadline) {
						if _, err := os.Stat(ready); err == nil {
							break
						}
						time.Sleep(10 * time.Millisecond)
					}
					cancel()
				}()
			}
			r, status := Execute(ctx, "whisper", in, cfg)
			failResult(t, r, status)
			if mode == "cancel" && (status != 130 || r.Error.Code != "cancelled") {
				t.Fatal(r, status)
			}
			emptyDir(t, in.WorkDir)
		})
	}
}

func TestMissingAndTamperedDependencies(t *testing.T) {
	in := inferRequest(t)
	cfg := toolFixture(t)
	p, _ := cachedModel(context.Background(), in.Model)
	os.WriteFile(p, []byte("tampered"), 0600)
	r, status := Execute(context.Background(), "whisper", in, cfg)
	failResult(t, r, status)
	if r.Error.Code != "invalid_model" {
		t.Fatal(r)
	}
	os.Remove(p)
	r, status = Execute(context.Background(), "whisper", in, cfg)
	failResult(t, r, status)
	if r.Error.Code != "missing_model" {
		t.Fatal(r)
	}
	if _, err := verifiedExecutable(context.Background(), cfg.WhisperPath, strings.Repeat("0", 64)); err == nil {
		t.Fatal("tampered executable accepted")
	}
	d := request("decode")
	d.Artifacts = in.Artifacts
	d.WorkDir = in.WorkDir
	r, status = Execute(context.Background(), "ffmpeg", d, Config{})
	failResult(t, r, status)
	if r.Error.Code != "missing_tool" {
		t.Fatal(r)
	}
	emptyDir(t, in.WorkDir)
}

func TestDecoderArgvAndFailure(t *testing.T) {
	for _, mode := range []string{"decoder", "decoder-fail"} {
		t.Run(mode, func(t *testing.T) {
			in := request("decode")
			in.Artifacts = []c.TranscribeArtifact{{Path: wavFixture(t, t.TempDir(), 16000, 1)}}
			in.WorkDir = t.TempDir()
			t.Setenv("TRANSCRIBE_TEST_TOOL", mode)
			t.Setenv("TRANSCRIBE_TEST_WAV", in.Artifacts[0].Path)
			argv := filepath.Join(t.TempDir(), "argv.json")
			t.Setenv("TRANSCRIBE_TEST_ARGV", argv)
			r, status := Execute(context.Background(), "ffmpeg", in, toolFixture(t))
			if mode == "decoder-fail" {
				failResult(t, r, status)
				emptyDir(t, in.WorkDir)
				return
			}
			if status != 0 {
				t.Fatal(r.Error)
			}
			b, _ := os.ReadFile(argv)
			var args []string
			json.Unmarshal(b, &args)
			joined := strings.Join(args, "|")
			if !strings.Contains(joined, "-protocol_whitelist|file|-i|"+in.Artifacts[0].Path+"|-map|0:a:0|") {
				t.Fatal(args)
			}
		})
	}
}

func TestSetupVerifiedThenOffline(t *testing.T) {
	name := modelFixture(t)
	info := binmgr.WhisperModels[name]
	p, _ := cachedModel(context.Background(), name)
	data, _ := os.ReadFile(p)
	os.Remove(p)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.Write(data) }))
	info.URL = srv.URL
	binmgr.WhisperModels[name] = info
	p, err := SetupModel(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	srv.Close()
	if _, err := cachedModel(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	if _, err := SetupModel(context.Background(), name); err != nil {
		t.Fatal("offline cache", err)
	}
	if hits.Load() != 1 {
		t.Fatal("unexpected network requests", hits.Load())
	}
	b, _ := os.ReadFile(p)
	if !bytes.Equal(b, data) {
		t.Fatal("wrong cached model")
	}
}

func TestExportPreservesExistingAndCleansFailures(t *testing.T) {
	for _, mode := range []string{"exists", "format", "cancel", "text"} {
		t.Run(mode, func(t *testing.T) {
			in := request("export")
			in.Output = filepath.Join(t.TempDir(), "speech.txt")
			in.Format = "txt"
			in.Segments = []c.TranscribeSegment{{StartMS: 0, EndMS: 1000, Text: "hello"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "exists":
				os.WriteFile(in.Output, []byte("original"), 0600)
			case "format":
				in.Format = "bad"
			case "cancel":
				cancel()
			case "text":
				in.Segments[0].Text = "\x00"
			}
			r, status := Execute(ctx, "export", in, Config{})
			failResult(t, r, status)
			if mode == "exists" {
				b, _ := os.ReadFile(in.Output)
				if string(b) != "original" {
					t.Fatal("existing output changed")
				}
			} else {
				emptyDir(t, filepath.Dir(in.Output))
			}
		})
	}
}

func TestRenderLongTimesAndLiteralText(t *testing.T) {
	var out bytes.Buffer
	err := Render(context.Background(), &out, "vtt", []c.TranscribeSegment{{StartMS: 360000001, EndMS: 360001002, Text: "<b> &\n\n -->"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "100:00:00.001 --> 100:00:01.002\n&lt;b&gt; &amp; --&gt;") {
		t.Fatal(out.String())
	}
}

func TestRequestWireCompatibility(t *testing.T) {
	for _, stage := range []string{"acquire", "decode", "infer", "export"} {
		body := fmt.Sprintf(`{"version":"bashy-transcribe-stage-v1","stage":%q,"input":"a path","artifacts":[{"path":"audio.wav"}],"segments":[{"start_ms":0,"end_ms":100,"text":"hi"}],"work_dir":"/tmp/owned","future_optional":true}`, stage)
		in, err := ReadRequest(strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		if in.WorkDir != "/tmp/owned" {
			t.Fatal(in)
		}
		if _, err := ReadRequest(strings.NewReader(body + "{}")); err == nil {
			t.Fatal("trailing data accepted")
		}
	}
}

func TestConcurrentExportPublishesOnlyOneResult(t *testing.T) {
	in := request("export")
	in.Output = filepath.Join(t.TempDir(), "shared.txt")
	in.Format = "txt"
	in.Segments = []c.TranscribeSegment{{StartMS: 0, EndMS: 1000, Text: "complete transcript"}}
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, status := Execute(context.Background(), "export", in, Config{})
			if status == 0 {
				successes.Add(1)
			} else {
				if r.Error == nil || len(r.Artifacts) != 0 {
					t.Error("failed contender reported success artifacts")
				}
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("publication successes=%d, want one", successes.Load())
	}
	b, err := os.ReadFile(in.Output)
	if err != nil || string(b) != "complete transcript\n" {
		t.Fatalf("output=%q err=%v", b, err)
	}
	items, err := os.ReadDir(filepath.Dir(in.Output))
	if err != nil || len(items) != 1 {
		t.Fatalf("leftovers=%v err=%v", items, err)
	}
}

func TestOversizedRequestCannotHideTrailingData(t *testing.T) {
	valid := `{"version":"bashy-transcribe-stage-v1","stage":"acquire","input":"file"}`
	input := valid + strings.Repeat(" ", 16<<20) + "{}"
	if _, err := ReadRequest(strings.NewReader(input)); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized request accepted: %v", err)
	}
}
