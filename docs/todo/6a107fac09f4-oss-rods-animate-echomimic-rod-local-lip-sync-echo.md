---
id: 6a107fac09f4
kind: feature
title: 'oss-rods: animate-echomimic rod (local lip sync: EchoMimicV3 Flash, image + audio -> talking take) with bashy-only provisioning'
seq: 3236
status: todo
priority: p1
labels:
    - media
    - film
    - windows
created: 2026-11-14T12:18:48.834395Z
sprint: 635
sprint_id: b2a7f529-ebc1-5236-9cb0-1327ba3c2e8c
sprint_title: 'M8 showcase: commercial final + lip sync (carried from Sprint 627, operator decision)'
---

Sprint 635 (conductor claude-fable5.1-s635). Operator rules: local first; everything missing goes into examples/ and is registered as a custom command; all installs via bashy (bashy uv for Python/venv/packages, never winget/pip on the host); Windows native first.

SPEC = the proven recipe. Read docs/sprint-635-echomimic-staging-reference.ps1 (the exact PowerShell that stages and runs the model natively on a Windows NVIDIA host; it is the spec, not the deliverable) and umbrella docs/sprint-635-lipsync-evidence.md (the pinned revisions, the two inference patches, the twelve attempts and their causes). Verified on an A10G (AWS attempt 12: visible lip sync).

Deliver (bashy, branch from main):
- examples/oss-rods/animate-echomimic.bsh: a rod for role animate, route oss, in the style of image-klein.bsh / transcribe-whisperx.bsh (oss-preflight.bsh helpers; --doctor --json read-only; never provisions during a render; refuses when weights/env are missing and prints the exact staging command). Request: {image, audio, prompt?, duration_s? or video_length?, seed?, sample_size? (default 512), steps (default 8)} -> take.mp4 (H.264 + AAC, audio muxed) + a bashy-svc-rod-v1 result. Fixed pins: source antgroup/echomimic_v3 7e89489ca51c0d008fc1963ec6c03fc5bd0b9397; weights BadToBest/EchoMimicV3 311e176905a8c4c24b240b530488fe636ce4d249 (echomimicv3-flash-pro), alibaba-pai/Wan2.1-Fun-V1.1-1.3B-InP fc913c34361f4ec879e2f9c78b4f11ae50a937d1, TencentGameMate/chinese-wav2vec2-base 3991242c806928916fff4a8c0e4f76acf661b743; Python stack torch 2.6.0 cu124, diffusers 0.33.1, transformers 4.51.3, accelerate 1.6.0, numpy 1.26.4, huggingface_hub 0.30.2, pyloudnorm, mmgp, decord, moviepy, imageio[ffmpeg] (ffmpeg comes from imageio-ffmpeg, exported as IMAGEIO_FFMPEG_EXE/FFMPEG_BINARY; no winget); env USE_TF=0 TRANSFORMERS_NO_TF=1 HF_HUB_DISABLE_XET=1 PYTORCH_CUDA_ALLOC_CONF=expandable_segments:True; the two patches applied to a COPY of infer_flash.py at staging time (low_cpu_mem_usage=False; mmgp offload.profile LowRAM_HighVRAM budget 80 % of VRAM replacing the first pipeline.to(device) and neutralising the second).
- A --stage (or a separate staging subcommand) that provisions deliberately: bashy uv python install 3.10, bashy uv venv under BASHY_OSS_CACHE/bashy/oss/animate-echomimic, bashy uv pip install the pins, git clone + checkout, huggingface-cli download at the pinned revisions into the cache, sha256 of the transformer weight printed. Cross-OS: a PowerShell fence only where Windows needs it; compute the script dir with src=${BASH_SOURCE[0]//\\//}; call tools only as registered commands (register ffmpeg from imageio-ffmpeg via bashy commands add when absent); no has=jq; native paths to native tools (kb page native-windows-rods-under-bashy…).
- Register in examples/defaults (provides animate, route oss, requires has=uv), README row in examples/oss-rods/README.md, docs/service-rod-contract.md untouched unless needed.
- Tests: oss_rods_test.go cases for the rod (doctor missing/present; render refuses without staged weights; a fixture stand-in generator via an env override like NARRATE_LOCAL_PY does, producing a synthetic mp4; envelope shape) — fixture only, no model on the dev box. Targeted: go -C examples/oss-rods test -run Echomimic ./... and go -C examples/defaults test ./... (GOWORK=off).
- Do NOT run the model on the Windows test hosts yourself: the conductor is running the staging reference on the second Windows host now and holds both host claims. Commit with Sprint/Story/Story-ID + your Agent trailer; do not push; no --yes, no spend. Submit with bashy sprint submit 635 <story> -m '<evidence>'. Time box 90 min.

ADDENDUM (operator, same evening): the example must be REUSABLE as a Bash# skill-script plus a dag.md.
- The rod is the skill-script: `~~~skill as guide` frontmatter (name, description, requires) like the other rods, runnable as `bashy --bashsharp examples/oss-rods/animate-echomimic.bsh` and as the registered command.
- Add `examples/film/dag.md` (literate DAG, `bashy dag` conventions: `###` targets with Requires:/Sources:/Effects:, bodies run in cwd, use "$BASHY" for recursion) with targets: `stage-lipsync` (provision env + weights through the rod's staging; idempotent), `doctor-lipsync` (rod --doctor --json), `proof-lipsync` (one 5 s take from PROJECT's locked shot still + its sung segment; writes takes/<shot>.mp4 + takes.json with provider/model/revision/elapsed/sha256), `takes-lipsync` (every close-up shot of the locked storyboard whose character still exists; same takes.json), `draft-lipsync` (film draft DIR --takes takes/ --route oss), `review-lipsync` (contact sheet + mouth grid + motion metric per take into takes/review/). Vars: PROJECT (film dir), SHOTS (optional list). Document the targets in examples/film/README.md.
- Keep every command a registered one; no bare PATH tools; cross-OS (Windows native first).
