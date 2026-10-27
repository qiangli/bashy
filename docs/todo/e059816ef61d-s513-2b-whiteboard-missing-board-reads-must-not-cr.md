---
id: e059816ef61d
kind: bug
title: S513.2b whiteboard missing-board reads must not create sessions
seq: 2739
status: assigned
priority: p0
created: 2026-10-27T17:23:43.676332Z
weave: 299
assignee: muse-conductor
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
---

Review finding in submitted whiteboard commit 05d542c8: server main.go loadLocked creates and persists a board on first call; state/export/other read paths appear to call it for unknown names. A guest GET /api/state?board=chosen can therefore reserve a name before facilitator create, causing create conflict and owner-control failure; repeated guesses can fill disk. Reproduce with native server test, then make non-create reads return 404 and ensure only explicit create action mints a new board and reveals facilitator key. Preserve main default board startup behavior and existing valid data. Add targeted regression test; rerun go test -race and real browser E2E. Scope examples/whiteboard only. Commit with injected Sprint513/Story trailers in isolated workspace; manager gates/merges. No containers/heavy dev-host build or paid infra.
