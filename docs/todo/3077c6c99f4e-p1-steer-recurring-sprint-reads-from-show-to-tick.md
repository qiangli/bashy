---
id: 3077c6c99f4e
kind: task
title: 'P1: steer recurring sprint reads from show to tick'
seq: 228
status: todo
priority: p1
created: 2026-09-06T01:03:06.821367Z
sprint: 127
---

MEASURED: sprint show 126 = 38,371 B (~9,600 tokens), of which 25,881 B is the acceptance
text. sprint tick = 936 B (~234 tokens). RATIO 40x. On sprint 127, still 7x.

SPRINT 127 SCOPE: correct the existing workflow guidance and hinting only. A new
`--brief` output mode or any additional projection is a deferred feature.

`show` is the RIGHT read for a takeover — a successor needs the acceptance criteria in full.
It is the WRONG read for the tenth time in a shift, and it grows without bound as the thread
does. `sprint tick` already exists and covers the recurring read; the defect is that nothing
POINTS THERE. `show` remains the obvious verb, and a manager doing the honest thing —
re-reading the sprint each turn — pays ~10k tokens for a mostly-constant document.

THIS IS THE SAME PRINCIPLE AS docs/request-progress-reporting.md: coordination gets cheaper
by projecting better, not by sending more. The projection exists; the routing does not.

SMALLEST HONEST FIXES, in order of preference — pick, do not do all:
  1. `sprint show` prints a one-line footer naming `sprint tick <id>` as the per-turn read.
     Zero risk, and it is a HINT not a behaviour change.
  2. The conductor skill and THE TICK help block lead with tick for the recurring read and
     reserve show for takeover. (Partly done — verify it actually reads that way.)
  3. Consider `sprint show --brief` that omits acceptance + thread. Only if 1 and 2 prove
     insufficient; a third verb spelling is a cost of its own.

DO NOT truncate `show` itself. A takeover needs the whole thing, and a successor silently
handed a trimmed acceptance is exactly the handover defect sprint 127 is about.
