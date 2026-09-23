---
id: 0fa44d46b0fc
kind: bug
title: S250 recover managed macOS Podman helpers on cached binary
seq: 325
status: assigned
priority: p0
labels:
    - macos
    - podman
created: 2026-09-23T12:10:57.146443Z
weave: 18
assignee: qiangli
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

Mac final-product candidate Bashy 8a68fab1/sh 4888f8d7, Tour 82c2f98:
unchanged full BashSharp Tour is 38 pass/2 fail/0 skip/0 XFAIL.
`advanced/dockerfile` and `advanced/k8s` return exit 0 but emit
`bashy podman: open $HOME/Library/Caches/bashy/bin/podman-helpers/containers.conf: no such file or directory`,
differing from pinned transcripts. Earlier candidate 5353ba3 passed 40/40 on
the same host while that helper cache directory existed.

Source mechanism: `internal/agentos/engines_podman.go` `provisionPodman`
returns `cachedPodman` early, bypassing `provisionDarwinHelpers`;
`applyManagedPodmanEnv` then calls `writeDarwinConfOverride`, which fails when
the helper directory is absent. Repair managed macOS cache recovery narrowly,
with a focused test for cached Podman and missing helpers. Preserve the original
transcripts, deadlines, and Podman behavior.

Verify both focused cases and the full unchanged Mac 40-case Tour against a
rebuilt public-head candidate. Coordinate the Bashy head change with Linux
and Windows final gates. Raw log:
`/Users/noviadmin/s250-8aaa6171-evidence/final-product-8a68fab1/logs/bashsharp-tour.log`,
SHA-256 `8fad09f0b7476cba8f16a59b753603f9dca1a5272614b9e7907d8957af7aa5c4`.
Story #676 remains open.
