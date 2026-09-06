---
id: 0175453fed17
kind: task
title: 'Bug: Meet steering misses the resident manager unified inbox'
seq: 239
status: done
priority: p0
created: 2026-09-06T04:01:13.134317Z
sprint: 127
closed: 2026-09-06T04:22:10.137475Z
---

Independent rebuilt-source run of script/e2e-sprint-modes.sh after #235/#237: 22/23 passed; H4 failed. The Meet send path was invoked for the sprint dedicated room with body `apps meet steering probe`, addressed to mgr-agent, but `bashy inbox --as mgr-agent --peek` contained the prior MB steering event and not the Meet body. Worker had reported 23/23, so this may be a nondeterministic delivery bug or a human-author fixture mismatch; measure, do not assume. Reproduce repeatedly in hermetic HOME, inspect the Meet timeline and unified inbox projection, then fix the supported human Apps Meet -> registered resident manager inbox path or correct the test only if it is using an invalid public invocation. Acceptance: H4 passes repeatedly and asserts exact body receipt, not send success.
