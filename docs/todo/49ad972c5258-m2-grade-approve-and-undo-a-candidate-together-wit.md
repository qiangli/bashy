---
id: 49ad972c5258
kind: feature
title: M2 Grade, approve and undo a candidate together with its spec change
seq: 3333
status: done
priority: p1
labels:
    - shapeware
created: 2026-11-28T06:50:10.250559Z
assignee: claude-opus5.5-s741
sprint: 742
sprint_id: 7dde34d4-f382-5aba-8ff5-071c5e3548b8
sprint_title: 'Shaping: bugs and issues become spec changes (issue-to-spec support)'
closed: 2026-11-28T08:31:22.042823Z
closed_by: claude-opus5.5-s741
---

Today candidate.bsh fails any change to existing assertions, so a fix that changes pinned behaviour cannot pass; in classgo sprint 733 S4 the owner hand-carried the spec change and undo had to withdraw it separately. Make the spec delta part of the candidate: candidate.bsh grades the work copy against the proposed spec (old assertions minus the ones the owner-reviewed delta replaces); bashy shape approve activates both and shape undo reverts both in one step, recorded in the ledger. Verification: replay the S4 KI-3 approve/undo with no manual spec restore.
