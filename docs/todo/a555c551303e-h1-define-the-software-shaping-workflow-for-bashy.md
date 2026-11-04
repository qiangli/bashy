---
id: a555c551303e
kind: requirement
title: H1 Define the software-shaping workflow for Bashy apps
seq: 3055
status: todo
priority: p2
labels:
    - shaping
created: 2026-11-04T06:51:46.948421Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

Any Bashy user, on their own host (no satellite needed), shapes a Bashy app: checkout at the pinned commit, change, build and test on the host, swap the running app with data kept, one-step swap back to the release, recorded source and build identity; other hosts unaffected. Define commands, states and failure behavior using existing Bashy pin/build/service mechanisms. First worked example: the manual Klatch shaping on elephant (Sprint 563 F14: host git, two podman builds, image swap, secret copy). KISS/MVP: reuse existing Bashy tool pin, build and service commands; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
