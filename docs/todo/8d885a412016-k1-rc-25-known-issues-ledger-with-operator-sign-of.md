---
id: 8d885a412016
kind: doc
title: K1 rc.25 known-issues ledger with operator sign-off
seq: 3150
status: todo
priority: p0
labels:
    - release
created: 2026-11-06T21:50:00.494044Z
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

Supersedes rc.23 F0 fb86f77012fd. One table in the rc.25 release notes: rc.23 carry-overs 2ff43d03b869 (F8 sandbox island), 5585a1e08aff (F6 guard network escape), 73f5b066f684 (F1 git push -q), 5c07e9f0d9b0 (inherited SIG_IGN; check against the Darwin CGO_ENABLED=0 change), 91cc382c185d (Darwin lane, likely stale: reconcile or close); every BLOCKED leg from E1 and S1 with its owning sprint (565, 568, 579); every pre-existing layer-ratchet failure from Sprint 575 S0 (e.g. TestCoreImportBoundary). No fixes in this story: post-release fixes arrive as shaped changes. Operator signs off the ledger before promotion.
