---
id: f2cb0585bacf
kind: feature
title: 'S583.13 Egress: git remote, handoff, feed, mirror/rclone, circle check --staged'
seq: 3144
status: todo
priority: p3
labels:
    - boundary
    - egress
    - boundary-later
created: 2026-11-06T21:45:57.152459Z
---

Egress and sharing. git commit/push and gh: remote must belong to the repo's circle (coordGuard, bashy/internal/agentos/coord.go). handoff/resume artifacts and feed posts carry their circle and are refused toward non-members; mirror, rclone, kopia destinations and loom/act-runner registration are bound to a circle; browser/fetch uploads checked. `bashy circle check --staged` runs per-circle deny patterns (e.g. personal/family identifiers never in a team repo, team identifiers never in public) and is called by the pre-commit hook when present; umbrella `script/scrub-gate.sh` stays unchanged as the public circle policy.
Depends on: S583.2, S583.4.
Acceptance: push to a remote outside the circle refused; handoff to a non-member refused; staged personal pattern in a team repo blocked.
