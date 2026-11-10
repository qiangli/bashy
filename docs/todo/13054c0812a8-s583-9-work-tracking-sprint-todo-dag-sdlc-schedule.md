---
id: 13054c0812a8
kind: feature
title: 'S583.9 Work tracking: sprint/todo/dag/sdlc/schedule/claim/steward circle partitions and zero-migration default'
seq: 3140
status: todo
priority: p3
labels:
    - boundary
    - sprint
    - boundary-later
created: 2026-11-06T21:45:56.538969Z
---

Work tracking. sprint, todo/issue, dag, sdlc, schedule, claim, and steward/conductor partition host stores by circle via yoke/pkg/scope.Resolve (~/.bashy/<tool>/ for default circle, ~/.bashy/<tool>/owner/ at 0700, ~/.bashy/<tool>/circles/<circle>/ for family/friend/team/guest: sprint queue.json, coord ledger, steward seat journal, user todo ring); repo stores take the repo binding (.bashy/circle). A sprint's StoryRoots must all belong to the sprint's circle; conductor:N addresses resolve only within it. Zero-migration legacy rule: unlabeled data in ~/.bashy/<tool>/ stays in place as the default circle with no bulk rewrite; cross-host story materialization preserves uuid identity and circle.
Depends on: S583.2.
Acceptance: team sprint invisible from family circle and vice versa; mixed-circle StoryRoots refused; existing board unchanged in default circle.
