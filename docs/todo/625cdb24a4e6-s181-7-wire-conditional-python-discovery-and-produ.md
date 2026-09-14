---
id: 625cdb24a4e6
kind: task
title: S181.7 — wire conditional Python discovery and product e2e
seq: 282
status: done
priority: p1
created: 2026-09-14T20:42:14.291799Z
assignee: codex-gpt5.6-sol
sprint: 181
closed: 2026-09-14T21:30:12.924217Z
closed_by: codex-gpt5.6-sol
---

Wire Python adapter discovery only for Bash++ source units that contain Python blocks, preserve cmd/bash and POSIX isolation, add help and CLI e2e for direct and qualified calls. Gate: agentos tests and CI=true make test. Depends on sh S181.6. Sprint: #181.
