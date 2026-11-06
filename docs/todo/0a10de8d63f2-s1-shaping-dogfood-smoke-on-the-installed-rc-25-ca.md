---
id: 0a10de8d63f2
kind: test
title: S1 Shaping dogfood smoke on the installed rc.25 candidate
seq: 3149
status: todo
priority: p0
labels:
    - release
    - shaping
created: 2026-11-06T21:50:00.33224Z
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

Proves rc.25 is ready for shaping. On the dev host, from the installed frozen candidate (canonical installed path), run the hello-world CLI lifecycle (examples/shaping/hello-world): shape wish -> verify -> approve -> undo -> restore. Then one real shaped change through the promotion path: local shape -> shape submit --push to an org remote (P1) -> a teammate adopts via fetch + shape wish --from -> --gh opens the upstream PR. Record exact candidate SHA, installed binary digest, commands and results as one evidence record. A failure is tracked in the known-issues ledger, never reported as passing. Depends on P1 (if P1 misses the cut, record the local-only lifecycle and the push/PR legs as BLOCKED).
