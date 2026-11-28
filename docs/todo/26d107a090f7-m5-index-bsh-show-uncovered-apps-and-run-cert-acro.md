---
id: 26d107a090f7
kind: enhancement
title: 'M5 index.bsh: show uncovered apps and run cert across surfaces'
seq: 3336
status: done
priority: p3
labels:
    - shapeware
created: 2026-11-28T06:50:11.320621Z
assignee: claude-opus5.5-s741
sprint: 742
sprint_id: 7dde34d4-f382-5aba-8ff5-071c5e3548b8
sprint_title: 'Shaping: bugs and issues become spec changes (issue-to-spec support)'
closed: 2026-11-28T08:31:23.283689Z
closed_by: claude-opus5.5-s741
---

shapeware/index.bsh check only lints. Add a coverage view (apps or route files with no surface, from an optional apps list) and a cert/test target across surfaces. Copied verbatim into repos, so change it once in bashy. The ClassGo owner assumed all apps were lifted when only one was; a coverage line would have shown it.
