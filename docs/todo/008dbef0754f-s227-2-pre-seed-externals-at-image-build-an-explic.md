---
id: 008dbef0754f
kind: feature
title: 'S227.2 Pre-seed externals at image build: an explicit manifest bakes bashy go / python island / node into BASHY_BIN_CACHE; runtime resolves from cache only, fails closed by name'
seq: 306
status: blocked
priority: p0
labels:
    - oci
    - binmgr
    - airgap
created: 2026-09-20T21:23:20.124265Z
---

**DEFERRED — unlinked from sprint 227 (operator, 2026-09-20: a simple image build of bashy's existing features, KISS).** A pre-seed manifest + BASHY_OFFLINE are new machinery. In 227 the image is bashy-only; externals get a matrix row ("not in the image — run `bashy check --prepare` against a mounted BASHY_BIN_CACHE"). This story is the follow-up when a seeded variant is wanted.

Every external (`bashy go`, uv/python islands, node, cargo, clang, loom/zot/seaweedfs/kopia, MinGit) is a binmgr download-on-first-use into `$BASHY_BIN_CACHE` (`yoke/pkg/binmgr`). philosophy.md §3 says "pre-seed the cache and the air-gap is intact" but there is no verb that pre-seeds: each tool is fetched by whichever command first needs it (`bashy go version`, `bashy check --prepare <script>`).

Deliver:
1. `bashy prefetch <name>...` (or `bashy check --prepare --all` — pick the smaller change, record the choice): resolves each named external through the existing binmgr pins, downloads + verifies it into the cache, prints the cache layout. No new registry; the pins table already is the manifest.
2. `build-image` accepts `EXTERNALS="go python node"` (default: none) and runs the prefetch in the builder stage, then `COPY`s `/var/cache/bashy/bin` into the scratch image. Toolchains are glibc-dynamic: stage their closure with `examples/quickstart/stage-closure.sh` (already proven for python in quickstart mode-c) — generalize that script rather than write a second one.
3. Offline posture: when `BASHY_OFFLINE=1` (set in the image), a cache miss returns `bashy: <tool> is not pre-seeded in this image (BASHY_BIN_CACHE=…); rebuild with EXTERNALS="… <tool>"` and exit non-zero — never a network attempt. `yoke/pkg/atlas/localfirst_test.go` stays green (loop verbs acquire no `net` effect). Cross-repo by construction: `BASHY_OFFLINE` is enforced inside `yoke/pkg/binmgr` (the only place a download starts), so this story lands as a yoke commit + a bashy commit + the `.sibling-pins` bump, in that order.

Acceptance: a `.bsh` with a Python island and one with a Go island run in the image under `--network=none`; an image built without `node` running a TS island fails with the named message and no DNS/connect attempt (prove with `--network=none` + exit code + message grep); `bashy inspect` shows the cache row pointing at the baked path.

Dogfooding note (operator, 2026-09-20): the builder stage that runs the prefetch is a `bashy podman build` stage — the same engine as S227.1; the prefetch itself is the in-image bashy calling binmgr, so the seeded cache is produced by the product, not by a host script.
