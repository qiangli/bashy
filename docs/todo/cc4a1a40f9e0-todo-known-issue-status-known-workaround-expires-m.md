---
id: cc4a1a40f9e0
kind: feature
title: 'todo: known-issue status (known/workaround/expires); move #3230 into it as KI-1'
seq: 3315
status: todo
priority: p1
labels:
    - shaping
    - todo
created: 2026-11-25T15:44:09.785979Z
sprint: 721
sprint_id: 69fc2c51-8faf-58e7-83ac-f27007837ae7
sprint_title: 'Shapeware pilot on form: spec v0 layout, one-step launch'
---

Operator-approved follow-on from umbrella Sprint 712 (Q6; docs/sprint-712-recommendation.md section 10, tabletop section 5(i)).

Problem: `bashy todo` has no state for "known, worked around, tracked until it expires". So #3230 (yoke c78fb76d: first-navigation net::ERR_ABORTED on Windows Chrome-for-Testing, one bounded retry) was closed as wontfix, which hides it.

Smallest change: a todo status for known issues (name to settle: `known`), carrying `workaround` and `expires` fields, that does not count as open work but is listed as known and is flagged when expired. Then move #3230 into it, with the KI-1 text from the recommendation section 5(i).

Acceptance: unit tests for the status (create, list, expiry flagged); #3230 shows as known with an expiry; existing statuses unchanged. KISS: no feed and no field network (those stay in the backlog).
