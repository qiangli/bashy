---
id: 1ee0c4e59988
kind: bug
title: weave pull --force exits 0 without merging and without a reason
seq: 3255
status: done
priority: p1
created: 2026-11-20T12:29:31.665162Z
assignee: agy-gemini4-carbon
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
closed: 2026-11-20T18:55:30.04306Z
closed_by: claude-opus5.5
---

2026-11-20 bashy run #123 (branch rebased onto main by the conductor): 'weave pull 123 --force' printed only the isolation warning, exit 0, main unchanged, run still submitted. Merged by hand with git merge --no-ff. Root cause not yet recorded; a pull that does not merge must say why and exit non-zero.
