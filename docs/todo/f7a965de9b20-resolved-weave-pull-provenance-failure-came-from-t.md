---
id: f7a965de9b20
kind: task
title: 'Resolved: weave pull provenance failure came from the pre-fix installed binary'
seq: 240
status: done
priority: p2
created: 2026-09-06T04:12:55.618098Z
sprint: 127
closed: 2026-09-06T04:36:44.663073Z
---

RECONCILED after Dragon install. The failure was reproduced with the old installed Bashy build e8648e2 while source already contained Story #236 commit 63090d50. That accepted fix adds TestWeavePullRequireReviewPreservesSourceProvenanceInMerge and weaveMergeCommitMessage, which propagates validated Sprint/Story/Story-ID trailers. Current installed build 46a02a5 includes it; focused coverage and canonical coreutils gate pass. This is a stale-binary duplicate, not an additional product defect.
