---
id: fff8880d90e6
kind: bug
title: Repair Darwin hosted Ollama no-egress sandbox profile
seq: 1929
status: wontfix
priority: p1
created: 2026-10-13T18:25:50.148616Z
assignee: s436-codex-manager
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T18:41:58.174194Z
---

Hosted run 38445979881 builds Darwin arm64, then native no-egress gate exits 65 before protocol proof because sandbox-exec rejects remote ip 127.0.0.1:18997 (host must be * or localhost). Use accepted localhost:18997 profile and verify the exact port is allowed while adjacent port is denied; retain deny controls and fail-closed verdict. Rerun five-target hosted gate.

Merged into Sprint 436 parent story 732cbb22d353 (signed embedded runner shipped). Preserve existing commits, fork tags, failed-run evidence and this checklist under the parent; this child is not independently accepted. Required release targets are Linux amd64/arm64, Darwin arm64, and Windows amd64; Darwin amd64 is optional. The operator restored Windows to Sprint 436 scope.
