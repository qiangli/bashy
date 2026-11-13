package apirods

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var buildOnce sync.Once
var buildBin string
var buildErr error

func root(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(file)))
}
func binary(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX fixture scripts")
	}
	buildOnce.Do(func() {
		d, e := os.MkdirTemp("", "s627-api-rods-*")
		if e != nil {
			buildErr = e
			return
		}
		buildBin = filepath.Join(d, "bashy")
		c := exec.Command("go", "build", "-o", buildBin, "./cmd/bashy")
		c.Dir = root(t)
		if w := filepath.Join(filepath.Dir(c.Dir), "go.work"); exists(w) {
			c.Env = append(os.Environ(), "GOWORK="+w)
		}
		if out, e := c.CombinedOutput(); e != nil {
			buildErr = &fixtureError{e, string(out)}
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return buildBin
}

type fixtureError struct {
	err error
	out string
}

func (e *fixtureError) Error() string { return e.err.Error() + "\n" + e.out }
func exists(p string) bool            { _, e := os.Stat(p); return e == nil }

// These commands replace the HTTP command boundary and artifact fetch. No
// transport to a vendor, model, browser or local server is opened.
const fakeBashy = `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_LOG"
last=""; for a in "$@"; do last=$a; done
case "$*" in
  *replicate-media:prediction*) cat <<'JSON'
{"outcome":"success","text":"{\"id\":\"pred1\",\"status\":\"succeeded\",\"output\":\"https://replicate.delivery/fixture/media\"}"}
JSON
    ;;
  *replicate-media:*) cp "$last" "$FAKE_CAPTURE"; cat <<'JSON'
{"outcome":"success","text":"{\"id\":\"pred1\",\"status\":\"starting\"}"}
JSON
    ;;
  *) exit 30 ;;
esac
`
const fakeCurl = `#!/bin/sh
out=""; while [ $# -gt 0 ]; do case $1 in --output) out=$2; shift 2;; *) shift;; esac; done
[ -n "$out" ] || exit 31
cp "$FAKE_MEDIA" "$out"
`

type fixture struct {
	t                                *testing.T
	dir, log, capture, media, script string
	env                              []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d := t.TempDir()
	binDir := filepath.Join(d, "bin")
	if e := os.Mkdir(binDir, 0755); e != nil {
		t.Fatal(e)
	}
	for name, body := range map[string]string{"fake-bashy": fakeBashy, "curl": fakeCurl} {
		if e := os.WriteFile(filepath.Join(binDir, name), []byte(body), 0755); e != nil {
			t.Fatal(e)
		}
	}
	f := &fixture{t: t, dir: d, log: filepath.Join(d, "calls"), capture: filepath.Join(d, "input.json"), media: filepath.Join(d, "media"), script: filepath.Join(root(t), "examples", "api-rods", "replicate-media.bsh")}
	f.env = append(os.Environ(), "BASHY_API_BASHY_BIN="+filepath.Join(binDir, "fake-bashy"), "BASHY_API_POLL_INTERVAL=0", "BASHY_API_MAX_POLLS=2", "BASHY_HINTS=off", "FAKE_LOG="+f.log, "FAKE_CAPTURE="+f.capture, "FAKE_MEDIA="+f.media, "BASHY_SVC_OUT="+filepath.Join(d, "out"), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return f
}
func (f *fixture) run(kind string, extra []string, args ...string) (string, int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	argv := append([]string{"--bashsharp", f.script, kind}, args...)
	c := exec.CommandContext(ctx, binary(f.t), argv...)
	c.Env = append(append([]string{}, f.env...), extra...)
	b, e := c.CombinedOutput()
	if e == nil {
		return string(b), 0
	}
	if x, ok := e.(*exec.ExitError); ok {
		return string(b), x.ExitCode()
	}
	f.t.Fatal(e)
	return "", 0
}
func (f *fixture) request(v map[string]any) string {
	f.t.Helper()
	v["schema_version"] = "bashy-svc-request-v1"
	b, e := json.Marshal(v)
	if e != nil {
		f.t.Fatal(e)
	}
	p := filepath.Join(f.dir, "request.json")
	if e := os.WriteFile(p, b, 0600); e != nil {
		f.t.Fatal(e)
	}
	return p
}

func TestRefusalAndDryRunNeverReachHTTP(t *testing.T) {
	f := newFixture(t)
	req := f.request(map[string]any{"prompt": "ballad", "lyrics": "hello"})
	if out, code := f.run("song", nil, req); code != 126 || !strings.Contains(out, "requires --yes") {
		t.Fatalf("refusal: %d %s", code, out)
	}
	if out, code := f.run("song", nil, "--dry-run"); code != 0 || !strings.Contains(out, `"cost_class":"api"`) {
		t.Fatalf("dry-run: %d %s", code, out)
	}
	if exists(f.log) {
		t.Fatal("refusal or dry run reached HTTP boundary")
	}
}

func TestFixturePredictionsProduceTypedArtifacts(t *testing.T) {
	for _, tc := range []struct {
		kind, role   string
		req          map[string]any
		command, ext string
	}{
		{"song", "song", map[string]any{"prompt": "soft ballad", "lyrics": "Hello"}, "replicate-media:song", ".wav"},
		{"image", "image.generate", map[string]any{"prompt": "portrait", "count": 1}, "replicate-media:image", ".png"},
		{"image", "image.edit", map[string]any{"prompt": "same person, blue light", "image": "face.png"}, "replicate-media:image-edit", ".png"},
		{"animate", "animate", map[string]any{"image": "face.png", "audio": "take.wav", "duration_s": 4}, "replicate-media:animate", ".mp4"},
	} {
		t.Run(tc.role, func(t *testing.T) {
			f := newFixture(t)
			if _, err := exec.LookPath("ffprobe"); err != nil {
				t.Skip("ffprobe required for media verification")
			}
			fixture := map[string]string{
				"song":    filepath.Join(root(t), "examples", "transcribe", "fixtures", "spanish-proverb.wav"),
				"image":   filepath.Join(root(t), "examples", "media", "sample", "slides", "01.png"),
				"animate": filepath.Join(root(t), "examples", "api-rods", "fixtures", "short.mp4"),
			}[tc.kind]
			f.media = fixture
			f.env = append(f.env, "FAKE_MEDIA="+fixture)
			if tc.kind == "animate" || tc.role == "image.edit" {
				paths := []string{"face.png"}
				if tc.kind == "animate" { paths = append(paths, "take.wav") }
				for _, p := range paths {
					if e := os.WriteFile(filepath.Join(f.dir, p), []byte("media fixture"), 0600); e != nil {
						t.Fatal(e)
					}
				}
			}
			req := f.request(tc.req)
			out, code := f.run(tc.kind, []string{"BASHY_SVC_YES=1", "BASHY_SVC_ROLE=" + tc.role}, req)
			if code != 0 {
				t.Fatalf("fixture: %d %s", code, out)
			}
			var result struct {
				Schema  string `json:"schema_version"`
				Outputs []struct {
					Path  string `json:"path"`
					Media string `json:"media_type"`
				} `json:"outputs"`
			}
			if e := json.Unmarshal([]byte(out), &result); e != nil {
				t.Fatalf("result %s: %v", out, e)
			}
			if result.Schema != "bashy-svc-rod-v1" || len(result.Outputs) != 1 || filepath.Ext(result.Outputs[0].Path) != tc.ext || !exists(result.Outputs[0].Path) {
				t.Fatalf("result: %+v", result)
			}
			calls, _ := os.ReadFile(f.log)
			if !strings.Contains(string(calls), tc.command) || !strings.Contains(string(calls), "replicate-media:prediction") {
				t.Fatalf("calls: %s", calls)
			}
			input, _ := os.ReadFile(f.capture)
			if !json.Valid(input) {
				t.Fatalf("input is invalid JSON: %s", input)
			}
		})
	}
}
