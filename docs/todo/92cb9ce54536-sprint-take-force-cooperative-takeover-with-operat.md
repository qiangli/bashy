---
id: 92cb9ce54536
kind: feature
title: 'sprint take --force: cooperative takeover with operator approval and incumbent handoff'
seq: 3121
status: todo
priority: p0
labels:
    - sprint
created: 2026-11-06T17:24:36.293262Z
assignee: claude-opus5.5
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

Problem: take --force silently seizes a fresh lease; the incumbent keeps acting and writes no handoff. MVP: bashy sprint take ID --force --approved-by OPERATOR --reason TEXT [--grace 10m]. (1) Records a takeover request with the operator approval on the thread. (2) Notifies the incumbent (mb + SteerLive/instruct when its session is managed): 'write sprint handoff -m and stop conducting'. (3) Transfers the lease when the incumbent runs sprint handoff, or when the grace period expires (recorded as forced, no brief). (4) After transfer, gated verbs from the former holder are refused with 'seat taken by X'. (5) The new holder sees the handoff brief and a re-triage-due flag. Reuse existing take/handoff/lease code (weave_story.go, sprint_lease_coord.go); no new transport. Verification: unit tests for cooperative path, grace-expiry path, refusal of the former holder, missing --approved-by refused. Baseline: S562 assessment (operator-started Codex manager: 0/69 self-initiated turns, 4-9.5h reaction lag, 18 submissions pending 29-38h, 0 rejects, 5 self-accepts, 10 planned stories grew to 41).
