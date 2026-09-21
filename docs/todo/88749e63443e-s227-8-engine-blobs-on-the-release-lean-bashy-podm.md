---
id: 88749e63443e
kind: feature
title: 'S227.8 Engine blobs on the release: lean bashy podman self-provisions on linux amd64/arm64 + darwin — fix the never-firing engine-blobs trigger, decide latest-vs-pinned engine tag'
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

The premise of the sprint fails at its third line today, on every platform. Measured 2026-09-20 against the published releases:

- the lean `bashy podman` (internal/agentos/engines_stub.go) resolves a host podman on $PATH, else `provisionEngine` → `binmgr.ProvisionManaged` → `fetchReleaseGz`, which looks for `podman-<goos>-<goarch>.gz` on the LATEST release of `qiangli/bashy` (yoke/pkg/binmgr/managed.go);
- `v0.24.9` (latest) carries zero engine blobs; the only release that ever did is `v0.9.0` (darwin/arm64 only: podman + gvproxy + vfkit);
- `.github/workflows/engine-blobs.yml` has run ONCE (workflow_dispatch, 2026-07-04: macos-14 success, macos-13 cancelled at the 24 h cap). Its `release: published` trigger never fires because `release.yml` publishes with `secrets.GITHUB_TOKEN` (GitHub suppresses workflow-caused events for that token). Its matrix has NO linux runner, although `scripts/publish-engine-blobs.sh` already knows the linux set (`podman gvproxy`);
- there is no `Build` hook on the engine ManagedSpec, so a fresh host with no podman on $PATH gets `engineNotFoundMessage` — a second download, which the sprint premise names a story-level failure.

Deliver, in this order:
1. DECISION (operator, recorded here before claiming): where the lean binary looks for blobs. (a) every release re-publishes blobs — a podman-from-source build per release, the thing that hit the 24 h cap; or (b) a dedicated rolling engine release tag (e.g. `engines-v1`, `BASHY_ENGINE_TAG` override next to the existing `BASHY_ENGINE_REPO`) that `fetchReleaseGz` reads when set — blobs are rebuilt only when podman is bumped. Recommendation: (b); podman moves independently of bashy, and a blob build failing must never gate a bashy release (the workflow header already says so).
2. Publish blobs for linux/amd64 + linux/arm64 (ubuntu runner + arm64 runner or the Linux test host via `publish-engine-blobs.sh`) and darwin/arm64 + darwin/amd64; the linux build is the one the sprint's self-contained gate actually exercises. Record build time per platform — if a runner cannot finish under the cap, the named self-hosted host builds it and the evidence says so.
3. Fix the trigger: `release.yml` calls the blob job via `workflow_call` or `workflow_dispatch` with a PAT/app token, OR the runbook's promote step dispatches it by hand — pick one, put it in the release runbook (kb) as a gate line.
4. A cold-cache probe in the release checklist: `BASHY_BIN_CACHE=$(mktemp -d) PATH=<scrubbed> bashy podman --version` on linux + macOS must answer from the fetched blob; its output line is the evidence.

Windows: no podman blob is published or needed — podman on Windows drives a WSL Linux machine and cannot build or run the nanoserver (Windows-native) image that S227.5 produces; that job's host engine is the recorded exception (S227.5 / S227.7).

Acceptance: a fresh host (linux test host + a macOS host) with an empty `BASHY_BIN_CACHE` and no podman/docker on `$PATH` runs `bashy podman --version` from the release download alone, with the `binmgr: fetching podman (…)` line in the evidence; blob assets listed per platform with digests in the evidence record; the trigger fix proven by one release whose blob run id is linked; `docs/airgap-image.md` §"What the host needs" names rootless prerequisites (`newuidmap`/`subuid`) or states rootful as the requirement, per distro measured.
