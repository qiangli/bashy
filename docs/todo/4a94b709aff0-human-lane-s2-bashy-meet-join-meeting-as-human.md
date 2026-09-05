---
id: 4a94b709aff0
kind: task
title: 'Human lane S2: bashy meet join <meeting> --as <human>'
seq: 210
status: todo
priority: p0
created: 2026-09-05T15:47:51.797421Z
sprint: 126
---

'bashy meet join' does not exist: invite is organizer-only, observe is read-only, tell presumes a seat. Add ONE verb that seats a human in a known meet. Gate: join a running meet you did not organize; receive the transcript so far; 'meet tell' lands in the transcript; 'meet show' lists you as a participant; the seat is released on exit and the room card reflects it. Reuse invite/observe/tell internals - no new store, no new transport.

Implementation home: `coreutils/pkg/meet`, exposed by bashy's existing meet
front door. Reconcile with coreutils story `0892cb5be1d3` (carried from dhnt
Sprint 99): extend that one explicit join implementation to accept a human
principal. Preserve its invariant that a durable seat never mints ephemeral
agent liveness; do not build a parallel human-only join path.
