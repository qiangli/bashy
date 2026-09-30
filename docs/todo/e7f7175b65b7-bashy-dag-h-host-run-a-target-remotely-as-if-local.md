---
id: e7f7175b65b7
kind: feature
title: 'bashy dag -H <host>: run a target remotely as if local (sync Sources, run, stream, fetch Generates)'
seq: 366
status: todo
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:12.148081Z
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Scope (8 pt): extend dag's existing Host:/--mesh dispatch: for -H, sync the target's declared Sources (content-addressed, changed files only) into a stable remote workspace over the peer channel, run the same dag target there, stream stdout/stderr and exit status, copy declared Generates back. Acceptance: red/green; e2e 'bashy dag -H novidesign.local test' on a small repo; second run transfers only deltas.
