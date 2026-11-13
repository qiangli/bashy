# Sprint 627: offline OSS rod renders

Story `e013898155eb` widens the song follow-up to every OSS rod. The failure
was a missing local checkpoint binding: ACE-Step resolved its checkout's
`checkpoints` path and tried a ModelScope fallback, then selected an unstaged
4B LM. WhisperX also chose a remote model name when its override was absent.

Implementation:

1. Resolve staged model paths under `BASHY_OSS_CACHE/bashy/oss` and validate
   required files before invoking inference. Fail with the pinned `hf download`
   staging command when a file is absent.
2. Launch ACE-Step from the rod's `--serve` entry after binding the checkout
   to that snapshot; leave LM off unless a staged 1.7B LM is explicitly chosen.
3. Set offline Hub, Transformers, ModelScope, uv and pip controls for all OSS
   renders and use local loader APIs for narration and image models.
4. Run the hermetic fixture gate, then one offline song render on the claimed
   Windows test host and inspect its log for zero download markers.

Package environments and model snapshots are staged separately from renders.
The immutable revisions and weight checksums are recorded in the umbrella's
`docs/sprint-627-m2b-weights-evidence.md`.
