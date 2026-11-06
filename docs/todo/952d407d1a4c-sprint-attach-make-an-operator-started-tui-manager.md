---
id: 952d407d1a4c
kind: feature
title: 'sprint attach: make an operator-started TUI manager wakeable and token-bearing'
seq: 3104
status: todo
priority: p3
labels:
    - sprint
    - post-mvp
created: 2026-11-06T17:08:01.948523Z
---

Problem: only foreman-launched managers receive BASHY_SPRINT_LEASE_TOKEN and a control socket; an interactive Codex/Claude seat produced 42 bypass and 5 override entries and could not be steered. Behavior: bashy sprint attach ID registers the running session identity and instance, provides the lease token, and exposes a steer path (PTY/control socket via the chat session package) so notices wake it. Verification: attached session receives a run-terminal steer; gated verbs pass without bypass. Baseline (S562 assessment): an operator-started Codex manager initiated 0 of 69 turns itself; 31.6h idle vs 18.5h active; worker results waited 4-9.5h for a reaction; 18 submissions pending 29-38h; 0 rejects; 5 self-accepts; all workers single-vendor internal subagents invisible to bashy.
