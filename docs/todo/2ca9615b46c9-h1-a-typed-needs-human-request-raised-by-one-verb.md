---
id: 2ca9615b46c9
kind: feature
title: H1 A typed needs-human request raised by one verb and shown on the board
seq: 3341
status: todo
priority: p0
labels:
    - hitl
created: 2026-11-28T14:03:07.077575Z
sprint: 745
sprint_id: 79f14d8a-2015-5a93-ba77-60cb7f70e108
sprint_title: 'Human-in-the-loop routing: a designated delegate answers ''needs you'' and reaches the human on their chosen channels'
---

One verb (for example bashy hitl ask, or an extension of inbox human send) that any agent or sprint manager uses to raise a request: kind (decision, approval, action, info), title, options with a recommendation, refs (sprint, story, commit, artifact), deadline, urgency, the requester's identity. Durable, one record per request, a state machine, idempotent re-raise. Listed by bashy sprint show / the board as 'Needs you' with age and owner; answerable by id. Replaces ad hoc 'Needs you' prose in manager reports. Depends on H0.
