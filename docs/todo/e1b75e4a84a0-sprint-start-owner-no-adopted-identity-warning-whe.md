---
id: e1b75e4a84a0
kind: bug
title: 'sprint start --owner: no adopted-identity warning when launching a managed session for another agent'
seq: 3122
status: todo
priority: p3
labels:
    - sprint
    - post-mvp
created: 2026-11-06T17:34:45.701765Z
---

Observed during S562 takeover: a Claude supervisor ran 'sprint start 562 --owner s562-codex-manager' (a registered Codex clone) to launch a managed Codex manager. Start succeeded and the managed session is codex, but output warned 'take the seat under YOUR name instead ... --tool claude', treating a delegated launch as identity adoption. Expected: when start launches a managed session whose tool matches the owner's binding, suppress the warning or state 'delegated launch by CALLER'.
