---
id: 1118472f556c
kind: feature
title: 'S227.1 bashy image build: the release ships the static bashy_scratch artifact (amd64+arm64); bashy fetches it and builds a FROM-scratch image through bashy podman — no source on the host'
seq: 305
status: todo
priority: p0
labels:
    - linux
    - oci
    - dag
created: 2026-09-20T21:23:20.094892Z
sprint: 227
---

DECIDED (operator, 2026-09-20): no source on the user's host. The release ships the static artifact; bashy fetches it and builds the image through its own podman. `bashy git` / `bashy go` are the developer path (README "From source"), not part of the image story.

Today `make build-bashy-scratch` builds the static linux/amd64 `bashy_scratch` artifact and `scripts/verify-bashy-scratch.sh` proves it in a throwaway `FROM scratch` image. Nothing publishes it, nothing keeps an image.

Deliver:
1. `build-bashy-scratch` for amd64 AND arm64 (GOARCH; keep the amd64 path stable — it has a consumer); `dist`/`release.yml` publish `bashy-scratch-linux-{amd64,arm64}` beside the other assets, listed in `checksums.txt`.
2. `bashy image build [--arch amd64|arm64] [--tag T]` — a small builtin, one code path: fetch `bashy-scratch-linux-<arch>` for the running version from the release through binmgr (checksum-verified, cached in `BASHY_BIN_CACHE`; `BASHY_SCRATCH_BIN=<path>` uses a local artifact instead), write the Containerfile (`FROM scratch`, `COPY bashy /bashy`, `/tmp`, `/work`, `ENTRYPOINT ["/bashy"]`, env `OTEL_TRACES_EXPORTER=none BASHY_TELEMETRY_QUIET=1 HOME=/tmp`) into a temp context, exec `bashy podman build` — bashy's own engine (S227.8); `BASHY_OCI` stays the operator override. Tag `localhost/bashy:<version>-linux-<arch>`.
3. `dag.md` target `build-image` (Makefile mirror) = the repo/CI form: `make build-bashy-scratch` then `bashy image build` with `BASHY_SCRATCH_BIN` pointed at it, so CI builds the candidate, not a published artifact.

The user contract (README "Containers" section + the verb's help):

    bashy image build
    bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<ver>-linux-<arch> --bashsharp ./script.bsh

The image is bashy as it exists: bash 5.3 surface, `--posix`, `--bashsharp`, builtin coreutils, dag/weave/check/transpile. No externals inside (S227.2 deferred): an island that needs a toolchain fails under `--network=none` as any download does, and the matrix (S227.3) says so.

License: the image contains bashy only — no base image, no third-party file — so it carries bashy's own license and nothing else (docs/licensing-supply-chain-policy.md). Nothing GPL enters at any rung.

Acceptance: `bashy image build` on the dev box and on the Linux test host through `bashy podman` for both arches; `--version`, `-c 'printf ok'` and a mounted `.bsh` using builtin coreutils pass under `--network=none --read-only --cap-drop=ALL --tmpfs /tmp`; `verify-bashy-scratch.sh` still green; the two artifacts on the `-dev` release with checksums; compressed + uncompressed size per arch in the evidence record.
