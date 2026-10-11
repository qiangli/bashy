---
id: 67380890250f
kind: test
title: S12 integrated synergy DAG fixture and bounded fake gate
seq: 1893
status: todo
priority: p2
labels:
    - bashy
    - dag
created: 2026-10-11T09:39:43.518126Z
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
---

Scoped implementation child of umbrella S12 07917b37185b. Add one reviewable examples/dag/synergy/dag.md and a bounded deterministic test/runner where all external agent turns and messenger ingress can be stubbed, but commands use actual Bashy meet dm, kb/todo, approval/notify/event interfaces. The DAG sequence is unpaired meet dm refusal -> paired sender input -> Claude plan into kb/todo -> Codex implementation -> approval request -> /approve -> done notification, with event IDs and raw outcomes. Manager owns claimed Linux full DAG and two-host/live agent checks, so no fabricated green. Use current Yoke S6 pairing at 687d595+, Bashy S16/S19 monitor; coordinate file ownership: examples/dag/synergy and new test file only, avoid S419 tmpl files and Codync examples. Verify focused GOWORK=off go test -short ./internal/agentos/... if feasible plus bashy dag syntax/graph check; report exact skipped live parts. Commit Sprint/Story/Story-ID/Agent trailers in private run, no push. Keep original S12 open.
