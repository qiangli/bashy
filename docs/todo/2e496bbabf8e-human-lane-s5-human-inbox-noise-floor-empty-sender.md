---
id: 2e496bbabf8e
kind: task
title: 'Human lane S5: human inbox noise floor - empty senders and duplicate broadcasts'
seq: 213
status: todo
priority: p1
created: 2026-09-05T15:47:51.864296Z
sprint: 126
---

'bashy inbox human' currently renders duplicated bus/fleet.unresolved broadcasts, three with an EMPTY sender (bus:3097, bus:3098, bus:3104). Gate: a test reproduces the empty-sender and duplicate rows against the current store and goes red first; after the fix duplicates collapse to one row carrying a count, empty-sender records are dropped at ingestion with the drop counted, and DIRECTED messages are never collapsed or dropped.

Implementation home: `bashy/internal/agentos` unified-mailbox ingestion and
rendering. The named bus records are local dhnt regression fixtures only.
