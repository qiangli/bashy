---
id: f8fed0a7bd83
kind: feature
title: sprint reject plus self-accept guard
seq: 3106
status: assigned
priority: p1
labels:
    - sprint
created: 2026-11-06T17:08:02.246552Z
weave: 26
assignee: claude-opus5
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

MVP: bashy sprint reject ID STORY -m reason records the decision and returns the story to its worker (notified), ending re-claim churn. accept refuses when the accepter is the story's claimer or submitter unless --override --reason (audited). Code: yoke/pkg/weave/weave_story_claim.go. Verification: unit tests for reject->resubmit->accept round-trip and self-accept refusal. Baseline: S562 assessment (operator-started Codex manager: 0/69 self-initiated turns, 4-9.5h reaction lag, 18 submissions pending 29-38h, 0 rejects, 5 self-accepts, 10 planned stories grew to 41).
