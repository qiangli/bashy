---
id: 3f2dabf6ad3d
kind: test
title: 'S227.3 The airgap gate: supported-command matrix (docs/airgap-image.md) proven by smoke-airgap-container, run in CI on linux amd64/arm64 as a release gate'
seq: 307
status: todo
priority: p1
labels:
    - airgap
    - docs
    - gate
created: 2026-09-20T21:23:20.154291Z
sprint: 227
---

"Supported commands" is a proven list, not an assertion. Deliver `scripts/airgap-container-smoke.sh` (Makefile `smoke-airgap-container`, dag `smoke-airgap`), modeled on `scripts/quickstart-container-smoke.sh`: builds the S227.1 image, then one probe per row under `--network=none --read-only --cap-drop=ALL`.

`docs/airgap-image.md`, three columns — works in the image / not in the image (with the reason) / degrades with a stated limit:
- shell: bash 5.3, `--posix`, `--bashsharp` (.bsh with agentic{} and contracts);
- userland: the coreutils inventory — rows generated from `bashy commands`, never hand-copied;
- dag, weave, check, transpile;
- not in the image: externals/islands needing a toolchain (use `bashy check --prepare` against a mounted cache — S227.2 deferred), git, podman/ollama engines, otel stack, login/tessaro/sphere/cloud CLIs (remote by design, philosophy.md §3);
- ast/graph without grammar blobs: the stated limit (docs/TODO.md grammar note).
Plus §"What the host needs" per OS from S227.0's inventory.

CI: `.github/workflows/airgap-image.yml`, modeled on `quickstart-scratch.yml` — linux amd64 + arm64 running `smoke-airgap-container`, listed in the release runbook (kb) as a gate at the `-dev` tag. Engine on the runner: try rootless `bashy podman` (the S227.8 linux blob); if it fails on the hosted runner, `BASHY_OCI=docker` as `quickstart-scratch.yml` already does, with the reason recorded. No Windows/macOS CI jobs — the per-OS proof is S227.0 on real hosts.

Acceptance: the script SKIPs (exit 0) without an engine and FAILs on any row mismatch with one; the doc's table is regenerated from the script's output (one source); workflow green on the closing candidate, run id in the evidence; one line in the umbrella docs/INDEX.md + abstract in docs/README.md; umbrella bashy pin bumped.
