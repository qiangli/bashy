---
id: ae9b5ef7e7e8
kind: task
title: Freeze the Go 1.27.1 Linux Profile B candidate
seq: 33
status: done
priority: p0
created: 2026-09-03T00:23:52.704875Z
assignee: claude-opus5.5
sprint: 110
closed: 2026-09-30T13:04:22.277479Z
closed_by: claude-opus5.5
---

Build the certification-shaped Linux artifact with Go 1.27.1; record its commit pins, toolchain, build inputs, checksum, and staged Profile B manifest so all later evidence names one exact SUT.

## Review 2026-09-30 (steward)

- Status: not started; still valid and the first product step of Sprint 110. Go 1.27.1 is already the pinned toolchain (`toolchain go1.27.1` in bashy and sh go.mod since af93afe, 2026-09-07), so this is a freeze of today's heads, not a toolchain migration.
- Outdated: "Profile B" only. Since the 2026-09-10 boundary note, Sprint 110 owns the FULL POSIX arm (all 117 sets, including the coreutils providers), so the manifest must stage Profile D (Bashy shell + Bashy utilities) and Profile B (Bashy shell + GNU userland) from the same build. The "pre-Go-1.27" reference no longer exists; the current reference point is the `sprint-269` tag and bashy v0.31.0 (c5d7ad2).
- Next step: choose one candidate = pushed heads of bashy/sh/coreutils/yoke (umbrella pins 2026-09-30: bashy 178dc5b, sh 55289834, coreutils dc805500, yoke fb99202); on the Linux certification runner build static linux/amd64 `cmd/bash` and the coreutils multicall with go1.27.1; record commits, `go version -m`, build flags, sha256 and both staged manifests in ONE evidence record.
- Acceptance: one manifest names exactly one SUT digest; a rebuild from the recorded pins reproduces it; every later Sprint 110/100 story cites that digest.
- Depends on: 1727b5446a3b (runner). Blocks: 34046901d606, 401837b6352f, f093f2d6bba7 and the Sprint 100 remeasurement.
- Addendum (certification-body guidance, 2026-09-30): amd64 and arm64 are NOT assumed to be one product family - each architecture is certified separately unless a family application is accepted. Freeze and manifest ONE architecture per candidate (linux/amd64 = the controlled target); an arm64 candidate is a separate, separately budgeted freeze, only if the owner decides to certify arm64.
