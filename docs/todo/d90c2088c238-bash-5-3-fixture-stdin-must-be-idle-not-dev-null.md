---
id: d90c2088c238
kind: task
title: bash-5.3 fixture stdin must be idle, not /dev/null
seq: 206
status: todo
priority: p0
created: 2026-09-04T21:07:14.797511Z
sprint: 115
---

The bash53 gate went red on the read fixture the moment the sh pin advanced through the read -t 0 poll. bashy is CORRECT: measured against real bash 5.3, both print 0 on /dev/null and both print 1 on an open idle descriptor, so read.right's expected 1 belongs to the environment bash's own suite runs in (a terminal: no input available, not at EOF). tools/bash53suite handed each fixture os/exec's default stdin, which is /dev/null and always readable. Give the fixture an idle pipe instead, and pin it with a test.
