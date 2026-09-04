---
id: d2669d62b9a6
kind: task
title: 'bashy: a backgrounded external command does not survive shell exit (GNU bash does)'
seq: 204
status: todo
priority: p1
created: 2026-09-04T05:46:30.429343Z
sprint: 100
---

MEASURED, PRE-EXISTING (not introduced by the sh flake fixes; an old-vs-new engine A/B is 0/5 for both, GNU bash 5/5). 'bashy -c "/usr/bin/touch F &"' never creates F; neither does the slow form '/bin/sh -c "sleep 1; : > F" &'. GNU bash forks, the child is reparented and survives. bashy cancels the job context at shell exit and takes the carrier and its child with it. This is very likely WHY the removed yieldExternalLaunch() 1ms sleep was written: it looked like it made fast one-liners work. It did not — it only made a test pass. The real fix belongs in the exit path (detach surviving async jobs the way a fork-based shell does), never in a head-start sleep.
