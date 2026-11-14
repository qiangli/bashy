// Copyright (c) 2026 qiangli
// See LICENSE for licensing information

// Sprint 635: animate-echomimic (EchoMimicV3 Flash). Fixture generator only:
// no model, weight download, install or clone runs on the dev box.
package ossrods

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeEchomimicGen stands in for the patched infer_flash.py: it requires the
// offline + recipe env, records its argv and writes a synthetic clip into
// --save_path (in a subdirectory, as the real script may).
const fakeEchomimicGen = `#!/bin/sh
set -eu
for key in HF_HUB_OFFLINE TRANSFORMERS_OFFLINE HF_DATASETS_OFFLINE UV_OFFLINE PIP_NO_INDEX; do eval "value=\${$key}"; [ "$value" = 1 ] || exit 91; done
[ "$USE_TF" = 0 ] && [ "$TRANSFORMERS_NO_TF" = 1 ] && [ "$HF_HUB_DISABLE_XET" = 1 ] && [ "$PYTORCH_CUDA_ALLOC_CONF" = expandable_segments:True ] || exit 92
printf '%s\n' "$*" >"$ECHOMIMIC_ARGS_LOG"
save=""
while [ $# -gt 0 ]; do case $1 in --save_path) save=$2; shift 2 ;; *) shift ;; esac; done
mkdir -p "$save/sub"
printf 'synthetic-echomimic-clip' >"$save/sub/sample-0.mp4"
`

// fakeMuxFFmpeg is the registered ffmpeg in these tests: the mux copies the
// FIRST -i (the generated clip) to the last operand.
const fakeMuxFFmpeg = `#!/bin/sh
in=""; prev=""
for a in "$@"; do
  case $prev in -i) [ -n "$in" ] || in=$a ;; esac
  prev=$a
done
if [ -n "$in" ] && [ -f "$in" ] && [ "$prev" != "$in" ] && [ "$prev" != - ]; then cp "$in" "$prev"; fi
exit 0
`

// echomimicFixture registers the stub ffmpeg and animate-echomimic (accepts
// /model=^echomimic, as DEFAULT_COMMANDS does) and returns the argv log.
func (f *fixture) echomimicFixture() string {
	f.t.Helper()
	ff := filepath.Join(f.root, "mux-ffmpeg.sh")
	if err := os.WriteFile(ff, []byte(fakeMuxFFmpeg), 0o755); err != nil {
		f.t.Fatal(err)
	}
	for _, args := range [][]string{
		{"commands", "add", "ffmpeg", "--set", "exec.0=" + ff, "--set", "effects.0=read", "--set", "effects.1=write", "--set", "effects.2=exec"},
		{"commands", "add", "animate-echomimic", "--set", "exec.0=" + f.bin, "--set", "exec.1=--bashsharp", "--set", "exec.2=" + filepath.Join(f.rodsDir, "animate-echomimic.bsh"),
			"--set", "provides.0=animate", "--set", "route=oss", "--set", "requires=has=uv",
			"--set", "accepts.0.path=/model", "--set", "accepts.0.pattern=^echomimic",
			"--set", "effects.0=read", "--set", "effects.1=write", "--set", "effects.2=exec", "--set", "effects.3=net"},
	} {
		cmd := exec.Command(f.bin, args...)
		cmd.Env = f.env
		if out, err := cmd.CombinedOutput(); err != nil {
			f.t.Fatalf("bashy %v: %v\n%s", args, err, out)
		}
	}
	gen := filepath.Join(f.root, "echomimic-gen.sh")
	if err := os.WriteFile(gen, []byte(fakeEchomimicGen), 0o755); err != nil {
		f.t.Fatal(err)
	}
	log := filepath.Join(f.root, "echomimic-args.log")
	f.env = append(f.env, "ANIMATE_ECHOMIMIC_PY="+gen, "ECHOMIMIC_ARGS_LOG="+log)
	return log
}

func (f *fixture) echomimicRequest(fields map[string]any) string {
	f.t.Helper()
	img, wav := filepath.Join(f.root, "still.png"), filepath.Join(f.root, "sing.wav")
	writePNG(f.t, img)
	writeWAV(f.t, wav)
	body := map[string]any{"schema_version": "bashy-svc-request-v1", "image": img, "audio": wav}
	for k, v := range fields {
		body[k] = v
	}
	raw, _ := json.Marshal(body)
	p := filepath.Join(f.root, "echomimic-req.json")
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		f.t.Fatal(err)
	}
	return p
}

// The rod renders a take through the door with the pinned recipe's
// arguments, muxes the audio, records provenance, and is selected only for
// a request that asks for it (accepts /model).
func TestAnimateEchomimicRod(t *testing.T) {
	f := newFixture(t)
	argsLog := f.echomimicFixture()
	out := filepath.Join(f.root, "out")
	req := f.echomimicRequest(map[string]any{"model": "echomimicv3-flash-pro", "duration_s": 5, "seed": 7011})
	env, stderr, code := f.runDoor(nil, "animate", "--route", "oss", "--json", "--out", out, req)
	if code != 0 || env.Outcome != "success" || env.Provider != "animate-echomimic" || env.CostClass != "oss" || len(env.RawOutputs) != 1 {
		t.Fatalf("echomimic take: exit %d %+v\n%s", code, env, stderr)
	}
	o := env.RawOutputs[0]
	if o["media_type"] != "video/mp4" || filepath.Base(o["path"]) != "take.mp4" {
		t.Fatalf("output = %v", o)
	}
	if !strings.Contains(env.Text, "EchoMimicV3 echomimicv3-flash-pro@311e176 (125 frames at 25 fps, 8 steps, seed 7011") {
		t.Errorf("text = %q", env.Text)
	}
	clip, _ := os.ReadFile(o["path"])
	if string(clip) != "synthetic-echomimic-clip" {
		t.Fatalf("take is not the muxed generator clip: %q", clip)
	}
	argv, _ := os.ReadFile(argsLog)
	for _, want := range []string{"--video_length 125", "--sample_size 512 512", "--num_inference_steps 8", "--seed 7011",
		"--sampler_name Flow_Unipc", "--enable_teacache", "--fps 25", "--config_path config/config.yaml", "--weight_dtype bfloat16"} {
		if !strings.Contains(string(argv), want) {
			t.Errorf("generator argv missing %q: %s", want, argv)
		}
	}
	var take map[string]any
	raw, err := os.ReadFile(filepath.Join(out, "take.json"))
	if err != nil || json.Unmarshal(raw, &take) != nil {
		t.Fatalf("take.json: %v %s", err, raw)
	}
	sum := sha256.Sum256(clip)
	if take["sha256"] != hex.EncodeToString(sum[:]) || take["provider"] != "animate-echomimic" ||
		take["revision"] != "311e176905a8c4c24b240b530488fe636ce4d249" || take["source_revision"] != "7e89489ca51c0d008fc1963ec6c03fc5bd0b9397" {
		t.Errorf("take.json = %v", take)
	}
	if _, err := os.Stat(filepath.Join(out, "gen")); err == nil {
		t.Errorf("raw generator output left in the out dir")
	}

	// video_length wins over duration_s; sample_size and steps pass through.
	req = f.echomimicRequest(map[string]any{"model": "echomimic", "video_length": 81, "duration_s": 9, "sample_size": 384, "steps": 4})
	if env, stderr, code = f.runDoor(nil, "animate", "--route", "oss", "--json", "--out", filepath.Join(f.root, "out2"), req); code != 0 {
		t.Fatalf("second take: exit %d %+v\n%s", code, env, stderr)
	}
	argv, _ = os.ReadFile(argsLog)
	for _, want := range []string{"--video_length 81", "--sample_size 384 384", "--num_inference_steps 4"} {
		if !strings.Contains(string(argv), want) {
			t.Errorf("generator argv missing %q: %s", want, argv)
		}
	}

	// A request that does not ask for EchoMimic never selects the rod.
	req = f.echomimicRequest(nil)
	env, _, code = f.runDoor(nil, "animate", "--route", "oss", "--json", req)
	if code != 126 || env.Outcome != "unavailable" {
		t.Fatalf("request without model: exit %d %+v", code, env)
	}
}

// Without staged weights a render refuses, names the exact staging command
// and runs no fetch, install or clone.
func TestAnimateEchomimicRefusesUnstaged(t *testing.T) {
	f := newFixture(t)
	f.echomimicFixture()
	marker := filepath.Join(f.root, "network-fetch")
	for _, name := range []string{"uv", "git", "curl", "wget", "hf", "huggingface-cli", "python", "python3"} {
		body := "#!/bin/sh\nprintf '%s\\n' fetch >>'" + marker + "'\nexit 99\n"
		if err := os.WriteFile(filepath.Join(f.stubbin, name), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	req := f.echomimicRequest(map[string]any{"model": "echomimicv3-flash-pro"})
	env, stderr, code := f.runDoor([]string{"ANIMATE_ECHOMIMIC_PY="}, "animate", "--route", "oss", "--json", "--out", filepath.Join(f.root, "out"), req)
	if code == 0 || env.Outcome != "error" || len(env.RawOutputs) != 0 {
		t.Fatalf("unstaged render: exit %d %+v\n%s", code, env, stderr)
	}
	for _, want := range []string{"not staged: python venv", "not staged: BadToBest/EchoMimicV3@311e176905a8c4c24b240b530488fe636ce4d249", "stage it with: bashy animate-echomimic --stage"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("refusal missing %q:\n%s", want, stderr)
		}
	}
	if b, err := os.ReadFile(marker); err == nil {
		t.Fatalf("a fetch/install command ran: %s", b)
	}
	if _, err := os.Stat(filepath.Join(f.root, "cache", "bashy", "oss", "animate-echomimic", "venv")); err == nil {
		t.Fatal("render provisioned a venv")
	}
}

// --doctor --json is read-only: every unstaged item names --stage; a staged
// cache is available. --stage --dry-run names the pins and writes nothing.
func TestAnimateEchomimicDoctorAndStagePlan(t *testing.T) {
	f := newFixture(t)
	f.echomimicFixture()
	rod := filepath.Join(f.rodsDir, "animate-echomimic.bsh")
	run := func(args ...string) ([]byte, string) {
		cmd := exec.Command(f.bin, append([]string{"--bashsharp", rod}, args...)...)
		cmd.Env = append(append([]string{}, f.env...), "ANIMATE_ECHOMIMIC_PY=")
		var stderr strings.Builder
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("rod %v: %v\n%s%s", args, err, out, stderr.String())
		}
		return out, stderr.String()
	}
	doctor := func() map[string]any {
		out, _ := run("--doctor", "--json")
		var doc map[string]any
		if err := json.Unmarshal(out, &doc); err != nil || doc["schema_version"] != "bashy-svc-doctor-v1" {
			t.Fatalf("doctor is not a doctor document: %v\n%s", err, out)
		}
		return doc
	}
	cache := filepath.Join(f.root, "cache", "bashy", "oss", "animate-echomimic")
	doc := doctor()
	missing := fmtJSON(doc["missing"])
	if doc["available"] != false || !strings.Contains(missing, "staged: python venv") || !strings.Contains(missing, "bashy animate-echomimic --stage") {
		t.Fatalf("doctor on an empty cache: %v", doc)
	}
	if _, err := os.Stat(cache); err == nil {
		t.Fatal("doctor created the cache")
	}

	_, plan := run("--stage", "--dry-run")
	for _, want := range []string{"uv python install 3.10", "uv venv", "torch==2.6.0", "diffusers==0.33.1", "transformers==4.51.3",
		"accelerate==1.6.0", "numpy==1.26.4", "huggingface_hub==0.30.2", "pyloudnorm", "mmgp",
		"git clone -q https://github.com/antgroup/echomimic_v3", "checkout -q 7e89489ca51c0d008fc1963ec6c03fc5bd0b9397",
		"animate-echomimic-patch.py", "--revision fc913c34361f4ec879e2f9c78b4f11ae50a937d1",
		"--revision 3991242c806928916fff4a8c0e4f76acf661b743", "echomimicv3-flash-pro/diffusion_pytorch_model.safetensors",
		"--revision 311e176905a8c4c24b240b530488fe636ce4d249"} {
		if !strings.Contains(plan, want) {
			t.Errorf("stage plan missing %q:\n%s", want, plan)
		}
	}
	if _, err := os.Stat(cache); err == nil {
		t.Fatal("--stage --dry-run wrote the cache")
	}

	for _, rel := range []string{"venv/bin/python", "echomimic_v3/config/config.yaml", "echomimic_v3/infer_flash_bashy.py",
		"weights/Wan2.1-Fun-V1.1-1.3B-InP/diffusion_pytorch_model.safetensors", "weights/chinese-wav2vec2-base/pytorch_model.bin",
		"weights/transformer/diffusion_pytorch_model.safetensors", "staged.json"} {
		p := filepath.Join(cache, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if doc := doctor(); doc["available"] != true {
		t.Fatalf("doctor on a staged cache: %v", doc)
	}
}

// The staging patch reproduces the reference script's two edits on a copy,
// for LF and CRLF sources.
func TestAnimateEchomimicPatch(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "infer_flash.py")
	body := "def main():\n    t = load(low_cpu_mem_usage=True if not fsdp_dit else False)\n    v = load(x, low_cpu_mem_usage=True,\n)\n    pipeline.to(device=device)\n    run()\n    pipeline.to(device=device)\n"
	for _, nl := range []string{"\n", "\r\n"} {
		in := strings.ReplaceAll(body, "\n", nl)
		if err := os.WriteFile(src, []byte(in), 0o644); err != nil {
			t.Fatal(err)
		}
		dst := filepath.Join(dir, "infer_flash_bashy.py")
		out, err := exec.Command(py, "-I", filepath.Join(moduleRoot(t), "examples", "oss-rods", "animate-echomimic-patch.py"), src, dst).CombinedOutput()
		if err != nil || !strings.Contains(string(out), "PATCHED_MMGP") {
			t.Fatalf("patch: %v\n%s", err, out)
		}
		got, _ := os.ReadFile(dst)
		s := string(got)
		if strings.Contains(s, "low_cpu_mem_usage=True") || strings.Contains(s, "pipeline.to(device=device)") ||
			!strings.Contains(s, "offload.profile(pipeline, profile_type.LowRAM_HighVRAM, budgets={'*': budgets}, compile=False)") ||
			!strings.Contains(s, "* 0.80)") || !strings.Contains(s, "    pass  # second pipeline.to removed") {
			t.Fatalf("patched copy (nl %q):\n%s", nl, s)
		}
		if nl == "\r\n" && strings.Count(s, "\r\n") != strings.Count(s, "\n") {
			t.Errorf("CRLF source patched with mixed line endings")
		}
		if orig, _ := os.ReadFile(src); string(orig) != in {
			t.Fatal("patch modified the original infer_flash.py")
		}
	}
}
