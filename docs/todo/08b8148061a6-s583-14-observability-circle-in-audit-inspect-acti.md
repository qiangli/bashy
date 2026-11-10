---
id: 08b8148061a6
kind: feature
title: 'S583.14 Observability: circle in audit/inspect/activity/exechist, web console filtering'
seq: 3145
status: todo
priority: p3
labels:
    - boundary
    - audit
    - boundary-later
created: 2026-11-06T21:45:57.304988Z
---

Observability. inspect (doctor/context/audit), activity, out, exechist and telemetry record the active circle per event and every denied crossing; `bashy inspect context` and `doctor` report active circle + resolution reason and misconfigurations (unbound accounts, repos without binding). Web console panels (webconsole/scope.go) are filtered by the paired device's or Cloudbox member's circle (guest/family/friend/team); audit/exechist never contain owner secret values (secrets.Redactor from S583.3).
Depends on: S583.2, S583.3.
Acceptance: denied crossings visible in audit with circle pair and op; web console accessed by a team/guest member or device shows zero family/owner data.
