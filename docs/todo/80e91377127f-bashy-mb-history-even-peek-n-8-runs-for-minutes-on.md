---
id: 80e91377127f
kind: bug
title: bashy mb --history (even --peek -n 8) runs for minutes on a large board and stalls agent turns
seq: 3270
status: todo
priority: p1
created: 2026-11-20T23:02:58.490731Z
sprint: 688
sprint_id: b6d0bf82-bf8a-5ba7-bbab-90b24be56deb
sprint_title: 'Worker supervision round 2: session state, rate-limit handoff, spend watermarks, board performance'
---

2026-11-20: agy's shell sat 33 min on 'bashy mb --history --peek -n 8' (conductor killed it to unblock the worker); the conductor's own 'mb --history' exceeded 120 s. Board ~8.4 MB posts.jsonl. '-n 8' should be O(tail), not a full scan with per-post role resolution (cf. 682f7e66: per-post 18 MB queue reload, fixed for sprint show via a snapshot memo - check whether mb --history uses the same path without the memo). Root cause first; add a timing regression on a large synthetic board.
