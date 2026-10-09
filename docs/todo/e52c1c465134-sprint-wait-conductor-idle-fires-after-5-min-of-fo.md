---
id: e52c1c465134
kind: bug
title: 'sprint wait: conductor-idle fires after 5 min of foreman-log silence; the runbook threshold is 30 min'
seq: 360
status: done
priority: p3
labels:
    - sprint
created: 2026-09-30T13:37:09.838648Z
assignee: claude-opus5.5
sprint: 379
sprint_id: 908c2ac2-e7bc-55fe-86bd-d25046ac4684
sprint_title: 'bashy 1.0.0 feature list: bash + Bash# + Yoke'
closed: 2026-10-08T19:40:41.307195Z
closed_by: claude-opus5.5
---

Steward 2026-09-30 on Sprints #110/#319: 'bashy sprint wait N' returned 'conductor_idle: foreman log mtime 5m0s ago' several times while conductors were legitimately waiting on long background jobs (VSC preflight reruns, Linux leaves). kb:runbook-steward-supervising-conductors §3 treats 30 minutes without progress as the reassign threshold. Fix (KISS): idle threshold = 30m (optionally --idle D). Red/green: a 6-minute-quiet log does not fire; a 31-minute one does.
