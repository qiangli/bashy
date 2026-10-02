---
id: 35386b1a557d
kind: feature
title: 'bashy dag -H: remote dependency provisioning (toolchains via binmgr, pushed from the local cache when offline)'
seq: 367
status: todo
priority: p2
labels:
    - remote
created: 2026-09-30T17:19:13.704223Z
sprint: 345
sprint_id: f8f1645d-6d4d-53ee-97b3-0b61f720c5bb
sprint_title: Complete bashy dag -H remote execution and sandbox path
---

Scope (5 pt): before running on the remote, resolve the target's toolchains/managed tools there through binmgr; when the remote cannot reach the network, push the needed artifacts from the local binmgr cache over the peer channel (or use the #343 tunnel). Acceptance: red/green; e2e go test on an offline remote with Go pushed from the local cache.

2026-10-02 continuity: weave run #27 is abandoned, with one unmerged commit
on `agent/weave-issue-27`; `bashy weave status 27` reports an isolation
violation in the live checkout. The run is not acceptance evidence. The story
remains todo and its branch should be reviewed before any resumption or merge.
