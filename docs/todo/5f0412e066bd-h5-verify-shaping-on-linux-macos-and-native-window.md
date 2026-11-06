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
sprint: 579
sprint_id: b3b68c8f-69de-5654-bab3-e0665d048fd4
sprint_title: 'Software shaping: platform and agent E2E validation'
---

Depends on H3, H4. On Linux, macOS and native Windows (not WSL): (a) shape Klatch with bashy shape: wish or --from, verify, approve (data kept), undo; (b) shape bashy itself by delivering Sprint 570 H1 (handoff show): wish/--from, verify, approve (side slot), undo, submit; (c) regression: Sprint 526 web shaping of the console app behaves as before. Negative: a failing candidate is never activated. Keep raw logs and identities. Acceptance: all cases pass on all three platforms with recorded evidence.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. One shaping model, two front-ends: the existing web /shape/<app>/ page and /api/shape/{app}, and a new bashy shape CLI mirroring that API verb for verb; both use the shared yoke/pkg/shape engine; bashy src stays the version/source plumbing underneath. Same verbs and authorization for humans, agents and agent-assisted humans. Candidates always built in an isolated workspace, never a shared checkout. Sprint 526 web behavior unchanged. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

Hello World tutorial/fixture: [walkthrough](../../examples/shaping/hello-world/README.md). Reuse pinned source in isolated runs; this documentation is not E2E pass evidence.
