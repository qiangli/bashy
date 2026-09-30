---
id: e52c1c465134
kind: bug
title: 'sprint wait: conductor-idle fires after 5 min of foreman-log silence; the runbook threshold is 30 min'
seq: 360
status: todo
priority: p3
labels:
    - sprint
created: 2026-09-30T13:37:09.838648Z
sprint: 332
sprint_id: 5591ec3f-fa56-5047-a968-cd01097cee2a
sprint_title: 'bashy small improvements and bug fixes — #314 follow-up'
---

Steward 2026-09-30 on Sprints #110/#319: 'bashy sprint wait N' returned 'conductor_idle: foreman log mtime 5m0s ago' several times while conductors were legitimately waiting on long background jobs (VSC preflight reruns, Linux leaves). kb:runbook-steward-supervising-conductors §3 treats 30 minutes without progress as the reassign threshold. Fix (KISS): idle threshold = 30m (optionally --idle D). Red/green: a 6-minute-quiet log does not fire; a 31-minute one does.
