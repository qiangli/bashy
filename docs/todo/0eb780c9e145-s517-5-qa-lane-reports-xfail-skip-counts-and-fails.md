---
id: 0eb780c9e145
kind: task
title: S517.5 QA lane reports xfail/skip counts and fails on an xfail without a sprint story and target date
seq: 2736
status: done
priority: p2
labels:
    - known-failures
created: 2026-10-27T17:04:27.99151Z
assignee: muse-conductor
sprint: 517
sprint_id: 4589176f-2dd8-5458-bf93-1a15066eb26c
sprint_title: 'known failures to zero: Windows fleet fences, engine cases on peers, transpile-lowered skip; no xfail without a story and a date'
closed: 2026-10-27T18:59:58.231753Z
closed_by: s517-manager
---

script/bashy-release-qa-lane.sh (umbrella) and the tour check.sh: print the known-failing and skipped counts in the summary; a cases.tsv xfail must reference a story id with a due date, else the gate fails. Keeps known failures from becoming permanent. No skips or xfail markers added; remove the case's xfail marker in bashsharp-tour cases.tsv in the same delivery once it passes. Native verification on the Windows host required (host-access file ~/.bashy/s514-windows-test-host, outpost ssh key auth). No hostnames in code/docs.
