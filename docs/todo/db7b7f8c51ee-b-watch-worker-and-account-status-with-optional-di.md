---
id: db7b7f8c51ee
kind: feature
title: B. Watch worker and account status with optional differences
seq: 3310
status: done
priority: p1
created: 2026-11-25T09:58:23.352984Z
assignee: codex-gpt6.2-sol
sprint: 688
sprint_id: b6d0bf82-bf8a-5ba7-bbab-90b24be56deb
sprint_title: 'Worker supervision: show current status and watch changes'
closed: 2026-11-26T06:05:18.384822Z
closed_by: codex-gpt6.2-sol
---

Follow the information from part A for a chosen duration. An operator can request change-only output so they can see what changed without reading repeated unchanged values. An illustrative invocation is --watch <duration> --diff=true. Bashy already has sprint monitor --watch --duration and compact change events. Reuse that behavior where it fits and preserve existing invocations. Demonstrate one changing value, an unchanged observation and the watch ending. The implementing agents choose the command, flag details and output design. Sprint plan: umbrella docs/sprint-688-master-execution-plan.md.
