---
id: 88d35c8688f5
kind: feature
title: Attention queue and SLAs on sprint show, tick and checkpoint
seq: 3105
status: todo
priority: p3
labels:
    - sprint
    - post-mvp
created: 2026-11-06T17:08:02.096281Z
---

Problem: S562 logged 32 checkpoints with primary metric +0 and 21 uncovered stories untouched; P0 focus stale 25h; P0 next never dispatched while p2 work was staffed. Behavior: an attention queue at the top of show/tick/checkpoint lists pending-submission age, assigned-not-dispatched, failed-unsalvaged runs, idle workers, uncovered stories, priority inversion, each with SLA and suggested verb. checkpoint with metric +0 and a non-empty queue requires an action or recorded reason. Liveness counts actions, not mail reads. Verification: unit tests for each queue item and the checkpoint guard. Baseline (S562 assessment): an operator-started Codex manager initiated 0 of 69 turns itself; 31.6h idle vs 18.5h active; worker results waited 4-9.5h for a reaction; 18 submissions pending 29-38h; 0 rejects; 5 self-accepts; all workers single-vendor internal subagents invisible to bashy.
