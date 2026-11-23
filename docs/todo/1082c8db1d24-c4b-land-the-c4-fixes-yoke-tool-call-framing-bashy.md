---
id: 1082c8db1d24
kind: task
title: 'C4b: land the C4 fixes (yoke tool-call framing + bashy Node 18/screenshot fixture) and bump bashy''s yoke pin'
seq: 3290
status: assigned
priority: p0
created: 2026-11-21T20:54:42.119376Z
weave: 134
assignee: claude-fable5.1
sprint: 682
sprint_id: 623586bb-742f-570e-87c9-9af0c101bac1
sprint_title: Shaping-enable the example apps for consumers (viewer, session, form, board/whiteboard/table/hello-world, notepad/chart)
---

Conductor 2026-11-21 (holds the 682 lease; codex's lease was stale). Land, starting on current heads and checking drift first: (1) yoke branch origin/s682/8e5204e127c4 (d22a962a, adapter tool-call framing fix) onto yoke main, rebased if needed, with its unit tests; (2) bashy commits dc868d54 (run-scoped host Node 18 + ws preflight helper) and 1b8f2fc9 (bounded screenshot docs) from bashy weave run #133 onto bashy main; (3) bump bashy's yoke pin to include the framing fix (bashy mod sync per the drift rule; claim the bump - Sprint 677 is also bumping bashy pins; coordinate). Unit tests only. Do NOT run the paid rerun: it starts after Sprint 677's self-path regression fix (run #139) merges.
