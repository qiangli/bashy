---
id: 01a1ca69-1025-76c6-ae8a-2143766039c9
seq: 24
form: page
type: gotcha
title: Desktop helpers must stay read-only toward supervisor-run units (outpost daemon, apps console)
description: 'WHEN building a tray, launcher or other desktop helper for outpost/bashy: never start, stop, enable or spawn what outpost''s supervisor runs (the daemon, the apps console on 22749, other service units). Observe only (dial the admin port; outpost status --json at most once a minute) and open the admin UI for lifecycle. On Windows a second controller in the user session (outpost enable / bashy app serve from the tray) started a duplicate daemon beside the supervisor: console windows flashing from its probes, an admin-port clash, and a lost pidfile that made outpost stop report ''not running''. Programs launched from Task Scheduler contexts can also hang at startup there, so helper-spawned children are fragile. Shown in examples/tray (watch-only since Sprint 605).'
status: candidate
source:
    tool: claude
created: "2026-11-11T11:40:45Z"
---
