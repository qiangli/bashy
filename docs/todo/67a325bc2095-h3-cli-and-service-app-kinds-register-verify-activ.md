---
id: 67a325bc2095
kind: feature
title: 'H3 CLI and service app kinds: register, verify, activate per host'
seq: 3057
status: todo
priority: p2
labels:
    - shaping
created: 2026-11-04T06:51:48.400816Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

Depends on H2. Register CLI and service apps as shapeable (today app add --type shaped covers web only). Per-kind verifier checks: CLI = build + the app's declared test gate; service = build + tests + health check. Per-host activation on approve: bashy uses a side-slot binary with the release kept and PATH unchanged; Klatch's unit points at the shaped image with data kept. undo/restore returns to the previous or released version in one step. A failing candidate is never activated. Acceptance: activation and swap-back tested for both kinds, including an interrupted activation; status shows the active shaped identity.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. One shaping model, two front-ends: the existing web /shape/<app>/ page and /api/shape/{app}, and a new bashy shape CLI mirroring that API verb for verb; both use the shared yoke/pkg/shape engine; bashy src stays the version/source plumbing underneath. Same verbs and authorization for humans, agents and agent-assisted humans. Candidates always built in an isolated workspace, never a shared checkout. Sprint 526 web behavior unchanged. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
