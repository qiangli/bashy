---
id: bffd88426167
kind: feature
title: 'H2 The human designates a delegate (default: the steward) with an explicit scope'
seq: 3342
status: todo
priority: p0
labels:
    - hitl
created: 2026-11-28T14:03:07.504292Z
sprint: 745
sprint_id: 79f14d8a-2015-5a93-ba77-60cb7f70e108
sprint_title: 'Human-in-the-loop routing: a designated delegate answers ''needs you'' and reaches the human on their chosen channels'
---

A human principal sets who acts on their behalf (an agent seat, default the host steward) and what it may decide: by request kind, project/sprint, risk tag, and pre-approved defaults (reuse sprint directive semantics). A default non-delegable list that only the human can widen: production deploy or push to a deployed branch, security disclosure, credentials and secrets, spending money, deleting data, changing who has access. Requests route to the delegate first; an in-scope answer is recorded as 'decided by DELEGATE on behalf of HUMAN' with its evidence and is visible to the human, who can override within a window or revoke the delegation. Out-of-scope or unresolved requests escalate to the human (H3). Depends on H1.
