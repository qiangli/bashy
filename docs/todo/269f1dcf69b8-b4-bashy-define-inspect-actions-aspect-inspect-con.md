---
id: 269f1dcf69b8
kind: task
title: B4 bashy define + inspect actions aspect + inspect context counts surface the action facet
seq: 262
status: assigned
priority: p1
created: 2026-09-12T23:03:30.238493Z
weave: 7
assignee: qiangli
sprint: 161
---

Surface the action facet (coreutils C6).
- bashy define <word> prints the action facet in human and --json output for every concept that has one (no new subcommand — define must never gain one).
- bashy inspect actions [--json] [--kind command|script|agent|skill]: an ASPECT of inspect (Sprint 159 rule: a self-view is an aspect, never a verb) — the catalog of what this bashy can run, generic half only (no paths, no hosts), one row per action: kind identity contract latitude authority effects_declared executor.
- bashy inspect context --json gains actions: {command, script, agent, skill} counts only.
- Atlas inspect entry unchanged; docs/command-atlas.md and skills/bashy/SKILL.md teach inspect actions and the four families.
Tests: inspect_test.go facet rows for the four kinds; e2e bashy inspect actions --json parses and contains at least one of each family, using a test-scratch dag.md and scratch BASHY_SKILLS_DIR / BASHY_FLEET_DIR.
Gate: go test ./internal/agentos/...; e2e dispatch gate.
