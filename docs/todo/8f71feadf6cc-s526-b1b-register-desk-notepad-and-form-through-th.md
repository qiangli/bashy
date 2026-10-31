---
id: 8f71feadf6cc
kind: refactor
title: S526.B1b Register desk notepad and form through the public shaped-app path
seq: 2969
status: done
priority: p1
created: 2026-10-31T00:23:37.986492Z
weave: 386
assignee: codex-gpt6-sol
sprint: 526
sprint_id: a9ea1d04-f39a-569f-8949-81dcb6aa864a
sprint_title: 'Software Shaping dogfooding: all eligible Bashy built-in apps'
closed: 2026-10-31T01:16:06.026973Z
closed_by: s526-manager
---

Split from B1 (plan wave 1, B1b). notepad and form at bashy/examples/desk/{notepad,form} already ship shaped.json, contract.bsh, register.sh, dag.md. Register both through the same public shaped-app path an external app uses (no builtin/privileged route), so a principal can shape them in the bashy app UI with per-user revisions, history and Genie preview. Any missing capability becomes a generic public extension, never an app-only shortcut. Acceptance: focused red/green test that both register via the public path and appear in bashy app list as shaped, plus one per-user revision round trip on each. Evidence docs/sprint-526-evidence.md.
