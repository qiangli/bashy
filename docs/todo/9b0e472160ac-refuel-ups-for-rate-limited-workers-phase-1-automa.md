---
id: 9b0e472160ac
kind: feature
title: 'Refuel/UPS for rate-limited workers: phase 1 automatic handoff on exhaustion; phase 2 opt-in metered/local fallback relaunch'
seq: 3273
status: wontfix
priority: p2
created: 2026-11-20T23:29:00.578867Z
closed: 2026-11-20T23:38:35.675726Z
---

Operator idea 2026-11-20; conductor advice recorded. Depends on 6254c3c6 (live session state detection).
Problem: when a subscription worker hits its usage limit mid-story, the loss is not the wait but its context (what it tried, what it was about to do); forced reassignment starts cold.
PHASE 1 (recommended, no model, all vendors): on 'rate-limited' with reset farther than a threshold (default 30 min), bashy writes a deterministic handoff into the story: branch + commits, uncommitted diff summary, last screen tail, latest DESIGN/DONE board notes, last instruction, reset time. Manager then chooses: wait and resume the same agent ('bashy chat command <id> resume', S681) or reassign with the handoff. Acceptance: fixture-driven test from a muse 429 screen; handoff content is complete and contains no secrets.
PHASE 2 (opt-in, only if phase-1 data shows near-finished stories stranded): 'UPS' relaunch of the SAME tool on a metered API key or a local/pooled model via bashy's LLM gateway (cligw), then resume, primarily to write the handoff (finishing code only if the operator allows). Requirements: per-agent opt-in + hard budget cap; attribution of commits/ratings to the fallback model, never the original; per-vendor measured support for custom base URL and resume (vendor CLIs bind credentials at launch, so this is a relaunch, not a live swap). Non-goal: silent automatic spending.
