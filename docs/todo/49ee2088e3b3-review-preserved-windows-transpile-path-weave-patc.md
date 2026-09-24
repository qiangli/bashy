---
id: 49ee2088e3b3
kind: chore
title: Review preserved Windows transpile path weave patch
seq: 339
status: todo
priority: p2
labels:
    - windows
    - transpile
created: 2026-09-24T02:52:42.464369Z
sprint: 266
sprint_id: 16738595-c072-5838-be1b-603932c6df8e
sprint_title: Bashy shell, Coreutils, and BashSharp follow-up after v0.28.0
---

Killed Sprint 250 Bashy weave run #15 retains staged Story #321 work at ~/.bashy/weave/bashy-6497d06f/workspaces/issue-15: an MSYS path normalization implementation, focused tests, and a plan. Preserve its workspace and branch. Compare this draft against current released Bashy/main and the final Windows Go by Example/Tour receipts before deciding whether any repair remains. Acceptance: record the exact current-head reproducer or supersession, review callback/path semantics and tests if needed, then integrate only a proven minimal fix with unchanged fixtures and deadlines; otherwise dispose the run with a documented reason. Do not change v0.28.0 tags or assets.
