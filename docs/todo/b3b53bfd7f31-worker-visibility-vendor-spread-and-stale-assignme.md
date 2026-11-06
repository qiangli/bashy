---
id: b3b53bfd7f31
kind: feature
title: Worker visibility, vendor spread and stale-assignment reassignment
seq: 3108
status: todo
priority: p3
labels:
    - sprint
    - post-mvp
created: 2026-11-06T17:08:02.551868Z
---

Problem: workers were vendor-internal subagents not registered with bashy (agent list empty); a single quota pool exhausted the whole fleet at once; stories stayed assigned with no run activity for 25-31h. Behavior: alert when a manager has 0 registered active workers while ready P0/P1 stories exist; assign spreads across vendors/quota pools and warns past a share threshold; auto-reassign or escalate stories assigned with no run activity past SLA (extend sprintReassignTimedOut). Verification: unit tests for alert, spread warning and reassignment. Baseline (S562 assessment): an operator-started Codex manager initiated 0 of 69 turns itself; 31.6h idle vs 18.5h active; worker results waited 4-9.5h for a reaction; 18 submissions pending 29-38h; 0 rejects; 5 self-accepts; all workers single-vendor internal subagents invisible to bashy.
