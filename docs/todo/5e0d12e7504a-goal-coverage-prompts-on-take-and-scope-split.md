---
id: 5e0d12e7504a
kind: enhancement
title: Goal coverage prompts on take and scope split
seq: 3111
status: todo
priority: p3
labels:
    - sprint
    - post-mvp
created: 2026-11-06T17:08:03.014063Z
---

Problem: after takeover no goal links were added (21 uncovered, all P0 included); after the S573 split goal e4 still listed C# and PowerShell. Behavior: take and descope edits prompt to link uncovered P0 stories and update affected goal items. Verification: unit tests. Baseline (S562 assessment): an operator-started Codex manager initiated 0 of 69 turns itself; 31.6h idle vs 18.5h active; worker results waited 4-9.5h for a reaction; 18 submissions pending 29-38h; 0 rejects; 5 self-accepts; all workers single-vendor internal subagents invisible to bashy.
