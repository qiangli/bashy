---
id: 9643d77f3a2c
kind: spike
title: 'Shapeware pilot on form: in-body known-issue block + one TBD spec, fresh-agent run'
seq: 3312
status: todo
priority: p2
labels:
    - shaping
created: 2026-11-25T10:17:49.05743Z
---

Follow-on to umbrella Sprint 712 (story b35b89620f88, plan only). This is its case (iii): a feature delivered without a new binary. Keep it KISS/MVP: prove the rod once, on form only.

1. Check that the Bash# block syntax (`~~~skill as guide`) accepts another block kind. If not, record the gap before changing anything.
2. Add one known-issue block INSIDE form.bsh (operator decision 2026-11-25: in-body, not a separate file), for bug 64ef2626: symptom, workaround (run form.bsh directly), class 'workaround active', expiry. It retires when 64ef2626 is fixed.
3. Add one TBD spec, from the shaped.json genie example 'Group unanswered required fields at the top', with a check under checks/ that fails today.
4. Fresh agent session, no hints: it should get past the bug in one step, and implement the TBD via `bashy shape wish` → verify → approve so that checks/ pass.

Measure: tool calls to get past the bug (baseline 2026-11-25: 5), whether checks pass, wall time. Results go back to Sprint 712. No new verbs and no framework.
