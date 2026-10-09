---
id: 13b238a05214
kind: feature
title: 'dag -H: run a target against an SSHFS-mounted workspace (Sprint 344 integration)'
seq: 416
status: todo
priority: p2
labels:
    - remote
    - dag
created: 2026-10-09T03:22:36.47437Z
sprint: 402
sprint_id: 107fb1ff-6f9c-5bc1-8896-a26f792a6fd8
sprint_title: 'bashy dag post-1.0: reusable task graphs and remote execution'
---

Integrate dag -H with Sprint 344's SSHFS mount over the bashy peer channel. Instead of copying declared Sources, the remote target sees a scoped SSHFS mount of the invoking workspace; this covers undeclared or large inputs and sandbox bind-mounts. Sprint 344 owns the mount itself (upstream review, peer mount, sandbox e2e); this story owns only the dag wiring. Depends on Sprint 344 goal peer-mount and on story e7f7175b65b7. Post-v1.0: sshfs is out of bashy 1.0.0 (operator decision 2026-10-04, release checklist). Acceptance: a dag -H mount mode (spelling decided in this story) runs a target against the mounted workspace on a FUSE host; Generates land locally without a fetch step; on hosts without FUSE it falls back to content-hash sync and says so, never silently; teardown removes only the mount it created; red/green tests plus an e2e on a FUSE-capable test host; permissive-only dependencies (Sprint 344 constraint).
