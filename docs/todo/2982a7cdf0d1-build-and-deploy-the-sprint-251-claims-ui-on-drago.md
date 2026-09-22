---
id: 2982a7cdf0d1
kind: chore
title: Build and deploy the Sprint 251 Claims UI on Dragon
seq: 315
status: assigned
priority: p0
labels:
    - app
    - release
created: 2026-09-22T18:32:54.263038Z
assignee: codex-gpt5.6-sol
sprint: 254
sprint_id: d5056fd7-8077-55ef-bab2-2d4484df5170
sprint_title: Deploy Sprint 251 Claims UI on Dragon
---

Build the current published Bashy source with the pinned yoke=9c011d2241ac270d1398091123b93f59fd01f085; atomically install it to Dragon's existing ~/.local/bin front door; restart the managed apps service while preserving --pair --port 22749 --bind lan; verify service status and that /api/sprint exposes panel id claims. Do not redesign the UI, alter pairing state, or change unrelated services.
