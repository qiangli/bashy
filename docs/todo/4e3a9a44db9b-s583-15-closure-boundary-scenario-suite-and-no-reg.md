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
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Closure: scenario suite and regression evidence. Scenario tests with two temporary circles home and org:acme/team:infra plus owner, all under temp BASHY_HOME: kb/memory non-recall across circles; mb/meet non-member refusal and invisibility; aws profile crossing refused; owner todo/kb invisible everywhere and to ungranted agents; secret used by handle never appears in stdout/audit/exechist/mb/handoff; agent cannot pass --all-circles; non-TTY reveal refused. Regression gate (no regression from baseline): yoke go test $(go list ./... | grep -v /external/), bashy GOWORK=off make test and make test-bash 86/86 serial on the remote test host, not the dev box. One shared evidence record per candidate; pre-existing failures tracked separately.
Depends on: S583.2-S583.14.
Acceptance: all scenarios pass, regression gate green, evidence recorded.
