---
id: 8baccf0ce8be
kind: feature
title: HITL external channels for needs-you requests (email, Klatch, Google, calendar, quiet hours, digests)
seq: 3347
status: todo
priority: p3
labels:
    - hitl
    - post-mvp
created: 2026-11-28T15:17:07.939709Z
---

Deferred from Sprint 745 H3/H4 by the operator 2026-11-28 (the sprint board alert is enough for now). When a real need appears: per-human channel preferences (opt-in, default, urgency, quiet hours, digest), a channel adapter contract (a registered bashy command: send REQUEST-JSON -> delivery id, poll), email (no SMTP today; Google gws rod email.send), Klatch (needs a participant command), Google Calendar/Chat, calendar availability, deadline reminders via bashy schedule, single-use reply tokens or form links. Prove through software shaping on live accounts.
