---
id: ee4a7f9839fd
kind: task
title: 'builds: matrix fail-fast:false + zot v2.1.22'
seq: 1899
status: done
priority: p1
labels:
    - supply-chain
created: 2026-10-13T00:03:57.440819Z
assignee: muse-spark1.2
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T00:17:19.288176Z
closed_by: s434-manager
---

Small, fast. bashy builds/ only.
1. builds/_shared/bashy-build.yml: build matrix `fail-fast: false` (one failing leg must not cancel the others; qiangli/ollama run 38314047179 lost all build signal that way). Re-render ALL tools (builds/render.sh <each>) - the fork/ diff must be only that line.
2. builds/zot: bump UPSTREAM_TAG v2.1.21 -> v2.1.22 (latest stable; verify commit via git ls-remote https://github.com/project-zot/zot 'refs/tags/v2.1.22^{}', LICENSE Apache-2.0 at tag, GO_TOOLCHAIN = tag go.mod toolchain/go directive, review build-flag/ldflags diff vs v2.1.21, update README). Re-render.
Gate: builds/check.sh exit 0. Commit with trailers, sprint submit with SHA + gate output. Do NOT push to qiangli/* or tag (manager pushes). Do not touch other _shared files (another worker owns them).
