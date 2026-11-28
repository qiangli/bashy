---
id: 7c6c5782fe69
kind: feature
title: H5 Audit, override and policy for on-behalf decisions
seq: 3345
status: todo
priority: p2
labels:
    - hitl
created: 2026-11-28T14:03:09.382623Z
sprint: 745
sprint_id: 79f14d8a-2015-5a93-ba77-60cb7f70e108
sprint_title: 'Human-in-the-loop routing: a designated delegate answers ''needs you'' and reaches the human on their chosen channels'
---

Every delegate decision and human answer lands in the request record and the sprint thread with who, on whose behalf, which channel, when and the evidence. The human can list what was decided on their behalf, override a decision (which re-opens or reverts what the policy allows), and revoke a delegation at once. The sprint manage score and accept rules treat a delegate's in-scope answer as the human's; an out-of-scope answer by a delegate is refused, not silently accepted. Depends on H2.
