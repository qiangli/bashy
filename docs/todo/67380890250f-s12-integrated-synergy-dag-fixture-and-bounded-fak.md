---
id: 67380890250f
kind: test
title: S12 integrated synergy DAG fixture and bounded fake gate
seq: 1893
status: done
priority: p2
labels:
    - bashy
    - dag
created: 2026-10-11T09:39:43.518126Z
weave: 36
assignee: codex-gpt6-sol
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
closed: 2026-10-11T14:05:19.234167Z
resolution: fixed
closed_by: codex-gpt6-sol
---

Scoped implementation child of umbrella S12 07917b37185b. Add one reviewable examples/dag/synergy/dag.md and a bounded deterministic test/runner where all external agent turns and messenger ingress can be stubbed, but commands use actual Bashy meet dm, kb/todo, approval/notify/event interfaces. The DAG sequence is unpaired meet dm refusal -> paired sender input -> Claude plan into kb/todo -> Codex implementation -> approval request -> /approve -> done notification, with event IDs and raw outcomes. Manager owns claimed Linux full DAG and two-host/live agent checks, so no fabricated green. Use current Yoke S6 pairing at 687d595+, Bashy S16/S19 monitor; coordinate file ownership: examples/dag/synergy and new test file only, avoid S419 tmpl files and Codync examples. Verify focused GOWORK=off go test -short ./internal/agentos/... if feasible plus bashy dag syntax/graph check; report exact skipped live parts. Commit Sprint/Story/Story-ID/Agent trailers in private run, no push. Keep original S12 open.

Continuation 2026-10-11: AGY #33 exited -1 and Weave preserved its unfinished DAG skeleton as commit `79ef3f1970043e60a3d3a2ad830d995b033d4832` at `refs/salvage/abandoned-33` after guarded abandon. It is not accepted or merged. Codex Terra #34 owns the same story and may use that commit as starting material, then fix its gaps: no fake `echo` as an implementation turn, no first-random-todo selection, bounded deterministic fixture/test, event IDs and raw outcomes, actual current CLI syntax. Source-only DAG dry-run is insufficient. Preserve #33 failure evidence and original S12 live gates.

Manager review 10:42 UTC: Codex Terra #34 submitted `2c6b1d88`, but `event-ids.ndjson` is populated with hard-coded labels such as `pair-request` and `approval-pending` rather than IDs parsed from actual command receipts. The approval request also uses `|| true`, so it can silently fail. Sprint fail was recorded; unique source survives at `refs/salvage/abandoned-34`. Rework this SAME story using real IDs from raw CLI responses, fail-closed approval handling, and a test that asserts IDs originate in command output. If any current CLI emits no ID for a step, record that exact absence and omit an invented ID. Keep bounded fake turns and report live gates separately. Do not edit Sprint419 template files.

Manager recovery 10:51 UTC: GLM/Ycode #35 stayed on an external model connection for about ten minutes with no output, source edit or commit, then exited -1. Manager inspected child process, job logs and private Git tree and recorded the environment failure; `weave abandon 35 --disposition empty` removed only this empty workspace. No fixture source was delivered by #35. The same story is ready for a fresh bounded worker; the preserved #34 commit is still the source starting point, and the event-ID/confirm corrections above remain mandatory.

Manager independent verdict 2026-10-11 11:39 UTC: reviewed `1b0b35ce` scripts/tests for receipt-derived IDs, raw stdout/stderr/status, fail-closed approval and real Bashy commands. Private candidate `BASHY_E2E_BIN=<private binary> GOWORK=off go test -short -timeout 10m ./internal/agentos -run TestSynergyDAG -count=1 -v` passed. Managed merge gate at Bashy `2f6accf7` passed `go build ./...`, `go test ./internal/... ./tools/...`, and Bash 86/86. The first merge attempt was stale after an S20 target edit; the second found an inaccurate S20 recipe status header, fixed at `6abc1815`; the third passed. Weave auto-closed this scoped child, and `sprint accept` confirmed it was already closed. Original S12 `07917b37185b` remains open for exact Linux real-agent DAG, actual messenger ingress and two-host approval gates; no live result is inferred from this bounded fixture.

Manager rework 2026-10-11 14:04 UTC: a real Dragon Claude/Codex run returned a false green when both agents answered that they could not implement, but the fixture accepted a nonempty reply and marked the todo done. The same child is reopened under the Sprint 418 manager for a minimal exact-artifact check before approval, todo completion and notification; no new feature story was filed. The private concrete-spec rerun did create byte-exact `SYNERGY_OK\n`, while the original Linux/two-host acceptance remains open. Sanitized live outcomes: `/tmp/s418-s12-live/evidence/REPORT.md`. Rework source and negative regression belong to this child; acceptance of this source correction does not close original S12.
