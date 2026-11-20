---
id: cd3eb0aba1f4
kind: feature
title: 'S576 gate baseline: weave suite gates fail only on failures new relative to the base commit'
seq: 3250
status: todo
priority: p0
labels:
    - agents
created: 2026-11-20T04:15:27.993081Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

Rescope 2026-11-19 (plan-s576 §MVP). Observed: every gated bashy weave pull (runs #113, #114, #116) failed on 4 tests that already fail on main (todo 66d98310) and was merged by hand with evidence. Shape: run the gate on the base commit too (or reuse a cached base result per base sha), report pre-existing failures, block only failures absent from the base. Reuse yoke's ratchet idea (test/known-failures.txt) where it fits. Unit tests: pre-existing failure passes the gate with a report; a new failure blocks.
