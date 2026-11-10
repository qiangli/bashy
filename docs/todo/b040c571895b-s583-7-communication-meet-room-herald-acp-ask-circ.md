---
id: b040c571895b
kind: feature
title: 'S583.7 Communication: meet/room/herald/acp/ask circle membership and cross-host relay'
seq: 3138
status: todo
priority: p1
labels:
    - boundary
    - comms
created: 2026-11-06T21:45:56.236764Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Communication part 2: meet, room, herald/a2a, acp, ask. Meetings and host rooms carry their circle (yoke/pkg/meet session State, yoke/pkg/room; owner/family/friend/team/guest/public). invite, OpenTo self-seating, and role-holder seats require circle membership (roster.go); --shared rooms relayed cross-host (weave/session_remote.go RelaySharedRoomPost) only to paired outposts in the room's circle; sprint rooms (meetroom.Assume) inherit the sprint's circle. herald A2A/ACP peers bound to circles. ask/approve queue routes only to humans of the circle. Reconcile with S513 invite-only sharing and human-lane audience rules (101f7ec3b611 I14/I15: when multiple principals are present, permitted context is the intersection of their circles).
Depends on: S583.2, S583.8.
Acceptance: invite of a non-member refused (403 over HTTP); shared room never relays to a non-member peer; ask from a team circle never reaches a family/guest-only human.
