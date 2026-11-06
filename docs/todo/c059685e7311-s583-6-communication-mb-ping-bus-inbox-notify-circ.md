---
id: c059685e7311
kind: feature
title: 'S583.6 Communication: mb/ping/bus/inbox/notify circle authorization'
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

Communication part 1: mb, ping, bus, inbox, notify. Posts carry a circle. ForReader (yoke/pkg/bus/board.go) becomes an authorization filter, not a view: a reader sees only posts of circles it is a member of. Recipient resolution - agent name, role seat (roleaddress.go), audience selector (ensure.go FleetSelect), topics - limited to members of the sender's circle; sending to a non-member fails closed. Host store partitioned or labelled per circle (~/.bashy/mb). sprint ping / mb --sprint scope follow the sprint's circle. Cross-host name@host relay (bus/remote.go) checks the peer's circles.
Depends on: S583.2, S583.8.
Acceptance: mb --to a non-member refused; post invisible to non-member readers; role mail still holder-gated; legacy posts readable in default circle.
