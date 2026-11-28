---
id: 237c8e0f3be1
kind: doc
title: 'M1b kb runbook shapeware-issue-to-spec: a bug, new feature or missing feature becomes a spec change'
seq: 3332
status: done
priority: p0
labels:
    - shapeware
created: 2026-11-28T06:50:09.896557Z
assignee: claude-opus5.5-s741
sprint: 742
sprint_id: 7dde34d4-f382-5aba-8ff5-071c5e3548b8
sprint_title: 'Shaping: bugs and issues become spec changes (issue-to-spec support)'
closed: 2026-11-28T08:31:22.662899Z
closed_by: claude-opus5.5-s741
---

Revives 734 L1 74240b60a085 (closed wontfix), on the M1a format. Classify: bug -> KI-n entry plus a failing example (tbd, resolves: KI-n); new feature -> tbd feature; missing/tbd feature someone needs -> Build request on the existing entry; anything undecided -> owner open question. Include the fix step: KI closed with fixed-by, version bump (KI patch, feature minor). Worked cases: classgo KI-3 (date range undefined) and the four 741 bugs. Write with bashy kb in umbrella, then generalize into bashy kb defaults. Depends on M1a.
