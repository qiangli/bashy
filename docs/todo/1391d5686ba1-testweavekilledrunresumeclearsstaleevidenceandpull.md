---
id: 1391d5686ba1
kind: bug
title: TestWeaveKilledRunResumeClearsStaleEvidenceAndPullsFreshSubmission fails on yoke main at weave_reconcile_test.go:497 (resume submission exit=1)
seq: 3257
status: done
priority: p2
created: 2026-11-20T18:47:01.685995Z
assignee: muse-wonder-full
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
closed: 2026-11-20T21:49:23.6711Z
closed_by: claude-opus5.5
---

Pre-existing on yoke origin/main ef15fce3 (reproduced 2026-11-20 by the conductor in a clean detached worktree, GOWORK=off go test ./pkg/weave -run TestWeaveKilledRunResumeClearsStaleEvidenceAndPullsFreshSubmission). Output only shows the --issue deprecation warning; the real non-zero cause is not yet identified — root cause first. Consequence: the pull assertions after line 497 (edited by 1ee0c4e5) do not execute until this is fixed. Found during review of 1ee0c4e5; not caused by it.
