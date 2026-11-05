---
id: d25817fc29e4
kind: feature
title: 'H4 shape submit: hand shaped work into the sprint flow'
seq: 3074
status: todo
priority: p2
labels:
    - clean-baseline
    - shaping
created: 2026-11-05T01:04:41.955603Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

Depends on H2. bashy shape submit [--story ID] turns the shaped branch into a weave run linked to the story, records the shaped identity on the story, and goes through the unchanged review/merge gates. It interoperates with Sprint 570 offers (offer the shaped work to a reviewer or manager). Acceptance: a submitted shape is reviewable and mergeable like any weave run; the story shows the shaped identity; nothing merges without the normal gates.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. KISS/MVP: compose existing weave, self build/install, service and tool pin commands; no new framework or store; --json; same commands and authorization for humans, agents and agent-assisted humans. Shaping always in a weave workspace; swaps per host only; a failing shaped build is never used. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
