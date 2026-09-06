---
id: d284cf3e8ad2
kind: task
title: 'Bug: weave pull drops provenance on reviewed sprint merge'
seq: 236
status: done
priority: p0
created: 2026-09-06T03:22:35.169459Z
sprint: 127
closed: 2026-09-06T03:43:24.43778Z
---

Found in Tool-managed mode while integrating coreutils weave run #6. The submitted worker commit 4659a6b7 contained valid Sprint #127, Story #230, and full Story-ID trailers; weave review passed. `bashy weave pull 6 --require-review` then attempted a merge commit without the repository-required provenance block, so the commit-msg hook rejected integration and weave reported conflict. Fix the supported integration path so a linked sprint run with provenance-valid source commits can be merged by the manager without manual recovery. Add a regression test for a commit-msg hook requiring provenance. No new command or flag. Acceptance: reviewed run pulls cleanly and retains truthful provenance.
