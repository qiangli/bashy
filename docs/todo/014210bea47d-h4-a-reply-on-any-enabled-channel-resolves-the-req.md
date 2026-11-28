---
id: 014210bea47d
kind: feature
title: H4 A reply on any enabled channel resolves the request, authenticated as that human
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

Board UI buttons for the request's options; inbox human answer ID CHOICE; email reply with a single-use signed token tied to the request and the human; Klatch reply in the request's thread. Each path verifies the human's identity for that channel, refuses replay and spoofing (a forwarded email or another member's Klatch reply does not count), and records which channel answered. The answer wakes the requester (notify plus the agent's inbox) so a waiting sprint manager continues. Depends on H1 and H3.
