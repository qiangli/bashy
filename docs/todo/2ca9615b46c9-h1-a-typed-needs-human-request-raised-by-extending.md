---
id: 2ca9615b46c9
kind: feature
title: H1 A typed needs-human request raised by extending bashy approve, shown on the sprint board
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

Operator 2026-11-28: extend bashy approve, fewest subcommands; no new hitl verb or store. A request is raised through approve (existing 'approve request', extended, not argv-bound): kind (decision, approval, action, info), title, options with a recommendation, refs (sprint, story, commit, artifact), deadline, urgency, the requester's identity. Durable (not the 24h argv-approval TTL), one record per request, a state machine, idempotent re-raise by a request key. Listed by inbox human attention and bashy sprint show as 'Needs you' with age and owner; answerable by id with approve allow/deny or a chosen option. Replaces ad hoc 'Needs you' prose in manager reports. Route: shapeware-issue-to-spec, tbd feature on the lifted shapeware/hitl surface. Depends on H0.
