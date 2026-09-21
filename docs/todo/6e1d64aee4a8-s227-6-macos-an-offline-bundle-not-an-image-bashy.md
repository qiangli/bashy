---
id: 6e1d64aee4a8
kind: feature
title: 'S227.6 macOS: an offline bundle, not an image — bashy + launcher pair + seeded cache as one archive that runs a .bsh with networking disabled; record the decision'
seq: 310
status: blocked
priority: p2
labels:
    - macos
    - dist
created: 2026-09-20T21:23:20.241579Z
---

**DEFERRED — unlinked from sprint 227 (operator, 2026-09-20).** One rule for every OS: no per-OS bundles; the container form is the product. Sprint 227 covers macOS with S227.10 — the linux image through the self-contained `bashy podman` machine (vfkit). This story stays filed for a sprint that has a reason to ship a native darwin bundle (signing/notarization would come with it).

There is no macOS container image: on macOS an OCI engine (bashy podman / podman machine) runs a Linux VM, so the S227.1 linux image already IS the macOS container answer. What "optimized for macOS" can mean is the native air-gapped form: a self-contained archive.

Deliver `bashy dag dist-airgap` (or `dist` with `AIRGAP=1`, pick the smaller change) for darwin/{arm64,amd64}: `bashy` + `bashy.real` launcher pair (tools/installbashy order: payload first), a seeded `BASHY_BIN_CACHE` from the S227.2 manifest (darwin toolchains: python-build-standalone, Go), an `install.sh` that sets `BASHY_OFFLINE=1`, and nothing that phones home (OTEL exporter none, telemetry quiet). Signed/notarized is out of scope here — say so in the doc; note the "rm before cp over a signed binary" rule for upgrades.

Decision to record in docs/airgap-image.md: container → use the linux image under podman machine; native → this bundle. Size table row per arch.

Acceptance: on a macOS host with Wi-Fi off (or `pfctl` block for the process — whichever the test host allows), a .bsh with a Python island runs from the extracted bundle; `bashy go version` answers from the seeded cache; the same script with an unseeded external fails closed by name.
