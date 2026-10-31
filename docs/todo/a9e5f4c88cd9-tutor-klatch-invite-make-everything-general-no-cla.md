---
id: a9e5f4c88cd9
kind: refactor
title: 'tutor/klatch/invite: make everything general — no class, roster, host or schedule baked in'
seq: 3023
status: done
priority: p0
labels:
    - tutor
    - klatch
created: 2026-10-31T04:13:55.993725Z
weave: 405
assignee: codex-gpt6-astra
closed: 2026-10-31T04:30:44.907513Z
closed_by: s547-manager
sprint: 547
sprint_id: 1422bece-2466-50b8-986f-160a56a64f00
sprint_title: Teacher tools · tutor — AI-guided timeboxed sessions
---

tutor: make everything general — roles, audience and private content come from the session format (first role = owner), the default format is generic (session), and class-, meeting- and solo-style formats are examples only; no cohort, roster, host or schedule baked into code or docs. Tests cover meeting and solo formats and role-specific editions. Delivered in examples/tutor.
