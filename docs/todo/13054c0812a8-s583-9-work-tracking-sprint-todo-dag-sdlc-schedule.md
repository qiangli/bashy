---
id: 13054c0812a8
kind: feature
title: 'S583.9 Work tracking: sprint/todo/dag/sdlc/schedule/claim/steward circle partitions and migration'
seq: 3140
status: todo
priority: p1
labels:
    - boundary
    - sprint
created: 2026-11-06T21:45:56.538969Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Work tracking. sprint, todo/issue, dag, sdlc, schedule, claim, steward/conductor carry circles. Host stores partitioned ~/.bashy/<tool>/<circle>/ (sprint queue.json, coord ledger, steward seat journal, user todo ring); repo stores take the repo binding. A sprint's StoryRoots must all be in the sprint's circle; conductor:N addresses resolve only within it. Migration: unlabeled data = default circle, read in place, no bulk rewrite; cross-host story materialization keeps the uuid identity and the circle.
Depends on: S583.2.
Acceptance: org sprint invisible from home circle and vice versa; mixed-circle StoryRoots refused; existing board unchanged in default circle.
