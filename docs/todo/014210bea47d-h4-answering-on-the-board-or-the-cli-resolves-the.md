---
id: 014210bea47d
kind: feature
title: H4 Answering on the board or the CLI resolves the request, authenticated as that human, and wakes the requester
seq: 3344
status: todo
priority: p1
labels:
    - hitl
created: 2026-11-28T14:03:08.814032Z
sprint: 745
sprint_id: 79f14d8a-2015-5a93-ba77-60cb7f70e108
sprint_title: 'Human-in-the-loop routing: a designated delegate answers ''needs you'' and reaches the human on their chosen channels'
---

The board alert offers the request's options (recommendation marked, optional reason); the CLI path is bashy approve. Each path verifies the answerer is the human (or their in-scope delegate, H2); another member's answer is refused; a second answer after resolution is refused (first decider wins). Records which surface answered. The answer wakes the requester (notify plus the agent's inbox; approve wait returns) so a waiting sprint manager continues. Deferred with H3's channels: single-use email/Klatch reply tokens and form links. critical: security-relevant. Depends on H1 and H3.
