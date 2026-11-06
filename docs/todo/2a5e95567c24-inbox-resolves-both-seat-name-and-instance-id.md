---
id: 2a5e95567c24
kind: bug
title: Inbox resolves both seat name and instance id
seq: 3112
status: todo
priority: p3
labels:
    - sprint
    - post-mvp
created: 2026-11-06T17:08:03.165527Z
---

Problem: S562 handoff mail was routed to the instance id while the inbox looked up by name, so the incoming manager could not find it (4 operator prompts). Behavior: inbox and mb resolve messages addressed to a seat name or any of its instance ids. Verification: unit test with name- and instance-addressed messages. Baseline (S562 assessment): an operator-started Codex manager initiated 0 of 69 turns itself; 31.6h idle vs 18.5h active; worker results waited 4-9.5h for a reaction; 18 submissions pending 29-38h; 0 rejects; 5 self-accepts; all workers single-vendor internal subagents invisible to bashy.
