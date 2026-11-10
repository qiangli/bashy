---
id: 4e3a9a44db9b
kind: test
title: 'S583.15 Closure: boundary scenario suite and no-regression evidence'
seq: 3146
status: todo
priority: p1
labels:
    - boundary
    - closure
created: 2026-11-06T21:45:57.456515Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control (KISS): owner by default, circles from the member list, checked where data leaves'
---

Closure: scenario suite and no-regression evidence for the KISS scope (S583.1-.3, .11). Temporary BASHY_HOME and a fixture member list (owner + family, team, guest members).
1. Owner by default: nothing leaves through any inventoried egress point without a grant.
2. Members: member/non-member/removed/circle-changed through the LLM door.
3. Secrets: handle use never appears in egress surfaces; agent and non-TTY reveal refused.
Regression (no regression from baseline): yoke go test, bashy GOWORK=off make test + make test-bash 86/86 serial on the remote test host, not the dev box. One evidence record per candidate; pre-existing failures tracked separately.
Depends on: S583.1, .2, .3, .11.
Acceptance: scenarios pass, regression green, evidence recorded.
