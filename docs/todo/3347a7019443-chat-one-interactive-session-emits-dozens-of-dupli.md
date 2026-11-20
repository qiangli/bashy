---
id: 3347a7019443
kind: bug
title: 'chat: one interactive session emits dozens of duplicate join events'
seq: 3253
status: todo
priority: p2
created: 2026-11-20T11:16:44.372357Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

2026-11-20 10:58:49-56Z the room timeline recorded ~40 join events for the single live claude interactive session (bashy claude), all same actor/target. Suspect publishGeometry (OnResize) re-joining the card on every SIGWINCH. Inflates the 58 MB host timeline every reader scans. Root cause first.
