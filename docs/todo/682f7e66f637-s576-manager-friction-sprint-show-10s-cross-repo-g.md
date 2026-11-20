---
id: 682f7e66f637
kind: enhancement
title: 'S576 manager friction: sprint show <10s; cross-repo goal add --story; conductor docs/todo edits do not trip weave isolation'
seq: 3251
status: done
priority: p1
labels:
    - agents
created: 2026-11-20T04:15:28.182674Z
assignee: agy-gemini4-carbon
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
closed: 2026-11-20T20:46:08.081875Z
closed_by: claude-opus5.5
---

Rescope 2026-11-19 (plan-s576 §MVP). Observed on the dev host: (1) bashy sprint show took 2-4 min (per-account resource probes) — make the probe async/cached or opt-in; (2) sprint goal add --story cannot resolve stories stored in submodule docs/todo (bashy, yoke, agent-bench) — 'no issue in the register'; (3) weave flagged runs #69, #1, #2 isolation-violated because the conductor created/edited docs/todo files in the live checkout — exclude conductor-owned docs/todo paths (or record them as conductor edits) from the isolation check. One story, three small fixes, unit tests each.
