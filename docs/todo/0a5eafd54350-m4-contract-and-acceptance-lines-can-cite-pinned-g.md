---
id: 0a5eafd54350
kind: feature
title: M4 Contract and acceptance lines can cite pinned go tests
seq: 3335
status: assigned
priority: p1
labels:
    - shapeware
created: 2026-11-28T06:50:10.965174Z
assignee: claude-opus5.5-s741
sprint: 742
sprint_id: 7dde34d4-f382-5aba-8ff5-071c5e3548b8
sprint_title: 'Shaping: bugs and issues become spec changes (issue-to-spec support)'
---

ClassGo's sprint 733 characterization tests are the real evidence of current behaviour but sit outside any spec. Allow an acceptance line / assertion of the form 'go test -run NAME ./pkg => PASS' with the test named in the surface, so a fix that flips a pinned test is a spec-linked change and candidate.bsh runs it. Decide how mutate treats such assertions. Verification: classgo backup restore pinned test cited from the data-backup-restore surface.
