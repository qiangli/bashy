---
id: 753a4331ea22
kind: spike
title: Review sshfs-go fit for Bashy peer SFTP and FUSE hosts
seq: 378
status: todo
priority: p1
created: 2026-09-30T22:37:42.149122Z
sprint: 344
sprint_id: e6708b39-af54-589c-94b7-0c18e222bf57
sprint_title: SSHFS over the Bashy peer channel for remote workspaces and sandboxes
---

Inspect https://github.com/nxsre/sshfs-go at a pinned commit: license, API, dependencies, maintenance, key authentication, host-key verification, FUSE requirements, Docker plugin assumptions, and Windows/macOS/Linux support. Compare with Bashy embedded peer/SFTP and existing source-sync path. Deliver a short design decision and supported-host test matrix before integration.
