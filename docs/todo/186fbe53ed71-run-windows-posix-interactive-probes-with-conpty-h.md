---
id: 186fbe53ed71
kind: test
title: Run Windows POSIX interactive probes with ConPTY harness
seq: 344
status: todo
priority: p1
labels:
    - posix
    - windows
created: 2026-09-24T22:37:17.188241Z
sprint: 274
sprint_id: e14729b9-4d75-5803-aca0-594e943ca2df
sprint_title: Bashy GNU Bash 5.3, POSIX mode and full Go tests across platforms
---

Repair the POSIX interactive harness to use a native Windows ConPTY session and run the six probes inside Bashy GNU mode against Bashy POSIX. Record the absence of an independent upstream Bash 5.3 oracle and score only what is justified. Investigate the macOS tilde assignment difference and distinguish shell semantics from missing external commands. No Bashy product fixes.
