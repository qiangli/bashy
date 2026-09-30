---
id: 35386b1a557d
kind: feature
title: 'bashy dag -H: remote dependency provisioning (toolchains via binmgr, pushed from the local cache when offline)'
seq: 367
status: assigned
priority: p2
labels:
    - remote
created: 2026-09-30T17:19:13.704223Z
weave: 27
assignee: codex-gpt5.6-terra
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Scope (5 pt): before running on the remote, resolve the target's toolchains/managed tools there through binmgr; when the remote cannot reach the network, push the needed artifacts from the local binmgr cache over the peer channel (or use the #343 tunnel). Acceptance: red/green; e2e go test on an offline remote with Go pushed from the local cache.
