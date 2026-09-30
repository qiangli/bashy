---
id: 2c152d9bebb4
kind: feature
title: Wire indexed lookup into bashy search --files
seq: 345
status: done
priority: p1
labels:
    - search
created: 2026-09-30T05:13:29.634153Z
assignee: codex-gpt6-sol
sprint: 334
sprint_id: 2c3a1d0f-f046-5e3b-b086-b6b7a20505bd
sprint_title: Bashy indexed local filename search
closed: 2026-09-30T05:31:15.230217Z
closed_by: codex-gpt6-sol
---

Use the existing bashy search --files surface. Add explicit index/refresh/status controls and route filename queries to an index when available, retaining scan behavior for unindexed roots. Update command catalog/help/docs and build across macOS/Linux/Windows. This implements the user's correction to the original locate proposal.
