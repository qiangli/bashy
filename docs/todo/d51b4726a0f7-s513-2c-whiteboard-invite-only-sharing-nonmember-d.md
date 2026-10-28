---
id: d51b4726a0f7
kind: feature
title: S513.2c whiteboard invite-only sharing, nonmember denial and revocation
seq: 2753
status: done
priority: p0
created: 2026-10-28T05:45:12.714056Z
weave: 1
assignee: muse-conductor
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
closed: 2026-10-28T06:45:43.491132Z
closed_by: s513-manager
---

Implement owner-controlled participant invitations and revocation for named whiteboards, server-enforced on state, action, event/poll and export paths. A nonmember including a guessed URL is denied; invited users can draw/note and see sync; revoked user loses reads/writes immediately. Preserve main/legacy board data and facilitator control, avoid capability leakage. Add focused Go and browser E2E including restart persistence and prefixed URLs. Scope examples/whiteboard only. No Matrix changes.
