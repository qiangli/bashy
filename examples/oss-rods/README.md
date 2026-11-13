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

`image-klein` is the draft default; `image-qwen` remains an optional quality
upgrade. The FLUX.2 single-file checkpoint alone is not a full Diffusers
snapshot: stage its pinned transformer, text encoder, tokenizer, VAE, scheduler
and `model_index.json` under `image-klein/models/FLUX.2-klein-4B` (or set
`IMAGE_KLEIN_MODEL_DIR`). The rod checks these files before provisioning,
loads only local files and sets Hub/Transformers offline flags. Mac fallback for transcribe:
mlx-whisper (MIT).

Every rod:

- answers `ROD --doctor --json` with no side effects, even when its
  `requires:` fails (per-OS install lines);
- runs the disk-headroom preflight **before** any weight download
  (`oss_preflight_gb` in `oss-preflight.bsh`; Sprint 382 failed on a
  97%-full host);
- provisions pinned packages + weights on first use only, never at install;
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
