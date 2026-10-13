---
id: fff8880d90e6
kind: bug
title: Repair Darwin hosted Ollama no-egress sandbox profile
seq: 1929
status: assigned
priority: p1
created: 2026-10-13T18:25:50.148616Z
assignee: s436-codex-manager
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
---

Hosted run 38445979881 builds Darwin arm64, then native no-egress gate exits 65 before protocol proof because sandbox-exec rejects remote ip 127.0.0.1:18997 (host must be * or localhost). Use accepted localhost:18997 profile and verify the exact port is allowed while adjacent port is denied; retain deny controls and fail-closed verdict. Rerun five-target hosted gate.
