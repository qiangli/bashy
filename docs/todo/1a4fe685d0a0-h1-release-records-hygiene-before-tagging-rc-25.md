---
id: 1a4fe685d0a0
kind: chore
title: H1 Release records hygiene before tagging rc.25
seq: 3152
status: todo
priority: p2
labels:
    - release
created: 2026-11-06T21:50:00.820564Z
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

Close rc.23 F7 94ac7c89208a with the rc.23 promotion receipts (promoted 2026-10-30). Explain the v1.0.0-rc.26-dev (a0b4a1e6) version identity reported by the outpost host build that replaced rc.24-dev (no such tag on origin) before tagging v1.0.0-rc.25-dev, so release identities stay monotonic and unambiguous.

## Evidence (H1, 2026-11-06)

- Audited rc.23 F7 story `94ac7c89208a`: collected existing receipts from repo (`git show v1.0.0-rc.23`, `release/tier-verdicts/51271e35.json`, `docs/plan-s560-s539-master-execution.md` lines 45/47–52/69, `.github/workflows/promote.yml`). Appended evidence section to `docs/todo/94ac7c89208a-s539-f7-promote-bashy-v1-0-0-rc-23-same-bytes-spot.md`. Maintained status as `todo` because spot check (SHA256 comparison and container digest equality) and announcement receipts are MISSING.
- Investigated version stamping rules and the origin of `v1.0.0-rc.26-dev` reported on the Windows test host (installed outpost reports `outpost version v1.0.0-rc.26-dev`; installed bashy reports `5.3.0(1)-bashy-v1.0.0-rc.26-dev (83f5a5a176f3c9dd827bd4796d257090298890fc)`). Verified across Makefile, GoReleaser, scripts, and runtime stamping that neither `83f5a5a1` nor `a0b4a1e6` exists in local repos (bashy/outpost/yoke/sh) or origin. Confirmed the build originated from an unpublished tree with VERSION/tag set manually or via a local GoReleaser tag. Analyzed semver sorting (`rc.25-dev` < `rc.26-dev`) and outpost downgrade guard implications; evaluated operator options (A: keep rc.25 and reinstall explicitly; B: skip to rc.27-dev), recommended Option A, and authored `docs/releases/v1.0.0-rc.25-version-identity.md`.
