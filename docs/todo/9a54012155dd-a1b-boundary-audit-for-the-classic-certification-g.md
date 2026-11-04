---
id: 9a54012155dd
kind: requirement
title: A1b Boundary audit for the Classic certification graph
seq: 3046
status: done
priority: p0
labels:
    - clean-baseline
created: 2026-11-04T03:33:10.775828Z
weave: 10
assignee: genie-glm-5.3
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
closed: 2026-11-04T04:42:36.128438Z
closed_by: claude-opus5.5
---

Separate-owner slice of A1 (25dff6cbb152), filed only for independent ownership; A1 acceptance unchanged. Deliver docs/s561/boundary-audit.md: record actual B->A and C->A/B dependency violations that reach the Classic (shell + Sprint 341 base pure-Go utilities) product graph of bashy main (== 5a4bd03f outside docs), standalone GOWORK=off. Use existing ratchets first (e.g. TestGoSourceFrontEndIsNotLinkedIntoClassicBash) and go list -deps on ./cmd/bash and ./cmd/bashy; list each violation with import path, owning module, whether it is linked into the Classic binary, and whether it blocks the base profile. Fix nothing (fixes go to A3 only if they block the profile); no broad refactor; no new tool beyond a short go list/grep recipe recorded in the doc. Do not touch $HOME/projects/poc/dhnt or poc/bashy. Commit only that file with trailers Sprint: #561, Story: A1b, Story-ID: <this id>; BASHY_AGENT = assigned worker. Do not push.
