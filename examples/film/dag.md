---
name: film-lipsync
description: Lip-synced takes for a film's locked storyboard with the open-source EchoMimicV3 rod on a local NVIDIA GPU (Windows native first) — stage the model, check it, render a 5 s proof or every close-up shot, assemble a draft from the takes and review them. Use when a film project needs real lip sync instead of the still-pan fallback.
type: dag
default: doctor-lipsync
vars:
  PROJECT ?=
  SHOTS ?=
---

# Film lip sync — task graph

Drives the registered commands `bashy animate-echomimic` (the oss rod,
examples/oss-rods/animate-echomimic.bsh), `bashy film-lipsync`
(examples/film/lipsync.bsh) and `bashy film`. Bodies run in the invoking
directory (make parity); `PROJECT` is the film directory (`film init` /
`film draft` / `film lock` made it), `SHOTS` optionally narrows the shots
(1-based numbers or shot ids, space or comma separated).

```bash
bashy dag -f examples/film/dag.md stage-lipsync            # once per host (~12 GB)
bashy dag -f examples/film/dag.md proof-lipsync PROJECT=my-video
bashy dag -f examples/film/dag.md takes-lipsync PROJECT=my-video SHOTS="3 5"
bashy dag -f examples/film/dag.md draft-lipsync review-lipsync PROJECT=my-video
```

Renders never provision: an unstaged host refuses with the staging command.
The GPU is exclusive — claim it first (`bashy claim gpu --intent …`).

## Tasks

### stage-lipsync
Provision the pinned EchoMimicV3 stack and weights through the rod's staging
(bashy uv Python 3.10 venv, pinned packages, source checkout and patched copy,
Hugging Face weights at pinned revisions, ffmpeg from imageio-ffmpeg
registered). Idempotent: completed steps are skipped.
Effects: read, write, net, exec
Timeout: 3h

```bash
"$BASHY" animate-echomimic --stage
```

### doctor-lipsync
Read-only: what the rod still needs on this host (bashy-svc-doctor-v1).
Effects: read, exec

```bash
"$BASHY" animate-echomimic --doctor --json
```

### proof-lipsync
One 5 s take from the first close-up shot of the locked storyboard (its
locked still and its sung segment): `PROJECT/takes/NN.mp4` plus
`PROJECT/takes/takes.json` (provider, model, revision, elapsed_s, sha256).
Effects: read, write, exec
Timeout: 3h

```bash
[ -n "$PROJECT" ] || { echo "proof-lipsync: set PROJECT=DIR (the film directory)" >&2; exit 2; }
"$BASHY" film-lipsync takes "$PROJECT" --proof ${SHOTS:+--shots "$SHOTS"}
```

### takes-lipsync
Every close-up shot (`kind: close`) of the locked storyboard whose still
exists, each for its full sung segment, into the same `takes.json`.
Effects: read, write, exec
Timeout: 24h

```bash
[ -n "$PROJECT" ] || { echo "takes-lipsync: set PROJECT=DIR (the film directory)" >&2; exit 2; }
"$BASHY" film-lipsync takes "$PROJECT" ${SHOTS:+--shots "$SHOTS"}
```

### draft-lipsync
A draft that assembles the takes on the LOCK (`--locked`: the locked draft's
song, timing, portraits and shot bounds, so the takes cut from that song stay
in sync; animate-prerendered per shot with a take, the other shots keep the
oss route or the labelled still-pan fallback).
Effects: read, write, exec, net
Timeout: 2h

```bash
[ -n "$PROJECT" ] || { echo "draft-lipsync: set PROJECT=DIR (the film directory)" >&2; exit 2; }
[ -f "$PROJECT/takes/takes.json" ] || { echo "draft-lipsync: no $PROJECT/takes/takes.json (run proof-lipsync or takes-lipsync)" >&2; exit 1; }
"$BASHY" film draft "$PROJECT" --takes "$PROJECT/takes" --locked --route oss
```

### review-lipsync
Per take: a contact sheet, a mouth-region grid and the mouth-region motion
metric (tblend=difference + signalstats YAVG mean/max) into
`PROJECT/takes/review/` (`review.json`).
Effects: read, write, exec

```bash
[ -n "$PROJECT" ] || { echo "review-lipsync: set PROJECT=DIR (the film directory)" >&2; exit 2; }
"$BASHY" film-lipsync review "$PROJECT"
```
