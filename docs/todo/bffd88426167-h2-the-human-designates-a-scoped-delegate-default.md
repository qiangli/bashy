---
id: bffd88426167
kind: feature
title: 'H2 The human designates a scoped delegate (default: the steward); on-behalf answers are audited, overridable and revocable'
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

A human principal sets who acts on their behalf (an agent seat, default the host steward; reuse steward deputy scope and fencing) and what it may decide: by request kind, sprint/epic, risk tag, and pre-approved defaults (wire the existing sprint directive --preapprove matcher, which has no caller today). A closed default non-delegable set that only the human can widen: production deploy or push to a deployed branch, security disclosure, credentials and secrets, spending money, deleting data, changing who has access. Requests route to the delegate first; an in-scope answer is recorded as 'decided by DELEGATE on behalf of HUMAN' with its evidence (steward journal + sprint thread) and is visible to the human. Folded in from H5 (conductor decision 2026-11-28): the human can list what was decided on their behalf, override a decision (re-opens the request) and revoke a delegation at once; sprint accept/manage rules treat an in-scope on-behalf answer as the human's and refuse an out-of-scope one. Out-of-scope or unresolved requests escalate to the human on the board (H3). critical: security-relevant. Depends on H1.
