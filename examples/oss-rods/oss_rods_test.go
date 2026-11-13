// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Focused tests for the Sprint 627 M2 oss rods (song-acestep, image-qwen,
// narrate-local, transcribe-whisperx; see examples/oss-rods/README.md).
//
// VENUE: no model/weight downloads and no live model runs on the operator's
// primary workstation. Every test drives the rod through the real
// service door with a fixture generator behind the rod's _PY hook (same
// pattern as MEDIA_VOICE_PY), so the request, disk-headroom preflight,
// unavailable path and bashy-svc-v1 envelope are all exercised without a
// model. Live renders stay a remaining check for a claimed test host.
package ossrods

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

var (
	buildOnce sync.Once
	buildBin  string
	buildErr  error
	buildDir  string
)

func TestMain(m *testing.M) {
	var err error
	buildDir, err = os.MkdirTemp("", "oss-rods-*")
	if err != nil {
		panic(err)
	}
	code := m.Run()
	os.RemoveAll(buildDir)
	os.Exit(code)
}

// moduleRoot is the bashy repository root (the directory holding cmd/bashy).
func moduleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "cmd", "bashy")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("bashy repository root (cmd/bashy) not found above test file")
		}
		dir = parent
	}
}

func bashyBin(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fixture rods are POSIX paths")
	}
	if bin := os.Getenv("OSS_RODS_BASHY_BIN"); bin != "" {
		return bin
	}
	buildOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		bin := filepath.Join(buildDir, "bashy")
		cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/bashy")
		cmd.Dir = moduleRoot(t)
		if workspace := filepath.Join(filepath.Dir(cmd.Dir), "go.work"); fileExists(workspace) {
			cmd.Env = append(os.Environ(), "GOWORK="+workspace)
		}
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = &buildError{err, string(out)}
			return
		}
		buildBin = bin
	})
	if buildErr != nil {
		t.Fatalf("build ./cmd/bashy: %v", buildErr)
	}
	return buildBin
}

type buildError struct {
	err error
	out string
}

func (e *buildError) Error() string { return e.err.Error() + "\n" + e.out }

func fileExists(path string) bool { _, err := os.Stat(path); return err == nil }

// Fixture generators behind each rod's _PY hook. They parse the rod's real
// CLI and write real-typed fixture bytes: a silent WAV, a 1x1 PNG, an SRT.
// (Byte strings verified against python-generated fixtures with `file`.)
const fakeWavGen = `#!/bin/sh
out=""
while [ $# -gt 0 ]; do case $1 in --out) out=$2; shift 2 ;; *) shift ;; esac; done
printf '\122\111\106\106\244\076\000\000\127\101\126\105\146\155\164\040\020\000\000\000\001\000\001\000\100\037\000\000\200\076\000\000\002\000\020\000\144\141\164\141\200\076\000\000' >"$out"
head -c 16000 /dev/zero >>"$out"
`

const fakePngGen = `#!/bin/sh
outdir=""
while [ $# -gt 0 ]; do case $1 in --outdir) outdir=$2; shift 2 ;; *) shift ;; esac; done
png="$outdir/image-01.png"
printf '\211\120\116\107\015\012\032\012\000\000\000\015\111\110\104\122\000\000\000\001\000\000\000\001\010\006\000\000\000\037\025\304\211\000\000\000\013\111\104\101\124\170\234\143\140\200\002\000\000\011\000\001\373\122\270\251\000\000\000\000\111\105\116\104\256\102\140\202' >"$png"
echo "$png"
`

const fakeSRTGen = `#!/bin/sh
out=""
while [ $# -gt 0 ]; do case $1 in --out) out=$2; shift 2 ;; *) shift ;; esac; done
printf '1\n00:00:00,500 --> 00:00:02,000\nHello from the opening night\n' >"$out"
`

// fakeFFmpeg stands in for ffmpeg in tests: it copies -i INPUT to the last
// operand, which is exactly the trim shape the narrate rod uses.
const fakeFFmpeg = `#!/bin/sh
in=""; prev=""
for a in "$@"; do
  case $prev in -i) in=$a ;; esac
  prev=$a
done
out=$prev
if [ -n "$in" ] && [ -f "$in" ] && [ "$in" != "$out" ]; then cp "$in" "$out"; fi
exit 0
`

type rodDef struct {
	name      string
	provides  []string
	requires  string
	role      string
	request   map[string]any
	wantExt   string
	wantMedia string
	withInput string // "image" or "audio": staging a fixture input file
}

func rodDefs() []rodDef {
	return []rodDef{
		{
			name: "song-acestep", provides: []string{"song"},
			requires: "has=uv has=ffmpeg has=git",
			role:     "song",
			request:  map[string]any{"prompt": "a calm ballad", "duration_s": 30},
			wantExt:  ".wav", wantMedia: "audio/wav",
		},
		{
			name: "image-qwen", provides: []string{"image.generate", "image.edit"},
			requires: "has=uv",
			role:     "image.generate",
			request:  map[string]any{"prompt": "Nora on stage, blue light", "width": 512, "height": 512, "count": 1},
			wantExt:  ".png", wantMedia: "image/png",
		},
		{
			name: "image-qwen", provides: []string{"image.generate", "image.edit"},
			requires: "has=uv",
			role:     "image.edit",
			request:  map[string]any{"prompt": "same stage, warmer light"},
			wantExt:  ".png", wantMedia: "image/png",
			withInput: "image",
		},
		{
			name: "image-klein", provides: []string{"image.generate", "image.edit"},
			requires: "has=uv", role: "image.generate",
			request: map[string]any{"prompt": "Nora on stage, blue light", "width": 512, "height": 512},
			wantExt: ".png", wantMedia: "image/png",
		},
		{
			name: "image-klein", provides: []string{"image.generate", "image.edit"},
			requires: "has=uv", role: "image.edit",
			request: map[string]any{"prompt": "same stage, warmer light"},
			wantExt: ".png", wantMedia: "image/png", withInput: "image",
		},
		{
			name: "narrate-local", provides: []string{"narrate"},
			requires: "has=uv has=ffmpeg",
			role:     "narrate",
			request:  map[string]any{"text": "Hello from the opening night."},
			wantExt:  ".wav", wantMedia: "audio/wav",
		},
		{
			name: "transcribe-whisperx", provides: []string{"transcribe"},
			requires: "has=uv has=ffmpeg",
			role:     "transcribe",
			request:  map[string]any{"format": "srt"},
			wantExt:  ".srt", wantMedia: "application/x-subrip",
			withInput: "audio",
		},
	}
}

func writeWAV(t *testing.T, path string) {
	t.Helper()
	head := []byte("RIFF$\x00\x00\x00WAVEfmt \x10\x00\x00\x00\x01\x00\x01\x00@\x1f\x00\x00\x80>\x00\x00\x02\x00\x10\x00data\x80>\x00\x00")
	data := append(head, make([]byte, 16000)...)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePNG(t *testing.T, path string) {
	t.Helper()
	// 1x1 transparent PNG (same bytes as the fixture generator).
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
		0x89, 0x00, 0x00, 0x00, 0x0b, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9c, 0x63, 0x60, 0x80, 0x02, 0x00,
		0x00, 0x09, 0x00, 0x01, 0xfb, 0x52, 0xb8, 0xa9,
		0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44,
		0xae, 0x42, 0x60, 0x82}
	if err := os.WriteFile(path, png, 0o644); err != nil {
		t.Fatal(err)
	}
}

type fixture struct {
	t          *testing.T
	bin, door  string
	root       string
	rodsDir    string
	stubbin    string
	env        []string
	registered map[string]bool
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	stubbin := filepath.Join(root, "stubbin")
	if err := os.MkdirAll(stubbin, 0o755); err != nil {
		t.Fatal(err)
	}
	stubs := map[string]string{
		"uv": "#!/bin/sh\nexit 0\n", "git": "#!/bin/sh\nexit 0\n",
		"ffmpeg": fakeFFmpeg,
		// timeout drops its duration and execs the rest, so the door's
		// install-hint probe (`timeout 30 ROD --doctor`) works hermetically.
		"timeout": "#!/bin/sh\nshift\nexec \"$@\"\n",
	}
	for name, body := range stubs {
		if err := os.WriteFile(filepath.Join(stubbin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	f := &fixture{
		t:          t,
		bin:        bashyBin(t),
		door:       filepath.Join(moduleRoot(t), "examples", "service-door", "service-door.bsh"),
		root:       root,
		stubbin:    stubbin,
		registered: map[string]bool{},
	}
	f.rodsDir = filepath.Join(moduleRoot(t), "examples", "oss-rods")
	for _, d := range []string{"ring", "home", "cache"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fakes := map[string]string{
		"SONG_ACESTEP_PY": fakeWavGen, "IMAGE_QWEN_PY": fakePngGen,
		"IMAGE_KLEIN_PY": fakePngGen,
		"NARRATE_LOCAL_PY": fakeWavGen, "TRANSCRIBE_WHISPERX_PY": fakeSRTGen,
	}
	f.env = append(os.Environ(),
		"BASHY_COMMANDS_DIR="+filepath.Join(root, "ring"),
		"BASHY_HOME="+filepath.Join(root, "home"),
		"BASHY_HINTS=off", "BASHY_TELEMETRY_QUIET=1",
		"BASHY_SVC_BASHY_BIN="+f.bin,
		"BASHY_OSS_CACHE="+filepath.Join(root, "cache"),
		"TMPDIR="+root, "TMP="+root, "TEMP="+root,
		"PATH="+stubbin+string(os.PathListSeparator)+os.Getenv("PATH"))
	for hook, script := range fakes {
		p := filepath.Join(root, strings.ToLower(hook)+".sh")
		if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		f.env = append(f.env, hook+"="+p)
	}
	return f
}

// addRod registers rods/<name>.bsh with the record fields from
// DEFAULT_COMMANDS (provides may repeat; requires is one space-time expr).
func (f *fixture) addRod(name string, provides []string, route, requires string) {
	f.t.Helper()
	if f.registered[name] {
		return
	}
	f.registered[name] = true
	args := []string{"commands", "add", name,
		"--set", "exec.0=" + f.bin, "--set", "exec.1=--bashsharp",
		"--set", "exec.2=" + filepath.Join(f.rodsDir, name+".bsh")}
	for i, p := range provides {
		args = append(args, "--set", "provides."+string(rune('0'+i))+"="+p)
	}
	args = append(args, "--set", "route="+route)
	if requires != "" {
		args = append(args, "--set", "requires="+requires)
	}
	for i, e := range []string{"read", "write", "exec", "net"} {
		args = append(args, "--set", "effects."+string(rune('0'+i))+"="+e)
	}
	cmd := exec.Command(f.bin, args...)
	cmd.Env = f.env
	if out, err := cmd.CombinedOutput(); err != nil {
		f.t.Fatalf("commands add %s: %v\n%s", name, err, out)
	}
}

type envelope struct {
	Schema      string              `json:"schema_version"`
	Role        string              `json:"role"`
	Provider    string              `json:"provider"`
	Route       string              `json:"route"`
	Outcome     string              `json:"outcome"`
	Text        string              `json:"text"`
	CostClass   string              `json:"cost_class"`
	RawOutputs  []map[string]string `json:"outputs"`
	Unavailable []map[string]string `json:"unavailable"`
}

func (f *fixture) runDoor(extra []string, args ...string) (envelope, string, int) {
	f.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, f.bin, append([]string{"--bashsharp", f.door}, args...)...)
	// extra entries override same-named base entries (last wins).
	merged := map[string]string{}
	order := []string{}
	set := func(e string) {
		k, v, _ := strings.Cut(e, "=")
		if _, ok := merged[k]; !ok {
			order = append(order, k)
		}
		merged[k] = v
	}
	for _, e := range f.env {
		set(e)
	}
	for _, e := range extra {
		set(e)
	}
	for _, k := range order {
		cmd.Env = append(cmd.Env, k+"="+merged[k])
	}
	cmd.Dir = f.root
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		f.t.Fatalf("door %v: %v", args, err)
	}
	var env envelope
	if jerr := json.Unmarshal(out, &env); jerr != nil {
		f.t.Fatalf("door %v: stdout is not one envelope: %v\n%s\nstderr: %s", args, jerr, out, stderr.String())
	}
	if env.Schema != "bashy-svc-v1" {
		f.t.Fatalf("schema_version = %q", env.Schema)
	}
	return env, stderr.String(), code
}

func (f *fixture) requestFile(d rodDef) string {
	f.t.Helper()
	body := map[string]any{"schema_version": "bashy-svc-request-v1"}
	for k, v := range d.request {
		body[k] = v
	}
	switch d.withInput {
	case "image":
		p := filepath.Join(f.root, "init.png")
		writePNG(f.t, p)
		body["image"] = p
	case "audio":
		p := filepath.Join(f.root, "take.wav")
		writeWAV(f.t, p)
		body["audio"] = p
	}
	raw, err := json.Marshal(body)
	if err != nil {
		f.t.Fatal(err)
	}
	p := filepath.Join(f.root, "req.json")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// Every rod renders one typed artifact through the door on route oss with
// a fixture generator standing in for the model.
func TestOssRodEnvelopes(t *testing.T) {
	for _, d := range rodDefs() {
		d := d
		t.Run(d.role, func(t *testing.T) {
			f := newFixture(t)
			f.addRod(d.name, d.provides, "oss", d.requires)
			req := f.requestFile(d)
			out := filepath.Join(f.root, "out")
			args := []string{d.role, "--route", "oss", "--out", out, "--json", req}
			if d.role == "image.edit" {
				args = []string{"image.generate", "--role", "image.edit", "--route", "oss", "--out", out, "--json", req}
			}
			env, stderr, code := f.runDoor(nil, args...)
			if code != 0 || env.Outcome != "success" {
				t.Fatalf("exit %d outcome %q\n%s", code, env.Outcome, stderr)
			}
			if env.Provider != d.name || env.Route != "oss" || env.CostClass != "oss" {
				t.Errorf("envelope = %+v", env)
			}
			if env.Role != d.role {
				t.Errorf("role = %q, want %q", env.Role, d.role)
			}
			if len(env.RawOutputs) != 1 {
				t.Fatalf("outputs = %v", env.RawOutputs)
			}
			o := env.RawOutputs[0]
			if !filepath.IsAbs(o["path"]) || !strings.HasSuffix(o["path"], d.wantExt) {
				t.Errorf("output path = %v", o)
			}
			if o["media_type"] != d.wantMedia {
				t.Errorf("media_type = %q, want %q", o["media_type"], d.wantMedia)
			}
			if b, err := os.ReadFile(o["path"]); err != nil || len(b) == 0 {
				t.Errorf("artifact missing or empty: %v %v", o["path"], err)
			}
		})
	}
}

// A rod whose requires fails is unavailable: exit 126 with the rod's own
// install line from its --doctor answer, and the rod never runs.
func TestOssUnavailableInstallHint(t *testing.T) {
	f := newFixture(t)
	// uv and ffmpeg are the song rod's own doctor deps. Removing the stubs
	// and hiding the host PATH makes --doctor report them missing with
	// per-OS install lines; git stays satisfied via /usr/bin/git.
	for _, stub := range []string{"uv", "ffmpeg"} {
		if err := os.Remove(filepath.Join(f.stubbin, stub)); err != nil {
			t.Fatal(err)
		}
	}
	hermetic := "PATH=" + f.stubbin + string(os.PathListSeparator) + "/usr/bin:/bin"
	f.addRod("song-acestep", []string{"song"}, "oss", "has=uv has=ffmpeg has=git")
	req := f.requestFile(rodDef{request: map[string]any{"prompt": "x"}})
	env, stderr, code := f.runDoor([]string{hermetic}, "song", "--route", "oss", "--json", req)
	if code != 126 || env.Outcome != "unavailable" {
		t.Fatalf("exit %d outcome %q", code, env.Outcome)
	}
	if len(env.Unavailable) != 1 || env.Unavailable[0]["provider"] != "song-acestep" {
		t.Fatalf("unavailable = %v", env.Unavailable)
	}
	u := env.Unavailable[0]
	if !strings.Contains(u["reason"], "absent") {
		t.Errorf("reason = %q", u["reason"])
	}
	if !strings.Contains(u["install"], "uv") {
		t.Errorf("install line is not the rod's own remedy: %v", u)
	}
	if !strings.Contains(stderr, "install: "+u["install"]) {
		t.Errorf("stderr lacks the install line:\n%s", stderr)
	}
	if env.Route != "" && env.Route != "oss" {
		t.Errorf("route = %q on an unavailable envelope", env.Route)
	}
}

// The disk-headroom preflight fails closed before any provisioning when
// the cache cannot hold the weights.
func TestOssPreflightRefuses(t *testing.T) {
	f := newFixture(t)
	f.addRod("narrate-local", []string{"narrate"}, "oss", "has=uv has=ffmpeg")
	req := f.requestFile(rodDef{request: map[string]any{"text": "hi"}})
	env, stderr, code := f.runDoor([]string{"OSS_AVAIL_KB_OVERRIDE=100"},
		"narrate", "--route", "oss", "--json", req)
	if code == 0 || env.Outcome == "success" {
		t.Fatalf("preflight passed with 100 KiB free: exit %d %+v", code, env)
	}
	if !strings.Contains(stderr, "GiB free") || !strings.Contains(stderr, "BASHY_OSS_CACHE") {
		t.Errorf("preflight error names no remedy:\n%s", stderr)
	}
}

// --doctor answers without side effects: available with deps present,
// missing with the per-OS install line when they are absent.
func TestOssDoctor(t *testing.T) {
	f := newFixture(t)
	rod := filepath.Join(f.rodsDir, "transcribe-whisperx.bsh")
	run := func(env []string) (map[string]any, int) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, f.bin, "--bashsharp", rod, "--doctor", "--json")
		cmd.Env = append(append([]string{}, f.env...), env...)
		out, err := cmd.Output()
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("doctor: %v", err)
		}
		var doc map[string]any
		if jerr := json.Unmarshal(out, &doc); jerr != nil {
			t.Fatalf("doctor is not JSON: %v\n%s", jerr, out)
		}
		return doc, code
	}
	doc, code := run(nil)
	if code != 0 || doc["schema_version"] != "bashy-svc-doctor-v1" || doc["available"] != true {
		t.Fatalf("doctor with deps: exit %d %v", code, doc)
	}
	doc, code = run([]string{"PATH=/usr/bin:/bin"})
	if code != 0 || doc["available"] != false {
		t.Fatalf("doctor without deps: exit %d %v", code, doc)
	}
	missing, _ := doc["missing"].([]any)
	if len(missing) == 0 {
		t.Fatalf("doctor without deps lists nothing missing: %v", doc)
	}
	first, _ := missing[0].(map[string]any)
	if first["need"] == nil || first["install"] == nil || first["install"] == "" {
		t.Errorf("missing entry has no install line: %v", missing)
	}
}

// A request from another schema_version major is refused, not rendered.
func TestOssSchemaMajorRefused(t *testing.T) {
	f := newFixture(t)
	f.addRod("song-acestep", []string{"song"}, "oss", "has=uv has=ffmpeg has=git")
	p := filepath.Join(f.root, "req.json")
	raw, _ := json.Marshal(map[string]any{"schema_version": "bashy-svc-request-v9", "prompt": "x"})
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	env, _, _ := f.runDoor(nil, "song", "--route", "oss", "--json", p)
	if env.Outcome == "success" {
		t.Fatalf("v9 request rendered: %+v", env)
	}
}
