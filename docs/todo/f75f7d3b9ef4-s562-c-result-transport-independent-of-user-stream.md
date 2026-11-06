---
id: f75f7d3b9ef4
kind: bug
title: S562 C++ result transport independent of user stream state
seq: 3091
status: assigned
priority: p0
created: 2026-11-05T04:48:56.785426Z
assignee: s562-cpp-stream
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Repair native C++ result framing when user code sets cout failbit (empty/missing ifstream insertion). Preserve source stdout, stream state semantics, exit callbacks and nonzero exits; protocol must not depend on writable C++ iostream state. Exact source cases basic-ifstream-class L38/L144. Independent native, interpreted and compiled evidence plus lifecycle and malformed-frame regression required.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: see existing story details and S562 board; no verified run or branch reference recorded.
