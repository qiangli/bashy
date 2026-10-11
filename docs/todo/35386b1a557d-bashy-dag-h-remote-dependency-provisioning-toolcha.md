---
id: 35386b1a557d
kind: feature
title: 'bashy dag -H: remote dependency provisioning (toolchains via binmgr, pushed from the local cache when offline)'
seq: 367
status: wontfix
priority: p2
labels:
    - remote
created: 2026-09-30T17:19:13.704223Z
sprint: 402
sprint_id: 107fb1ff-6f9c-5bc1-8896-a26f792a6fd8
sprint_title: 'bashy dag post-1.0: reusable task graphs and remote execution'
closed: 2026-10-11T06:53:32.740364Z
---

Scope (5 pt): before running on the remote, resolve the target's toolchains/managed tools there through binmgr; when the remote cannot reach the network, push the needed artifacts from the local binmgr cache over the peer channel (or use the #343 tunnel). Acceptance: red/green; e2e go test on an offline remote with Go pushed from the local cache.

2026-10-02 continuity: weave run #27 is abandoned, with one unmerged commit
on `agent/weave-issue-27`; `bashy weave status 27` reports an isolation
violation in the live checkout. The run is not acceptance evidence. The story
remains todo and its branch should be reviewed before any resumption or merge.

Rescope 2026-10-10 (operator, MVP per KISS / rod not fish): WONTFIX. Composable today from a dag -H target body: the remote bashy provisions its own toolchains through binmgr, and bashy sandbox can run inside a target. Sprint 402 keeps only the dag -H core story e7f7175b65b7.
