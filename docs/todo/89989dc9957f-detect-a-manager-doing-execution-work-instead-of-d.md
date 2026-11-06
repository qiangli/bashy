---
id: 89989dc9957f
kind: enhancement
title: Detect a manager doing execution work instead of delegating
seq: 3109
status: todo
priority: p3
labels:
    - sprint
    - post-mvp
created: 2026-11-06T17:08:02.704566Z
---

Problem: the S562 manager ran 291 remote gate commands itself, personally performed host cleanup and serialized parallelizable lanes. Behavior: count manager-identity gate runs and commits versus dispatch/review actions per window; surface a delegate nudge in the attention queue when execution dominates. Verification: unit test over a synthetic thread/run ledger. Baseline (S562 assessment): an operator-started Codex manager initiated 0 of 69 turns itself; 31.6h idle vs 18.5h active; worker results waited 4-9.5h for a reaction; 18 submissions pending 29-38h; 0 rejects; 5 self-accepts; all workers single-vendor internal subagents invisible to bashy.
