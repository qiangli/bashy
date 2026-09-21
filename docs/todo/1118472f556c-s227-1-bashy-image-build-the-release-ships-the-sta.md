---
id: 1118472f556c
kind: feature
title: 'S227.1 bashy image build: the release ships the static bashy_scratch artifact (amd64+arm64); bashy fetches it and builds a FROM-scratch image through bashy podman — no source on the host'
seq: 305
status: doing
priority: p0
labels:
    - linux
    - oci
    - dag
created: 2026-09-20T21:23:20.094892Z
sprint: 227
---

DELIVERED (corbel, 2026-09-21). No source on the user's host: the release ships the static artifact; bashy fetches it and builds the image through its own podman. `bashy git` / `bashy go` remain the developer path (README "From source").

What landed:
1. `build-bashy-scratch` takes `BASHY_SCRATCH_GOARCH=arm64` (amd64 path `bin/scratch/bashy-linux-amd64` unchanged — it has a consumer); `.goreleaser.yaml` build id `bashy-scratch` (tags `bashy_scratch`, linux amd64+arm64, raw binary assets `bashy-scratch-linux-{amd64,arm64}` in `checksums.txt`).
2. `bashy self image [--arch] [--version] [--tag] [--engine]` (internal/agentos/self_image.go): fetches `bashy-scratch-linux-<arch>` for the running version (then its `-dev` prerelease) through binmgr's checksum→cache path, writes the Containerfile (`FROM scratch`, `COPY bashy /bashy`, `/tmp`, `HOME=/tmp/bashy`, `PATH=/`, telemetry off, `WORKDIR /work`, OCI labels, `ENTRYPOINT ["/bashy"]`), builds `localhost/bashy:<ver>-linux-<arch>` through `<this bashy> podman build --platform linux/<arch>`; `BASHY_SCRATCH_BIN` = a local artifact (the repo/CI form); `BASHY_OCI` = another engine. `self fetch/install` now never match the scratch asset.
3. `dag build-image` (Makefile mirror `build-image`): `build-bashy-scratch` for `BASHY_IMAGE_ARCH` (default host arch) + `self image` on it — images the candidate.
4. README "Containers — the offline image" with the two-line contract.

HOME is `/tmp/bashy`, not `/tmp`: Stage 0 output canonicalization rewrites the home prefix to `$HOME` on non-tty sinks (output_reduce.go, story 05c51640 — product behaviour, not this sprint's), and a container's stdout is never a tty; with HOME=/tmp every `/tmp/...` path a script printed came out as `$HOME/...` (measured). The matrix (S227.3) carries the row.

Measured on the Linux test host (Ubuntu 24.04 amd64, managed podman from S227.8, PATH without podman/docker): `self image` → `localhost/bashy:0.25.0-linux-amd64` in 6.7 s; `--version`, `-c 'printf ok'`, `--posix -c 'echo $((6*7))'`, and a mounted `.bsh` using wc/sort/head/grep/date/cut/ls/tr/uname all pass under `--network=none --read-only --cap-drop=ALL --tmpfs /tmp`; `curl` → "command not found" (nothing but bashy in the image); uncompressed 102.9 MB, compressed 46.5 MB (amd64). arm64 artifact built (`bin/scratch/bashy-linux-arm64`, static aarch64 ELF) — run proof for arm64 comes from S227.3's CI arm64 job. `verify-bashy-scratch` green; agentos tests + e2e dispatch gate green; Windows cross-build green.

Acceptance: image builds on the Linux test host through `bashy podman` ✔ (dev box has no machine — macOS proof is S227.0 on novidesign); offline contract ✔; verifier ✔; artifacts on the `-dev` release with checksums — pending the tag (next step); sizes recorded ✔.
