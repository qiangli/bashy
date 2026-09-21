---
id: 88749e63443e
kind: feature
title: 'S227.8 FOUNDATION — engine blobs on the pinned tag engines-v1: lean bashy podman self-provisions on linux amd64/arm64 + darwin from the release download alone'
seq: 313
status: todo
priority: p0
labels:
    - airgap
    - self-contained
    - release
created: 2026-09-21T05:25:09.831102Z
sprint: 227
---

The foundation of the sprint: `bashy podman` must work from the release download alone. Today it does not, on any OS — the lean engine dispatch (`internal/agentos/engines_stub.go` → `binmgr.ProvisionManaged` → `fetchReleaseGz`) looks for `podman-<goos>-<goarch>.gz` on the LATEST bashy release, and no release since `v0.9.0` (darwin/arm64 only) carries one. `engine-blobs.yml` ran once (2026-07-04), has no linux runner, and its `release: published` trigger never fires (`release.yml` uses `GITHUB_TOKEN`).

DECIDED (operator, 2026-09-20): **pinned engine tag.** Blobs live on a rolling release tag `engines-v1`; `fetchReleaseGz` reads `BASHY_ENGINE_TAG` (default `engines-v1`, next to the existing `BASHY_ENGINE_REPO`). Blobs are rebuilt only when podman/gvproxy/vfkit is bumped — never per bashy release, never a gate on one. No trigger fix: the existing `workflow_dispatch` is the mechanism; the release runbook (kb) gets one line: "bumped podman? dispatch engine-blobs with tag engines-v1".

Deliver:
1. `BASHY_ENGINE_TAG` in yoke binmgr (`fetchRelease(ctx, repo, tag)` — the parameter already exists) + the default in `engineSpec`; yoke commit, bashy commit, pins.
2. `engine-blobs.yml`: add `ubuntu-latest` (linux/amd64) and an arm64 linux runner to the matrix; `publish-engine-blobs.sh` already knows the linux set (`podman gvproxy`). Dispatch it once for `engines-v1`: linux amd64 + arm64, darwin arm64 (+ amd64 if the runner finishes under the cap; else recorded as deferred). Windows: no blob in 227 — a host podman on `$PATH` is what Windows gets (recorded in the doc).
3. Cold-cache probe in the release runbook: `BASHY_BIN_CACHE=$(mktemp -d) PATH=<no podman/docker> bashy podman --version` answers from the fetched blob.

License (operator reminder, 2026-09-20): podman, gvproxy and vfkit are Apache-2.0, built from source in CI and fetched + exec'd as separate processes — never linked into bashy; `publish-engine-blobs.sh` uploads the attribution NOTICE beside the blobs and that stays mandatory on `engines-v1` (docs/licensing-supply-chain-policy.md). The machine OS image podman fetches on `machine init` is upstream's, under upstream terms — bashy never redistributes it.

Acceptance: the probe passes on the Linux test host and a macOS host from the release download; the blob list on `engines-v1` (names + digests) and the dispatch run id are in the evidence record.
