---
id: 188896bb1660
kind: bug
title: mb send to an agent name with several live sessions is refused; a worker cannot address one conductor instance, and its own sender seat can be held by another session
seq: 3264
status: assigned
priority: p1
created: 2026-11-20T20:25:42.415345Z
assignee: agy-gemini4-carbon
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

Reported 2026-11-20 by muse (Sprint 678) via the operator: 'bashy mb send claude-opus5.5' refused (four live sessions share that name, none identifiable as the 678 seat from post metadata); 'mb send --as muse-wonder-full' refused because another session holds that seat. Workers fell back to role-addressed mail (mb:14789-14791, 14826-14828), which the conductor's inbox did not surface (S678 S1 DONE missed ~65 min). Related: agy reported the same ambiguity. Root cause first; candidate shapes: post metadata naming the conductor's sprint seat, addressing by seat (conductor:<sprint>) delivered to the seat holder's inbox, or instance labels. Acceptance: a worker can reach the conductor of sprint N directly and the conductor's inbox surfaces it.
