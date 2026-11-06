---
id: f2cb0585bacf
kind: feature
title: 'S583.13 Egress: git remote, handoff, feed, mirror/rclone, circle check --staged'
seq: 3144
status: todo
priority: p1
labels:
    - boundary
    - egress
created: 2026-11-06T21:45:57.152459Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Egress and sharing. git commit/push and gh: remote must belong to the repo's circle (coordGuard, bashy/internal/agentos/coord.go). handoff/resume artifacts and feed posts carry their circle and are refused toward non-members; mirror, rclone, kopia destinations and loom/act-runner registration bound to a circle; browser/fetch uploads checked. bashy circle check --staged runs per-circle deny patterns (e.g. personal names never in an org repo, org names never in public) and is called by the pre-commit hook when present; umbrella script/scrub-gate.sh stays unchanged as the public circle policy.
Depends on: S583.2, S583.4.
Acceptance: push to a remote outside the circle refused; handoff to a non-member refused; staged personal pattern in an org repo blocked.
