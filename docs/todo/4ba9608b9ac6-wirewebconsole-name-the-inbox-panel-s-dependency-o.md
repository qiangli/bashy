---
id: 4ba9608b9ac6
kind: task
title: 'wireWebConsole: name the Inbox panel''s dependency on the fleet seams'
seq: 205
status: todo
priority: p1
created: 2026-09-04T17:47:54.215309Z
sprint: 122
---

wireMessageBoard() now serves TWO console apps. With FleetNames nil the Inbox
roster silently loses every catalog agent and shows only names the timeline
happens to mention — a quiet fleet, not a missing hook. Say so at the call site,
which is the same failure mode the wiring test was written for.
