# Sprint 635 — animate-echomimic rod + film lip-sync DAG (plan as delivered)

Story `6a107fac09f4` (#3236). Spec: `docs/sprint-635-echomimic-staging-reference.ps1`
(the proven Windows-native recipe) and the umbrella's
`docs/sprint-635-lipsync-evidence.md` (pins, the two patches, twelve attempts).

## Pieces

1. `examples/oss-rods/animate-echomimic.bsh` — oss rod for `animate`, a Bash#
   skill-script (`~~~skill as guide`). Three modes:
   - `--doctor --json`: read-only; `has=uv`, a registered ffmpeg, and each
     staged item (venv, source, patched copy, three weights, `staged.json`),
     each missing item naming `bashy animate-echomimic --stage`.
   - `--stage [--dry-run]`: the only provisioning path — `bashy uv` Python
     3.10 + venv + pinned packages, `bashy git` clone/checkout, the patch
     (`animate-echomimic-patch.py`) on a COPY of `infer_flash.py`, weights at
     pinned revisions via the venv's `huggingface_cli`, ffmpeg from
     imageio-ffmpeg registered via `bashy commands add`, sha256 recorded.
   - render: refuses when anything is unstaged (exact staging command),
     offline env + recipe env, frames = video_length or duration_s × 25
     rounded up to 4k+1, generator under a watchdog, audio muxed (H.264 +
     AAC) through the registered ffmpeg, `take.json` provenance,
     `bashy-svc-rod-v1` result. `ANIMATE_ECHOMIMIC_PY` is the fixture hook.
2. Registration (`examples/defaults`): `animate-echomimic … animate|oss|has=uv|/model=^echomimic`.
   The `accepts` matcher matters: two unrestricted oss animate rods would be
   ambiguous to the selector, and a rod that refuses for missing weights
   (exit 1) would break `film draft`'s still-pan fallback (exit 126 only).
3. `examples/film/lipsync.bsh` (registered `film-lipsync`): `takes` (close-up
   shots of the locked storyboard → `takes/NN.mp4` + `takes.json`) and
   `review` (contact sheet, mouth grid, mouth-region motion metric).
4. `examples/film/dag.md`: `stage-lipsync`, `doctor-lipsync`, `proof-lipsync`,
   `takes-lipsync`, `draft-lipsync`, `review-lipsync`; vars `PROJECT`, `SHOTS`.

## Tests (fixture only; no model on the dev box)

`examples/oss-rods/echomimic_test.go` (door render + argv + provenance +
selection, unstaged refusal with no fetch, doctor/stage plan, the patch on LF
and CRLF), `examples/film` `TestLipsyncTakesReviewAndDraft`, defaults
registration.

## Remaining

A live `--stage` and render on a claimed Windows NVIDIA host (the conductor
holds both host claims for this sprint).
