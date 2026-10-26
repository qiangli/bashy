---
id: 5407bf1fae08
kind: task
title: 'S507.0 integration: pin bashy to yoke + outpost with the Sprint 507 fixes (reattach auth/placement/image, Windows routing, userkubeconfig path)'
seq: 2726
status: assigned
priority: p0
created: 2026-10-26T06:35:50.746165Z
assignee: s507-manager
sprint: 507
sprint_id: 77d7ab0f-6aef-5bd3-876d-3f50e9b585ea
sprint_title: 'YC demo blockers: four bug fixes (dks reattach, executor image default, AMD VRAM on Windows, kubectl /c/ paths)'
---

Manager-owned integration story for Sprint 507 pin bumps. Acceptance: bashy go.mod + srcModulePins at yoke/outpost heads containing c02f9c44 and f6a1f01; go build ./... and the full internal/agentos package pass.
