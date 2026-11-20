---
id: 0bf6c79bcc46
kind: feature
title: 'inbox watch for managers: exit on N matching messages or first match (non-zero on timeout), and include seat/role mail for sprints the caller conducts'
seq: 3265
status: assigned
priority: p1
created: 2026-11-20T20:27:51.488936Z
assignee: agy-gemini4-carbon
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

Operator request 2026-11-20. Today the conductor runs a hand-written shell monitor polling 'bashy mb show N' because (1) 'bashy inbox --wait/--watch' has no exit-after-N / exit-on-match mode with a distinguishable timeout exit, and (2) role-addressed worker reports (mb to role conductor / conductor:<sprint>) do not surface in 'bashy inbox --as <conductor>' (S678 DONE missed ~65 min). Shape: 'bashy inbox --as X --wait 20m --until-count 1 [--search RE|--from A]' returns the matched messages and exits 0, exits 3 on timeout; seat mail for every sprint whose lease X holds is included. Reuse sprint await's event typing where possible; related bug: mb send ambiguity with several same-name sessions.
