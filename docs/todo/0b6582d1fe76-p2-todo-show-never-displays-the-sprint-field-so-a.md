---
id: 0b6582d1fe76
kind: task
title: 'P2: todo show never displays the sprint field, so a writer cannot confirm their own write'
seq: 230
status: todo
priority: p2
created: 2026-09-06T01:03:37.221501Z
sprint: 127
---

MEASURED: `todo add --sprint 127` associates correctly — the --json row carries
sprint=127 and the sprint board sees the item. But `todo show <id>` text output contains
ZERO mentions of sprint.

COST: believed the association had failed, ran a redundant `todo edit --sprint`, and only
established the truth by grepping the board. A write you cannot read back in the view you
naturally read is indistinguishable from a write that did not happen.

FIX: `todo show` prints the sprint when set. One line. The data is already loaded.

RELATED BUT SEPARATE, do not merge: there is no way to list a sprint stories from the todo
side at all. `todo list --sprint N` correctly errors "unknown flag" (verified: exit=1, loud,
honest — an earlier claim that it silently returned empty was a MEASUREMENT ERROR on my
part, from piping stderr to /dev/null). That is a missing capability, not a lie, and it
may not be worth adding since `sprint show` already lists them.
