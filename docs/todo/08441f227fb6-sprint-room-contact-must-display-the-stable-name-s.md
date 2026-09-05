---
id: 08441f227fb6
kind: task
title: Sprint room contact must display the stable name sprint <id>
seq: 221
status: done
priority: p0
created: 2026-09-05T20:03:02.216372Z
assignee: claude-sonnet5
sprint: 126
closed: 2026-09-05T21:31:32.575401Z
---

BUG / Sprint 126. Observed after manager startup: Sprint-facing output identified the dedicated contact only as "Meet room 25" / "meet #25 · bus conductor.126". The short room number is explicitly display-only, released and reusable; it is not the room identity. The new object-owned-room design requires the stable human name "sprint <id>", so this sprint must be presented as exactly "sprint 126". Measured current state: bashy meet show 25 --json already has state.name="sprint 126"; creation/migration is correct. The defect is that sprint/contact presentation drops that name. GOAL: every sprint-facing contact surface leads with the stable object name sprint 126 (it may additionally show meet #25 and bus conductor.126), while the durable meet Ref remains the machine identity. Preserve the same room Ref, transcript, owner routing, and name across pause, handoff, take/resume, and sprint title edits. Heal legacy room metadata in place; never open a replacement room. GATE: red tests prove current sprint show/status/start/contact output is room-number-only; after the fix it exposes sprint <id>, and lifecycle tests prove the room Ref/name remain stable. Scope: coreutils/pkg/{role,role/meetroom,weave} and existing tests. No new store, transport, room, or naming scheme.
