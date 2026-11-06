---
id: c7709b8aa64d
kind: feature
title: 'sprint brief: operator-facing status without interrupting the manager'
seq: 3119
status: todo
priority: p3
labels:
    - sprint
    - operator
    - post-mvp
created: 2026-11-06T17:13:23.99423Z
---

Problem: the S562 operator asked 'status' ~20 times, each forcing a manager turn. Behavior: bashy sprint brief ID prints one screen: closed/in-flight/blocked, needs-you (pending approvals, decisions), scope growth and untriaged count, stale items past SLA, manager last action age, confidence-qualified ETA; optional periodic digest to the operator via notify. Read-only. Verification: golden output test on a fixture card.
