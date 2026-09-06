---
id: 0d6a1e64a2fc
kind: task
title: 'Bug: sprint tick wait can miss the first newly added story'
seq: 241
status: done
priority: p0
created: 2026-09-06T04:20:04.390304Z
sprint: 127
closed: 2026-09-06T04:30:45.047044Z
---

Dragon canonical coreutils gate reported TestSprintTickWaitReturnsOnTheFirstChange failed in suite (Open=0, want 1) but passed isolated rerun. Tool-managed autopilot relies on tick/wait to notice dynamically added stories. Make the wait/change observation deterministic and add a repeated regression gate.
