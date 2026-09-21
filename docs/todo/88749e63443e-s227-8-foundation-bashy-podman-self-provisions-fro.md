---
id: 88749e63443e
kind: feature
title: 'S227.8 FOUNDATION — bashy podman self-provisions from pinned upstream releases (linux podman-static; darwin client+gvproxy+vfkit; windows client): no bashy-built blobs, download+exec'
seq: 313
status: done
priority: p0
labels:
    - airgap
    - self-contained
    - release
created: 2026-09-21T05:25:09.831102Z
assignee: corbel
sprint: 227
closed: 2026-09-21T07:34:27.474359Z
closed_by: corbel
---

The foundation of the sprint: `bashy podman` must work from the release download alone. It did not, on any OS — the lean engine dispatch looked for `podman-<goos>-<goarch>.gz` on the LATEST bashy release and no release since `v0.9.0` (darwin/arm64 only) carried one; `engine-blobs.yml` ran once (2026-07-04), had no linux runner, and its `release: published` trigger never fires (`release.yml` uses `GITHUB_TOKEN`).

DELIVERED (corbel, 2026-09-21) — decision revised from "pinned engine tag" to the simpler thing the codebase already does for ollama and the Windows machine helpers: **bashy builds no engine blobs; podman comes from the UPSTREAM releases, pinned by version + sha256 committed in `internal/agentos/engines_podman.go`, download+exec, never linked or bundled.** No `engines-v1` tag, no CI build of podman (the build that once hit the 24 h runner cap), no yoke change.

| platform | source (pinned) | what it brings |
|---|---|---|
| linux amd64/arm64 | `mgoltzsche/podman-static` v6.1.2 (Apache-2.0 project) | static podman + conmon + crun/runc + netavark/aardvark-dns + pasta — a complete engine; bashy generates a `CONTAINERS_CONF_OVERRIDE` pointing conmon/runtimes/helpers into the cache tree and seeds the user policy.json when the host has none |
| darwin arm64 (amd64: podman v5.8.7, the last with a client) | `containers/podman` remote client zip + `gvisor-tap-vsock` v0.8.8 `gvproxy-darwin` + `crc-org/vfkit` v0.6.4 (signed universal) | the `podman machine` triad; `helper_binaries_dir` override |
| windows amd64/arm64 | `containers/podman` remote client zip (gvproxy.exe + win-sshproxy.exe ship inside; `winhelper` keeps its own pinned copy) | machine client on WSL2 |

License: podman, conmon, runc, netavark, aardvark-dns, gvproxy, vfkit are Apache-2.0; crun, pasta, fuse-overlayfs inside podman-static are GPL-2.0 — fetched at runtime and exec'd as separate processes, never redistributed by bashy (the runtime/installation-time exception; docs/licensing-supply-chain-policy.md). The podman machine OS image is podman's own fetch under upstream terms.

Measured (evidence record): macOS dev box, empty cache, PATH without podman/docker → `podman version 6.1.2`, 138 MB fetched (podman + gvproxy + vfkit), second run silent cache hit. Linux test host (Ubuntu 24.04 amd64, PATH mirror without podman/docker), empty cache → `podman version 6.1.2` in 2.0 s, 88 MB; `podman info` shows conmon 2.2.1, crun, netavark 2.1.0, aardvark-dns 2.1.0 all resolved from `$BASHY_BIN_CACHE`; a `FROM scratch` build + `run --network=none --read-only --cap-drop=ALL` executed through it (the lean binary itself is not static — S227.1's `bashy_scratch` artifact is).

Known, stated: `resolveEngineBinary` still prefers a host podman on `$PATH` (tier 3) — the gate scrubs `$PATH`; Linux rootless needs `newuidmap`/`subuid` from the host (the test host ran as root — rootless recorded in S227.0); linux/arm64 is pinned but proven only in CI (S227.3's arm64 runner).

Acceptance: cold-cache probe passes on the Linux test host and a macOS host from the release download (S227.0 re-runs it from published bytes); pins + digests are in the source, unit-tested for completeness (`TestPodmanPinsComplete`).
