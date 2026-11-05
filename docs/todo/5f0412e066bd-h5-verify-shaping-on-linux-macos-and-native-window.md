---
id: 5f0412e066bd
kind: test
title: H5 Verify shaping on Linux, macOS and native Windows
seq: 3075
status: todo
priority: p2
labels:
    - clean-baseline
    - shaping
created: 2026-11-05T01:04:42.119414Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

Depends on H3, H4. On Linux, macOS and native Windows (not WSL): (a) shape Klatch: change, build, test, use with data kept, release; (b) shape bashy itself by delivering Sprint 570 H1 (handoff show) as the dogfood case: start, test, use, release, submit. Negative: a failing shaped build is never used. Keep raw logs and identities. Acceptance: both cases pass on all three platforms with recorded evidence.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. KISS/MVP: compose existing weave, self build/install, service and tool pin commands; no new framework or store; --json; same commands and authorization for humans, agents and agent-assisted humans. Shaping always in a weave workspace; swaps per host only; a failing shaped build is never used. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
