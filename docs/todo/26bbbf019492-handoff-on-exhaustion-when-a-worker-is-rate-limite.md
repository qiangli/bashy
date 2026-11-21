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

## Rescope (operator, 2026-11-21): MVP, general for every bashy user

Promoted from Stretch to MVP in Sprint 688 (order: 6254c3c6 → this → 49cd45d9).
Exhaustion hits every vendor, on subscriptions and on prepaid credit / API keys,
so this is a product capability for all users, not a fleet fix. Design record:
umbrella `docs/sprint-688-master-execution-plan.md` §2a.

- Trigger on the classifier's outcome class, not vendor prose:
  - `window` with reset > threshold (default 30 m) → handoff; manager resumes
    after reset (`bashy chat command <id> resume`) or reassigns.
  - `window` with reset ≤ threshold → no handoff; wait, then resume.
  - `hard` (402, insufficient balance, or no reset) → handoff and immediate
    reassign to a flat-billed seat; the seat stays cooled until the user tops up.
- Works without a sprint: a standalone session gets the same handoff block in
  its session record.
- Workers are told to commit and push WIP at every green step, so the handoff
  only has to cover the last increment.
- Acceptance adds a 402 / insufficient-balance fixture and the shared user KB
  page "When your agent runs out of quota".
