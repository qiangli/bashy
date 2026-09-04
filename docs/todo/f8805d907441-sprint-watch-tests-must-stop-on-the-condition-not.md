---
id: f8805d907441
kind: task
title: Sprint --watch tests must stop on the condition, not the clock
seq: 207
status: todo
priority: p1
created: 2026-09-04T21:30:05.174934Z
sprint: 115
---

TestExternalSprintTakeWatchClaimsThenStreamsInbox gave the command a 250ms deadline that bounded the CLAIM as well as the stream. On the Windows CI runner the claim alone exceeded it and the command failed with 'sprint take: context deadline exceeded' (exit 1). The tests' errors.Is(err, context.DeadlineExceeded) escape hatch could never fire on that path because weave's exitCodeError carries a code and drops the cause. Stop on the condition instead: cancel as soon as every expected line is written, and keep a 30s safety net. Verified on a real windows/amd64 host: 10/10 PASS at ~0.45s each, which is itself over the old budget.
