# M2 oss rods — song, image, narrate, transcribe

Sprint 627 M2. One rod per open-source pick in
`docs/sprint-627-master-execution-plan.md`, all route `oss`, all behind the
M1 rod contract (`bashy/docs/service-rod-contract.md`). Animate stays out:
it needs a 24 GB NVIDIA host the fleet has not confirmed (M3).

## Rods

| Rod | Roles | Backend | Request → artifact |
|---|---|---|---|
| `song-acestep` | `song` | ACE-Step 1.5, pinned commit `ca1e85fe` (2026-08-29), driven via its local REST API (`uv run acestep-api`, `POST /release_task`, `POST /query_result`, `GET /v1/audio`) | prompt/lyrics/duration → `song.wav` |
| `image-qwen` | `image.generate`, `image.edit` | Qwen-Image-2511 + Qwen-Image-Edit-2511 via diffusers `0.43.0`; FP8/GGUF footprints need a 24 GB host | prompt/size/count (+ image/mask) → `image-NN.png` |
| `image-klein` | `image.generate`, `image.edit` | FLUX.2 klein-4B, pinned local Diffusers snapshot; offline only | prompt/size (+ image for edit) → one PNG |
| `narrate-local` | `narrate` | Chatterbox default, `kokoro:<voice>` light path; same pins as `media voice` | text/voice → `narration.wav` (silence-trimmed) |
| `transcribe-whisperx` | `transcribe` | WhisperX `3.8.6` on Whisper large-v3, `--compute_type int8` for Mac/CPU | audio → `transcript.srt`/`.txt` |
| `translate-ollama` | `translate` | Qwen3 `qwen3:8b` through a local ollama's HTTP API (Sprint 634); think off, temperature 0, one numbered request per cue list | SRT cues + target language → `translation.srt` (same timestamps, line count checked) |

All oss rods render from local snapshots under `BASHY_OSS_CACHE/bashy/oss`.
They set `HF_HUB_OFFLINE=1`, `TRANSFORMERS_OFFLINE=1`,
`MODELSCOPE_OFFLINE=1`, `MS_OFFLINE=1`, `UV_OFFLINE=1`, and
`PIP_NO_INDEX=1` before invoking a model. A missing weight or Python
environment fails before inference. Stage packages and the pinned model
snapshots as a separate, deliberate operation; use the immutable revisions
and SHA-256 list in `docs/sprint-627-m2b-weights-evidence.md` and the
offline gate in `oss_rods_test.go`. For example:

```sh
hf download --revision 19671f406d603126926c1b7e2adc169acbcade22 ACE-Step/Ace-Step1.5 \
  --local-dir "$BASHY_OSS_CACHE/bashy/oss/song-acestep/models/Ace-Step1.5"
```

For song, stage the pinned ACE-Step checkout and its Python environment,
then launch `bashy --bashsharp song-acestep.bsh --serve`. The launcher checks
the staged weights, binds the checkout's `checkpoints` path to the staged
snapshot, and starts the API with its LM disabled. Set
`SONG_ACESTEP_LM=1.7B` only when that LM's staged weight is present. The
render rod refuses a server that was not started by this launcher.

`translate-ollama` talks to `http://127.0.0.1:11434` (`TRANSLATE_OLLAMA_URL`
overrides). Its `--doctor --json` is read-only: it runs `bashy ollama list`
through a REGISTERED `ollama` command (`film doctor` registers the host's
ollama where bashy does not ship its own `ollama` verb; elsewhere register it
under another name and set `TRANSLATE_OLLAMA_COMMAND`) and checks the pinned
tag is listed. A render checks `/api/tags` and never pulls: stage the model
once with `ollama pull qwen3:8b`. A reply whose numbered line count differs
from the cue count fails (exit 1) instead of misaligning subtitles.

WhisperX defaults to `transcribe-whisperx/models/faster-whisper-large-v3`
and runs without alignment, which otherwise needs another unstaged model.
Chatterbox and Kokoro load their local files directly. Optional Qwen image
models also require local Diffusers snapshots.

`image-klein` is the only image rod registered by defaults. `image-qwen`
remains available for explicit registration as an optional quality upgrade;
registering both oss image rods requires `--provider` to resolve the route.
The FLUX.2 single-file checkpoint alone is not a full Diffusers
snapshot: stage its pinned transformer, text encoder, tokenizer, VAE, scheduler
and `model_index.json` under `image-klein/models/FLUX.2-klein-4B` (or set
`IMAGE_KLEIN_MODEL_DIR`). The rod checks these files before provisioning,
loads only local files and sets Hub/Transformers offline flags. Mac fallback for transcribe:
mlx-whisper (MIT).

Every rod:

- answers `ROD --doctor --json` with no side effects, even when its
  `requires:` fails (per-OS install lines);
- runs the disk-headroom preflight before inference
  (`oss_preflight_gb` in `oss-preflight.bsh`);
- refuses to provision packages or weights during a render;
- honors its `_PY` hook (`SONG_ACESTEP_PY`, `IMAGE_QWEN_PY`,
  `NARRATE_LOCAL_PY`, `TRANSCRIBE_WHISPERX_PY`) so tests exercise the
  request/preflight/envelope path with a fixture generator, same pattern as
  `MEDIA_VOICE_PY` in `examples/media/test.sh`;
- refuses a request whose `schema_version` major is not v1;
- prints one `bashy-svc-rod-v1` result naming absolute-ready outputs.

## License re-verification (2026-11-13, all permissive for commercial use)

| Model / package | License | Evidence fetched |
|---|---|---|
| ACE-Step 1.5 | MIT | LICENSE text, `ace-step/ACE-Step-1.5@main` |
| Qwen-Image-2511 (`Qwen/Qwen-Image`) | Apache-2.0 | LICENSE text via HF Hub |
| Qwen-Image-Edit-2511 | Apache-2.0 | HF API `license:apache-2.0` tag |
| Z-Image-Turbo (light alt) | Apache-2.0 | HF API `license:apache-2.0` tag |
| FLUX.2 klein-4B (light alt) | Apache-2.0 | HF API `license:apache-2.0` tag |
| Chatterbox (`chatterbox-tts`) | MIT | full MIT text via PyPI |
| Kokoro-82M | Apache-2.0 | HF API `license:apache-2.0` tag |
| WhisperX (`whisperx` 3.8.6) | BSD-2-Clause | LICENSE text, `m-bain/whisperX@main` |
| Qwen3-8B (`qwen3:8b`) | Apache-2.0 | HF model card `Qwen/Qwen3-8B` license tag |
| ollama (separate process, not linked) | MIT | LICENSE text, `ollama/ollama@main` |
| mlx-whisper (fallback) | MIT | PyPI metadata |
| openai-whisper (reference) | MIT | PyPI metadata |
| diffusers 0.43.0 | Apache-2.0 | PyPI metadata |

Excluded as not permissive — referenced by no rod: FLUX.2 dev and klein-9B,
SD3.5, Hunyuan models, LTX-2, YuE2, Sonic, F5-TTS, IndexTTS2, Higgs Audio,
MusicGen, Orpheus. ComfyUI (GPL) is not vendored; image work goes through
diffusers.

## Disk-headroom preflight (GiB that must be free in the rod cache)

| Rod | Need | Covers |
|---|---|---|
| song-acestep | 25 | 4–20 GB weights + uv env + workspace |
| image-qwen | 35 | 24 GB-class weights + uv env + workspace |
| narrate-local | 12 | Chatterbox/Kokoro weights + uv env |
| transcribe-whisperx | 10 | large-v3 (~3 GB) + torch env |
| translate-ollama | — | weights live in ollama's own store (staged once); no rod cache |

Override the cache with `BASHY_OSS_CACHE`; the failure message names the
remedy. `OSS_AVAIL_KB_OVERRIDE` replaces the `df` probe in tests only.

## Venue rule (operator mb:13538)

No model/weight downloads and no live model, ollama or podman runs on the
operator's primary workstation. This host carries code + stubbed/fixture
unit tests only (`go -C examples/oss-rods test ./...`).

## Remaining check (claimed test host: elephant or a droplet)

Deferred until a host is claimed; record cost and time with the run:

1. `bashy song --route oss --json req.json` → one `audio/wav` (ACE-Step
   server from the pinned checkout, `uv run acestep-api --port 18001`).
2. `bashy image --route oss --json req.json` → one `image/png`.
3. `bashy image --role image.edit --route oss --json req.json` →
   one edited `image/png`.
4. `bashy narrate --route oss --json req.json` → one `audio/wav`.
5. `bashy transcribe --route oss --json req.json` → one `.srt` within
   ±150 ms of the fixture timing.
6. Re-verify each LICENSE at provisioning time (this file is the 2026-11-13
   baseline, not a substitute).
