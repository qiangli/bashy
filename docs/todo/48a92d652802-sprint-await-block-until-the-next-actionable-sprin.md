---
id: 48a92d652802
kind: feature
title: 'sprint await: block until the next actionable sprint event'
seq: 3102
status: todo
priority: p0
labels:
    - sprint
created: 2026-11-06T17:08:01.641993Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

MVP: bashy sprint await ID [--timeout D] [--json] returns on the first typed event: run terminal/failed, story submitted, submission pending past SLA (default 30m), story assigned with no run activity past SLA (default 60m), mail to the seat, lease near expiry. Each event carries story/run ids and a suggested verb (accept|reject|assign|reassign). Builds on sprint wait (internal/agentos/sprint_wait.go); replace thread-text substring matching with structured queue/run state. Out of MVP: supervisor daemon, attach, vendor spread. Verification: unit test per event; scenario: worker run finishes while manager blocked -> returns within seconds. Baseline: S562 assessment (operator-started Codex manager: 0/69 self-initiated turns, 4-9.5h reaction lag, 18 submissions pending 29-38h, 0 rejects, 5 self-accepts, 10 planned stories grew to 41).
