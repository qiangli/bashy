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

Source mechanism: `provisionPodman` returns `cachedPodman` early, bypassing
`provisionDarwinHelpers`. The dispatch path can also return a flat cached
Podman from `resolveEngineBinary` without calling `provisionPodman`. Then
`applyManagedPodmanEnv` writes `containers.conf` into the absent helper
directory. Once helper recovery was added to dispatch, its first retry exposed
flat legacy `gvproxy` and `vfkit` cache files blocking binmgr's versioned
directories under those names. The verified managed downloads now use distinct
cache keys and leave legacy files intact.

Verified on clean public Bashy `529e0ac` with sh `111e307` and Tour `82c2f98`:
the focused `advanced/dockerfile` and `advanced/k8s` cases each returned 0
and matched their pinned transcripts; the full unchanged macOS Tour passed
**40/40**, zero failed, skipped, or known-failing. Full log SHA-256:
`21e098bf0aefdc6295226feac4960a5de24d8d00bc980272a181300ab6c8af0d`.
The private umbrella validation note records the candidate manifest and raw
evidence path. No fixture or deadline changed. Story #676 remains open for
the other host gates.
