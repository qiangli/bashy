---
id: 0e56442b0a75
kind: task
title: Validate nested timeout envelopes and split impossible agent schedules
seq: 3
status: todo
priority: p1
created: 2026-08-06T06:57:28.019217Z
---

Before chat/supervise/foreman launches a task containing bounded child operations, compare the enclosing agent/tool-call runtime budget against the aggregate worst-case child timeout envelope plus startup, polling, archival, and cleanup overhead. Fail fast with a clear diagnostic when the schedule cannot fit. Where operations are independent (for example multiple certification sets each capped at 600s under a 600s ycode foreground ceiling), automatically split them into separate serial turns with distinct evidence/ledgers. Never start a partial schedule that can be cut off by its parent timeout. Add unit tests for equality boundary, overhead, serial splitting, stop-on-failure, and tools with provider-specific maximum tool-call durations.
