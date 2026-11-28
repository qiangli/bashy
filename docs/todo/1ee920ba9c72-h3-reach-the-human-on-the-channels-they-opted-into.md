---
id: 1ee920ba9c72
kind: feature
title: 'H3 Reach the human on the channels they opted into: inbox and board attention, email, Klatch'
seq: 3343
status: todo
priority: p1
labels:
    - hitl
created: 2026-11-28T14:03:08.266142Z
sprint: 745
sprint_id: 79f14d8a-2015-5a93-ba77-60cb7f70e108
sprint_title: 'Human-in-the-loop routing: a designated delegate answers ''needs you'' and reaches the human on their chosen channels'
---

Per-human channel preferences: each channel opted in or out, a default channel, by urgency (for example: board only for low, email for normal, Klatch and email for urgent), quiet hours, digest vs immediate. Built-in channels: the human inbox plus an attention alert on the sprint board UI (always on), email (through a configured account; no credentials in the record; uses bashy's existing secret handling), Klatch message. A channel is an adapter behind one contract (send(request) -> delivery id; poll or webhook for replies), so the user can add or shape a method (for example SMS, Slack) as a bashy command or shapeware surface without changing core. Delivery state recorded per channel; failures retry with a bound and fall back to the next channel. Depends on H1.
