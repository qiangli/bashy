---
id: 80b272024354
kind: feature
title: 'H2 bashy shape verb: start, test, status, release'
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

Depends on H1. Implement bashy shape start <app> [--story ID] (weave workspace at the pin or main, branch shape/<id>), test (the app's declared gate in the workspace), status, and release (swap back), composed from existing commands. A failed test blocks use. Acceptance: verbs work for both app kinds in the workspace; no writes outside the workspace and the per-host slot; --json output.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. KISS/MVP: compose existing weave (workspace/branch), self build/install, service and tool pin commands; no new framework or store; --json; same commands and authorization for humans, agents and agent-assisted humans. Shaping always in a weave workspace (never a shared checkout); swaps affect only this host/user; a failing shaped build is never used; no satellite needed. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
