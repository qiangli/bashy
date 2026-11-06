---
id: 561bac1b3623
kind: feature
title: 'sprint directive: standing operator rules and pre-approvals on the card'
seq: 3118
status: todo
priority: p1
labels:
    - sprint
    - operator
created: 2026-11-06T17:13:23.850858Z
assignee: muse-spark1.2
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

MVP: bashy sprint directive add|list|rm ID 'TEXT' --by OPERATOR [--preapprove defer:p2,p3]. Directives show in sprint show/continuity and are included in take/start/instruct payloads; a --preapprove directive is what retriage checks before auto-approving a defer/drop. Replaces private manager state files and repeated nudges. Verification: unit tests; directive visible in show and instruct payload. Baseline: S562 assessment (operator-started Codex manager: 0/69 self-initiated turns, 4-9.5h reaction lag, 18 submissions pending 29-38h, 0 rejects, 5 self-accepts, 10 planned stories grew to 41).
