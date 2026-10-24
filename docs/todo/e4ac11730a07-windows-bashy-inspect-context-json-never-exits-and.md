---
id: e4ac11730a07
kind: bug
title: 'Windows: ''bashy inspect context --json'' never exits and burns a full core (busy loop) when launched non-interactively (cmd /c over ssh)'
seq: 1945
status: todo
priority: p1
labels:
    - windows
created: 2026-10-24T00:29:58.698973Z
sprint: 497
sprint_id: 16af0380-7ec7-5d41-a9c6-50e1b03cdf11
sprint_title: 'Fix the 2026-10-23 findings: satellite login/superadmin security, web console multi-satellite UX, fleet downgrade trap, Windows agent bugs, weave/sprint tooling, outpost headless/version skew'
---

Found 2026-10-23: on the approved Windows test host, two 'cmd /c bashy inspect context --json' processes from 11:14 (agent ssh sessions) were still running at 17:28, about 22,600 CPU-seconds each (2 cores together, CPU load 45%, fan noise). Killed. Reproduced with the newly installed build (bashy 49a3a6f0): Start-Process bashy.exe inspect context --json with redirected stdout was still running after 20 s with 20.6 CPU-seconds used. inspect context is the documented first call for agents, so every agent session on Windows can leave a spinning process. Expected: it completes in well under a second and exits; add a regression test that runs it with redirected stdio on Windows.
