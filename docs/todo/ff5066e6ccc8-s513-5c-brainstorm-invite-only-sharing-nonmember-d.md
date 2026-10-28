---
id: ff5066e6ccc8
kind: feature
title: S513.5c brainstorm invite-only sharing, nonmember denial and revocation
seq: 2754
status: done
priority: p0
created: 2026-10-28T05:45:12.990952Z
assignee: s513-manager
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
closed: 2026-10-28T06:54:48.989736Z
closed_by: s513-manager
---

Implement owner-controlled participant invitations and revocation for named brainstorm sessions, server-enforced on state, write, vote, export and polling. Nonmember and guessed URL denied; invited distinct users retain private-writing semantics; revoked user immediately denied. Preserve legacy main board and old records. Add Go/browser E2E including restart and prefixed URLs. Scope examples/brainstorm only. No Matrix changes.
