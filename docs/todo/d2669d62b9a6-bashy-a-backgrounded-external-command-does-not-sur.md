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

## Review 2026-09-30 (steward)

- Status: still reproduces. Checked 2026-09-30 with installed bashy 5.3.0-bashy-dev (d1f0847) on darwin: `bashy -c '/usr/bin/touch F &'` and the slow `/bin/sh -c 'sleep 1; : > G' &` form both leave no file. No fix commit in sh or bashy since the story was filed; sh 55289834 (2026-09-29, cancelled command takes its whole process tree) is in the opposite direction and must not be widened to asynchronous jobs at normal exit.
- Outdated: nothing; the linked goal item `bg-job-survives-exit` is correct.
- Next step: in sh interp's exit path, detach still-running asynchronous external jobs (process group left alive, as a fork-based shell does) instead of cancelling their context; keep cancellation for signals / explicit kill. Add a Linux+darwin regression that asserts the file appears after the shell exits.
- Acceptance: the two repro forms create their files (5/5); `make test-bash` 86/86 unchanged; nohup:* / background-related identities no worse in the next full arm.
- Depends on: nothing; can run in parallel with Sprint 110.
