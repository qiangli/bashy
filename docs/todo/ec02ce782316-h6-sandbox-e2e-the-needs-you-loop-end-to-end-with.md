---
id: ec02ce782316
kind: test
title: 'H6 Sandbox e2e: the needs-you loop end to end with fakes'
seq: 3346
status: todo
priority: p2
labels:
    - hitl
created: 2026-11-28T14:03:09.915188Z
sprint: 745
sprint_id: 79f14d8a-2015-5a93-ba77-60cb7f70e108
sprint_title: 'Human-in-the-loop routing: a designated delegate answers ''needs you'' and reaches the human on their chosen channels'
---

Operator 2026-11-28: e2e tests close the sprint; live accounts are proven later through software shaping where/when it best tests the features. Sandbox e2e (fake humans and agents, minutes, configurable waits) on the gate host: a manager raises the three item shapes from ClassGo 741 (deploy hotfix: non-delegable; owner review of lifted specs: delegable only if scoped; security-hotfix push: non-delegable); the in-scope item is answered by the steward and logged on-behalf; out-of-scope items appear as Needs-you on the board; the human answers on the board; a spoofed answer by another member is refused; the waiting manager resumes; override and revoke work. Depends on H2, H3, H4.
