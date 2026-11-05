---
id: 67a325bc2095
kind: feature
title: H3 Swap and swap-back for CLI and service apps
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

Depends on H2. Implement bashy shape use. For the CLI kind (bashy): the shaped binary goes in a side slot, the release is preserved, PATH is unchanged, and release reverts in one step. For the service kind (Klatch): the unit points at the shaped image with data kept, and release restores the pinned image. Status shows the shaped identity. A failing shaped build is never used. Acceptance: swap and swap-back tested for both kinds, including an interrupted swap.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. KISS/MVP: compose existing weave (workspace/branch), self build/install, service and tool pin commands; no new framework or store; --json; same commands and authorization for humans, agents and agent-assisted humans. Shaping always in a weave workspace (never a shared checkout); swaps affect only this host/user; a failing shaped build is never used; no satellite needed. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
