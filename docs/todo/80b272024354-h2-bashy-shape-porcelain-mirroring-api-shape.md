---
id: 80b272024354
kind: feature
title: H2 bashy shape porcelain mirroring /api/shape
seq: 3056
status: todo
priority: p1
labels:
    - shaping
created: 2026-11-04T06:51:47.644014Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

Depends on H1. Implement bashy shape <verb> <app> for status, wish (genie; --from <workspace> for hand-made changes), show, diff, verify, approve, reject, preview, suggest and incident, calling yoke/pkg/shape directly (no HTTP), plus history/undo/restore/release/offer/adopt as thin calls into bashy src. Optional --remote <console> drives another host through the same API. --json everywhere. Acceptance: each CLI verb produces the same state transitions, verifier evidence and records as its web endpoint (shared-engine tests); web shaping unchanged.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. One shaping model, two front-ends: the existing web /shape/<app>/ page and /api/shape/{app}, and a new bashy shape CLI mirroring that API verb for verb; both use the shared yoke/pkg/shape engine; bashy src stays the version/source plumbing underneath. Same verbs and authorization for humans, agents and agent-assisted humans. Candidates always built in an isolated workspace, never a shared checkout. Sprint 526 web behavior unchanged. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
