---
id: 3616260cd7de
kind: task
title: Reap orphaned Go test processes after timed-out runs
seq: 297
status: todo
priority: p2
created: 2026-09-15T12:42:22.042393Z
sprint: 194
---

Housekeeping observed during Sprint 191: two interp.test processes were reparented to PID 1 yet continued for 12 hours and 39 hours at roughly 400% CPU each, despite their own -test.timeout=10m flag. They materially distorted resource alerts until explicitly killed. Diagnose the process-tree cleanup seam for interrupted/timed-out bashy-managed gates and ensure descendants are terminated and reaped without touching unrelated live runs. Add a focused process-group regression and surface stale orphan test processes through the existing resource diagnostics. This is unrelated to the Go-fence deliverable and must not widen Sprint 191.
