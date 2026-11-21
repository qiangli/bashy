---
id: 26bbbf019492
kind: feature
title: 'Handoff on exhaustion: when a worker is rate-limited past a threshold, bashy writes a deterministic handoff into its story'
seq: 3275
status: todo
priority: p2
created: 2026-11-20T23:38:23.521946Z
sprint: 688
sprint_id: b6d0bf82-bf8a-5ba7-bbab-90b24be56deb
sprint_title: 'Worker supervision round 2: session state, rate-limit handoff, spend watermarks, board performance'
---

Conductor recommendation adopted by the operator 2026-11-20 (instead of the shelved refuel/UPS idea 9b0e4721). Depends on 6254c3c6 (live session state detection). On 'rate-limited' with reset farther than a threshold (default 30 min): append to the story branch + commits, uncommitted diff summary, last screen tail, latest DESIGN/DONE board notes, last instruction, reset time; no model call, no spend, no secrets. Manager then resumes the same agent after reset ('bashy chat command <id> resume') or reassigns with the handoff. Tests: fixture from a real 429 screen; handoff completeness; secret redaction.
