---
id: 9123b96d282d
kind: bug
title: S513.5b brainstorm missing-session reads must not create sessions
seq: 2741
status: todo
priority: p0
created: 2026-10-27T17:32:23.965835Z
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
---

Review finding in submitted brainstorm commit 987ee5f6: server main.go loadLocked creates a named session on first use and state/export GET paths call it. A guest GET of a chosen missing session can reserve its name before facilitator create; repeated guesses can fill disk. Reproduce, then make missing named-session reads return 404 and allow only explicit facilitator create to mint session/capability, while preserving startup/default main and legacy board.json compatibility. Add focused regression test; rerun go test -race and real browser E2E. Scope examples/brainstorm only, no whiteboard/Matrix edits, no containers/heavy dev-host jobs. Commit with injected Sprint513/Story trailers in isolated workspace. Manager gates/merges and keeps original story open until repair verified.
