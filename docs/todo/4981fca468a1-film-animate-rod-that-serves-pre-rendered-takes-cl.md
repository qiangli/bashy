---
id: 4981fca468a1
kind: feature
title: 'film: animate rod that serves pre-rendered takes (cloud lip-sync clips) with recorded provider, model and cost'
seq: 3235
status: todo
priority: p1
labels:
    - media
    - film
created: 2026-11-14T00:04:19.873199Z
sprint: 635
sprint_id: b2a7f529-ebc1-5236-9cb0-1327ba3c2e8c
sprint_title: 'M8 showcase: commercial final + lip sync (carried from Sprint 627, operator decision)'
---

Sprint 635 lip sync (conductor claude-fable5.1-s635; plan: the Sprint 635 card continuity + umbrella docs/todo/efd9bc004b56 M8 rules). Lip-synced performance clips are rendered on a self-terminating AWS GPU instance (EchoMimicV3 / InfiniteTalk, Apache-2.0) and returned through S3; the film must assemble them as ordinary animate takes with honest provenance.

Do (bashy only, branch from main 54e462db):
- New rod examples/oss-rods/animate-prerendered.bsh for role animate (contract docs/service-rod-contract.md; register in examples/defaults with route oss, requires has=ffmpeg): request {image, audio, shot (id or index), takes_dir} -> looks up takes_dir/<shot>.mp4 (or takes_dir/takes.json mapping shot -> file with provider, model, revision, instance_type, elapsed_s, cost_usd, clip sha256); verifies the clip's sha256 against takes.json when present; returns outputs [{path, media_type: video/mp4}] and text 'prerendered take <shot>: <model> on <instance> (USD x.xx)'; FAILS (exit 1, envelope error) when the shot has no take - never a silent still-pan fallback; --doctor --json read-only (ffmpeg registered, takes dir readable).
- film.bsh: --takes DIR (and FILM_TAKES env) passed to the animate stage so stage_opts animate adds --provider animate-prerendered when a take exists for the shot and the request carries takes_dir; the stage record keeps provider/route/cost_class as the rod reports plus the take's model/cost fields in text; shots without a take keep the existing behaviour (film-still-pan recorded as degraded).
- Tests: fixture takes dir (ffmpeg synthetic mp4 + takes.json) in examples/film/testdata; film_test.go: a draft with --takes uses the prerendered clip for the mapped shots and still-pan for the rest, the manifest shows both providers; a bad sha256 or a missing mapped take fails; oss_rods_test.go for the rod (success, missing take, sha mismatch, doctor). Targeted tests only: GOWORK=off go -C examples/film test ./... ; go -C examples/oss-rods test -run Prerendered ./... ; go -C examples/defaults test ./...
- Windows parity rules from Sprint 634 (kb page native-windows-rods-under-bashy…): registered command names only, no has=jq, native paths to native tools, no python3 dependency.
- Commits with Sprint/Story/Story-ID + your Agent trailer; do not push; no --yes, no spend; do not touch other agents' work; submit with bashy sprint submit 635 <story> -m '<evidence>'. Time box 75 min.
