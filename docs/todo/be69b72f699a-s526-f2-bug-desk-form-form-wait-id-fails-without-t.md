---
id: be69b72f699a
kind: bug
title: 'S526.F2 bug: desk form ''form wait ID'' fails without the owner key (undocumented)'
seq: 3030
status: todo
priority: p1
created: 2026-10-31T05:05:57.297625Z
sprint: 526
sprint_id: a9ea1d04-f39a-569f-8949-81dcb6aa864a
sprint_title: 'Software Shaping dogfooding: all eligible Bashy built-in apps'
---

Operator first real use: 'form wait ID' fails the same way (needs the owner key) although the skill says it blocks until answered. bashy examples/desk/form. Fix: wait works for the owner who created the form without extra flags (or documents and resolves the owner key automatically). Acceptance: red/green test; skill text matches behavior.
