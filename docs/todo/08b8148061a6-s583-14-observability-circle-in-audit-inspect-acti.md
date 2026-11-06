---
id: 08b8148061a6
kind: feature
title: 'S583.14 Observability: circle in audit/inspect/activity/exechist, web console filtering'
seq: 3145
status: todo
priority: p2
labels:
    - boundary
    - audit
created: 2026-11-06T21:45:57.304988Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Observability. inspect (doctor/context/audit), activity, out, exechist and telemetry record the circle per event and every denied crossing; bashy inspect context and doctor report active circle + reason and misconfigurations (unbound accounts, repos without binding). Web console panels (webconsole/scope.go) filtered by the paired device's circle; audit/exechist never contain owner values (redactor from S583.3).
Depends on: S583.2, S583.3.
Acceptance: denied crossings visible in audit with circle pair and op; web console from an org-paired device shows no home data.
