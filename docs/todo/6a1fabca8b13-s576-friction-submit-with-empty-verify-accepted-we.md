---
id: 6a1fabca8b13
kind: enhancement
title: 'S576 friction: submit with empty verify accepted; weave say not delivered to a working claude run'
seq: 3123
status: todo
priority: p2
labels:
    - sprint
    - post-mvp
created: 2026-11-06T18:02:18.473878Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

Observed while managing S576 (runs bashy#23, bashy#24, yoke#2): (1) sprint submit / weave worker submission records 'verify:' empty and is still offered for merge/closure; the manager then has no gate evidence and cannot rerun one (weave reverify/review rerun the recorded, empty, verify command). Refuse or flag a submission without verify evidence. (2) weave say to a 'working' claude run reports 'steering queued for run #N resume; no live delivery', so mid-run steering does not reach the worker. (3) A submitted run cannot be reworked (resume: no paused items; takeover: no takeover) — covered by f8fed0a7 sprint reject, noted here for the evidence.
