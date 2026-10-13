---
id: 0dbac7bb6e47
kind: task
title: Resume headlamp verified build (GO_MODULE_DIR for src/backend + gated image job) once upstream fixes GO-2026-5064/5338/5622/5932
seq: 1903
status: todo
priority: p3
labels:
    - supply-chain
created: 2026-10-13T01:18:18.296632Z
---

Blocked on upstream (S434 story 02288146 closed blocked-on-upstream). Work so far: bashy branch s434/headlamp-8a3ca0e3 (d5d8e3ae + 8a3ca0e3: headlamp v0.45.0 declaration, BLOCKED_ON_UPSTREAM recorded, fail-closed). Needs shared Go kind support for a module in a subdirectory (headlamp src/backend/go.mod) and a gated image job between audit and release (check.sh fixed job set). outpost refuses headlamp (outpost 4f0b2b0, story bae318dd). Resume when govulncheck shows those 4 IDs fixed or unreachable.
