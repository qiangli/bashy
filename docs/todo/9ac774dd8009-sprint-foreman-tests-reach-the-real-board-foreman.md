---
id: 9ac774dd8009
kind: bug
title: Sprint/foreman tests reach the real board, foreman and supervisor
seq: 3128
status: todo
priority: p2
labels:
    - sprint
    - post-mvp
created: 2026-11-06T18:45:04.540533Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

Defect observed 2026-11-06 (S576 run bashy#27, story 561bac1b): running the sprint/agentos tests on a host with the real HOME posted test sprint stage announcements ('Test sprint directive', 'Instruct test', 'E2E directive sprint') from conductor/test-agent to the shared message board (mb 12986-12999), and created a real ~/.bashy/foreman/sprint-1-manager session that the host supervisor treated as a stalled manager and relaunched every minute from a /tmp test binary. Expected: sprint/foreman test helpers default to a temporary HOME, an isolated bus and announce off, so no test can reach the real board, foreman or supervisor regardless of where it runs. Acceptance: a guard test that fails if a sprint test publishes outside its temp HOME.
