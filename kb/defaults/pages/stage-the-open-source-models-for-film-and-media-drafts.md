---
id: 01a21e5c-1e5d-7d75-a48d-002ab0044f2c
seq: 43
form: page
type: runbook
title: Stage the open-source models for film and media drafts
description: 'Which local open models a free (route oss) film/media draft needs — song ACE-Step 1.5, image FLUX.2 klein-4B (full Diffusers snapshot), transcribe faster-whisper large-v3 via WhisperX, narrate Kokoro/Chatterbox, optional translate and lip sync — with pinned revisions, sizes, environments, the BASHY_OSS_CACHE layout, reuse-before-download, and GPU notes — WHEN a film draft fails on a missing weight/env, a new host is set up for video drafts, or someone asks what models the oss route uses (keywords: oss, models, weights, stage, ACE-Step, FLUX, klein, whisperx, kokoro, chatterbox, BASHY_OSS_CACHE, film, song, image)'
tags:
    - media
    - film
    - oss
    - models
    - video
status: candidate
evidence: '2026-11-27: pins and sizes from bashy docs/sprint-627-m2b-weights-evidence.md and examples/oss-rods (README, rod staging messages); restaging on a Windows test host found FLUX klein under a native user profile cache and re-downloaded ACE-Step at ~10 MB/s.'
source:
    tool: claude
created: "2026-11-27T18:54:43Z"
---

A `film` or `media` draft on the free **oss** route runs open models locally. They are never fetched
at render time: every rod sets the Hub, Transformers, uv and pip offline flags and fails fast when a
weight or environment is missing. Staging is therefore a separate, deliberate step, done once per
host and reused by every later video.

## What each stage needs

| Stage (rod) | Model (Hub repo @ revision) | Download | Environment | Runs on |
|---|---|---|---|---|
| song (`song-acestep`) | ACE-Step 1.5 (`ACE-Step/Ace-Step1.5` @ `19671f40…a99e`): turbo DiT, VAE, Qwen3-Embedding-0.6B, plus the optional 5 Hz 1.7B LM | about 10 GB | a checkout of `ACE-Step/ACE-Step-1.5` at the rod's `ACESTEP_PIN`, then `uv sync` | CPU works (about 4 min for a 75 s draft) |
| image (`image-klein`) | FLUX.2 klein-4B (`black-forest-labs/FLUX.2-klein-4B` @ `e7b7dc27…`), as a **full Diffusers snapshot**: transformer, text encoder, tokenizer, VAE, scheduler and `model_index.json`. The 7.75 GB single-file checkpoint alone is not enough. | about 15 GB | `uv venv --python 3.11` plus `diffusers==0.43.0 transformers accelerate safetensors pillow torch` (a GPU torch build for the host) | GPU (about 30 s per 768x432 still) |
| transcribe (`transcribe-whisperx`) | faster-whisper large-v3 (`Systran/faster-whisper-large-v3` @ `edaa852e…`) | 3.1 GB | `uv venv --python 3.11` plus `whisperx==3.8.6` | CPU |
| narrate (`narrate-local`), told reels | Kokoro-82M (@ `f3ff3571…`, light) or Chatterbox (@ `5bb1f6ee…`, expressive) | 0.3 GB or 2.1 GB | the same pins as `media voice` | CPU |
| translate (`translate-ollama`), optional | `qwen3:8b` through a local ollama | about 5 GB | ollama, registered as `ollama-host` | CPU or GPU |
| animate (`animate-echomimic`), lip sync, optional | EchoMimicV3 Flash, staged with `bashy animate-echomimic --stage` | about 20 GB | the rod's own staging | large GPU |

A sung vertical reel needs song, image and transcribe: **about 28 GB** on first staging. Told
reels add a narrate voice. Full revisions, files and SHA-256 values are in bashy
`docs/sprint-627-m2b-weights-evidence.md`, and licence evidence is in
`examples/oss-rods/README.md`.

## Layout: where each rod looks

All rods resolve `$BASHY_OSS_CACHE/bashy/oss/<rod>/`. The default base is `XDG_CACHE_HOME`, then
`LOCALAPPDATA`, then `~/.cache`.

```
song-acestep/models/Ace-Step1.5/                 # hf download --revision … --local-dir here
song-acestep/acestep-1.5/  (+ .ready-<ACESTEP_PIN>, checkpoints -> ../models/Ace-Step1.5)
image-klein/models/FLUX.2-klein-4B/  + image-klein/.venv/     (or IMAGE_KLEIN_MODEL_DIR)
transcribe-whisperx/models/faster-whisper-large-v3/ + transcribe-whisperx/.venv/
```

## Stage

1. Download each model at its **pinned revision** into the rod's folder:

   ```sh
   hf download --revision REV REPO --local-dir "$BASHY_OSS_CACHE/bashy/oss/ROD/models/NAME"
   ```

   Over a slow link, set `HF_HUB_DISABLE_XET=1`. Run long downloads as a detached service, never
   tied to an ssh session.
2. **Reuse before you download.** An existing Hugging Face cache snapshot can be copied in with
   `cp -rL …/snapshots/REV/`. Look in *every* place a previous stage may have used: the Linux and
   WSL `~/.cache`, each Windows user's `%LOCALAPPDATA%\bashy\oss`, and other WSL distros. Search by
   model folder name, not by a guessed user name.
3. Build each environment exactly as the rod's "stage it with" message says. Song: clone, check out
   the pin, `uv sync`, then touch `.ready-<pin>`.
4. Start the song server with `bashy --bashsharp song-acestep.bsh --serve`. Stop it once the song
   stage is done, so it doesn't hold memory the image stage needs.
5. `film doctor` and `bashy providers select ROLE --route oss` confirm the routing.

## GPU notes

- Run one GPU job at a time.
- Keep draft stills small (for example 432x768 or 768x432). Larger decodes hung an integrated GPU
  in practice.
- When a host's GPU is only usable from a native Windows Python while `film` runs under WSL, point
  `IMAGE_KLEIN_PY` at a small shim. The shim runs the unmodified `image-klein.py` in the native venv
  through WSL interop and copies the PNG back. Record the shim's SHA-256 in the render evidence.
