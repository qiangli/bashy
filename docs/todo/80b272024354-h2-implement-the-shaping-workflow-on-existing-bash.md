---
id: 80b272024354
kind: feature
title: H2 Implement the shaping workflow on existing Bashy commands
seq: 3056
status: todo
priority: p2
labels:
    - shaping
created: 2026-11-04T06:51:47.644014Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

Depends on H1. Implement checkout, host-local build and test, swap and swap-back for a shaped app; a shaped build that fails its tests is never swapped in; the shaped identity is visible in service status. No new framework. KISS/MVP: reuse existing Bashy tool pin, build and service commands; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
