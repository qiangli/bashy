---
id: a37248198e5f
kind: bug
title: S562 compiled foreign scalar argument coercion parity
seq: 3093
status: assigned
priority: p0
created: 2026-11-05T04:53:05.707828Z
assignee: s562-foreign-scalars
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Preserve existing interpreted foreign argument behavior in compiled mode. Python boundary scalars calls annotated float as_float with quoted numeric string; interpreter foreignArgument parses float64 but lowered typed Go wrapper rejects literal string. Investigate documented typed/shell boundary and implement bounded lowering parity, preserving invalid-input errors and evaluating arguments once. Original fixture remains regression; no golden/source weakening. Native, run and compiled proof required.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: see existing story details and S562 board; no verified run or branch reference recorded.
