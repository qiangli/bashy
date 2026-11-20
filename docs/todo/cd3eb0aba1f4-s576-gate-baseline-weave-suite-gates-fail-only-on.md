---
id: cd3eb0aba1f4
kind: feature
title: 'S576 gate baseline: weave suite gates fail only on failures new relative to the base commit'
seq: 3250
status: done
priority: p0
labels:
    - agents
created: 2026-11-20T04:15:27.993081Z
assignee: claude-opus5.5
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
closed: 2026-11-20T18:11:49.756629Z
closed_by: claude-opus5.5
---

Rescope 2026-11-19 (plan-s576 §MVP). Observed: every gated bashy weave pull (runs #113, #114, #116) failed on 4 tests that already fail on main (todo 66d98310) and was merged by hand with evidence. Shape: run the gate on the base commit too (or reuse a cached base result per base sha), report pre-existing failures, block only failures absent from the base. Reuse yoke's ratchet idea (test/known-failures.txt) where it fits. Unit tests: pre-existing failure passes the gate with a report; a new failure blocks.

Closed 2026-11-20 (S576 triage, conductor): Delivered: yoke e3efbcfa, merge f62a8943 (run #82); weaveGateRescore wired into the pull gate (pkg/weave/weave_impl.go), cached base result per base sha; on main, in rc.38 candidate. Merged-run evidence reused; no fresh tests.
