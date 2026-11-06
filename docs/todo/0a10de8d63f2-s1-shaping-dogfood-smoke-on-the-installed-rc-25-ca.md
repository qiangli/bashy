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

Proves rc.25 is ready for shaping. On the dev host, use the installed frozen candidate (at the canonical installed path) to run the hello-world CLI lifecycle from examples/shaping/hello-world: shape wish -> verify -> approve -> undo -> restore.

Then run one real round trip with the P1 rod (a130d972) between two hosts (named by role):
1. Host A shapes a change.
2. `shape outgoing` lists it (exit 3, --json).
3. `shape push --to org`.
4. Host B: `shape incoming --from org` lists it.
5. Host B: `shape pull --from org` (gated, from the side slot), then `shape undo`.
6. Host A: `shape push --to upstream` opens the upstream PR.

Record the exact candidate SHA, the installed binary digest, the commands and the results as one evidence record. A failure is tracked in the known-issues ledger and never reported as passing. If P1 misses the cut, record the local lifecycle, and record the rod legs as BLOCKED.
