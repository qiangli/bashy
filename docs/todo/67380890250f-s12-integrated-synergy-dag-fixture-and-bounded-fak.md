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

Continuation 2026-10-11: AGY #33 exited -1 and Weave preserved its unfinished DAG skeleton as commit `79ef3f1970043e60a3d3a2ad830d995b033d4832` at `refs/salvage/abandoned-33` after guarded abandon. It is not accepted or merged. Codex Terra #34 owns the same story and may use that commit as starting material, then fix its gaps: no fake `echo` as an implementation turn, no first-random-todo selection, bounded deterministic fixture/test, event IDs and raw outcomes, actual current CLI syntax. Source-only DAG dry-run is insufficient. Preserve #33 failure evidence and original S12 live gates.

Manager review 10:42 UTC: Codex Terra #34 submitted `2c6b1d88`, but `event-ids.ndjson` is populated with hard-coded labels such as `pair-request` and `approval-pending` rather than IDs parsed from actual command receipts. The approval request also uses `|| true`, so it can silently fail. Sprint fail was recorded; unique source survives at `refs/salvage/abandoned-34`. Rework this SAME story using real IDs from raw CLI responses, fail-closed approval handling, and a test that asserts IDs originate in command output. If any current CLI emits no ID for a step, record that exact absence and omit an invented ID. Keep bounded fake turns and report live gates separately. Do not edit Sprint419 template files.

Manager recovery 10:51 UTC: GLM/Ycode #35 stayed on an external model connection for about ten minutes with no output, source edit or commit, then exited -1. Manager inspected child process, job logs and private Git tree and recorded the environment failure; `weave abandon 35 --disposition empty` removed only this empty workspace. No fixture source was delivered by #35. The same story is ready for a fresh bounded worker; the preserved #34 commit is still the source starting point, and the event-ID/confirm corrections above remain mandatory.
