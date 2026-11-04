---
id: 67a325bc2095
kind: test
title: H3 Verify shaping Klatch on Linux, macOS and native Windows
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

Depends on H2. Shape Klatch on L, M and native W (not WSL): change, build, test, swap with data kept, swap back; negative: failing build not swapped in. Keep raw logs and identities. KISS/MVP: reuse existing Bashy tool pin, build and service commands; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
