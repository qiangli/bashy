---
id: 8d5a0b25115e
kind: task
title: 'sh CI: background-launch test asserts a guarantee bash does not give, held up by a 1ms sleep'
seq: 202
status: todo
priority: p0
created: 2026-09-04T05:36:36.346299Z
sprint: 101
---

TestBackgroundExternalLaunchVisibleBeforeNextStatement asserted that an async external command's side effect is visible to the next statement. Measured: GNU bash never does this (0/20 on bash 3.2/darwin and 5.2/linux). The only thing making it pass was yieldExternalLaunch()'s time.Sleep(1ms) in the hot path of every backgrounded external command, which races under CI load (reproduced 4/25 in a one-CPU container). Remove the sleep; assert the real invariant instead.
