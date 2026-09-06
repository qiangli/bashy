---
id: d744ac1a4552
kind: task
title: 'P1: make displayed story IDs valid commit-provenance input'
seq: 231
status: todo
priority: p1
created: 2026-09-06T01:03:37.24574Z
sprint: 130
---

MEASURED: `todo list` prints an 8-character id (e.g. 39748beb). The commit-msg hook
requires the full 12 and refuses the commit:

SPRINT 127 SCOPE: repair the contradiction between two existing surfaces and add a
regression test. Do not add a new trailer format, provenance store, or command option.

  sprint commit-msg: commit provenance: Story-ID must be the full 12-character
  lowercase hex id, got "39748beb"

The full id appears only in `todo show` and in `sprint goal add` output. COST: one refused
commit and a hunt for the real id, while holding a value that looked complete.

THE REFUSAL IS GOOD — it names the required form and gives an example. The defect is
upstream: a display id that cannot be pasted into the gate the same tool enforces.

TWO CANDIDATE FIXES, and the second is probably right:
  1. `todo list` prints the full 12. Costs table width.
  2. The hook accepts an unambiguous PREFIX, the way git does everywhere else — and the
     way this tool ALREADY does: `todo show 39748beb` and `todo edit 39748beb` both
     resolve the short form. Only the hook does not. Resolution machinery already exists;
     the hook is the outlier.

Prefer 2 unless there is a reason the trailer must be a stable full id for later lookup —
in which case the hook should RESOLVE the prefix and say so, not refuse.
