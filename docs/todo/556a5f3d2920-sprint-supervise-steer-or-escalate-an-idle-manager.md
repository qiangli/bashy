---
id: 556a5f3d2920
kind: feature
title: 'sprint supervise: steer or escalate an idle manager when attention is pending'
seq: 3103
status: todo
priority: p3
labels:
    - sprint
    - post-mvp
created: 2026-11-06T17:08:01.797913Z
---

Problem: run-terminal notices reach a manager only via bus.SteerLive with a CtlSock; checkSilentSprintManagers runs only inside weave heartbeat, which exits after 15m idle and covers only foreman seats. Behavior: a supervisor loop per doing sprint computes the attention queue every N minutes; steers a managed manager at turn boundary; for an unmanaged or silent manager past SLA, escalates to the operator (notify/mb) and offers successor take. Generalize the steward_supervise pattern to sprint seats. Verification: scenario replay - worker submits while manager idle; managed session steered within SLA; unmanaged session escalated. Baseline (S562 assessment): an operator-started Codex manager initiated 0 of 69 turns itself; 31.6h idle vs 18.5h active; worker results waited 4-9.5h for a reaction; 18 submissions pending 29-38h; 0 rejects; 5 self-accepts; all workers single-vendor internal subagents invisible to bashy.
