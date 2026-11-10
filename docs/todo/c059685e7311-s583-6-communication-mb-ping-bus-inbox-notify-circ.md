---
id: c059685e7311
kind: feature
title: 'S583.6 Communication: mb/ping/bus/inbox/notify circle partitions and authorization'
seq: 3137
status: todo
priority: p1
labels:
    - boundary
    - comms
created: 2026-11-06T21:45:56.082986Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Communication part 1: mb, ping, bus, inbox, notify. Partition host board stores by circle via yoke/pkg/scope (~/.bashy/mb/ for default circle, ~/.bashy/mb/owner/ at 0700, ~/.bashy/mb/circles/<circle>/ for family/friend/team/guest) and stamp circle on posts. ForReader (yoke/pkg/bus/board.go) enforces circle membership authorization: a reader sees only posts of circles it belongs to. Recipient resolution — agent name, role seat (roleaddress.go), audience selector (ensure.go FleetSelect), topics — is restricted to members of the sender's active circle; sending to a non-member fails closed. sprint ping / mb --sprint scope follow the sprint's circle. Cross-host name@host relay (bus/remote.go) verifies that the target paired outpost belongs to the active circle.
Depends on: S583.2, S583.8.
Acceptance: mb --to a non-member refused; post invisible to non-member readers; role mail still holder-gated; legacy posts readable in default circle.
