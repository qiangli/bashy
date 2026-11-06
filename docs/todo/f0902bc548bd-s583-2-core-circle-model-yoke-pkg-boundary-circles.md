---
id: f0902bc548bd
kind: feature
title: 'S583.2 Core circle model: yoke/pkg/boundary, circles.yaml, resolution, bashy circle verb, BASHY_CIRCLE, boundaryGuard'
seq: 3133
status: todo
priority: p1
labels:
    - boundary
    - core
created: 2026-11-06T21:45:55.476745Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Core boundary model. New package yoke/pkg/boundary: Circle, Label, Resolve(cwd, env) (Circle, Reason), Allows(dataCircle, target, op), circles.yaml loader (~/.config/bashy/circles.yaml, 0600, never committed), committed repo binding .bashy/circle, default circle. yoke/pkg/scope.Resolve returns circle with ring; yoke/pkg/principal Owner from config instead of "local". New verb bashy circle (whoami, use, ls, show, check) with the resolution reason; shown in bashy inspect context. bashy exports BASHY_CIRCLE beside BASHY_PRINCIPAL/BASHY_EPISODE (yoke/pkg/policy/coord/identity.go). boundaryGuard middleware after claimGuard in bashy/internal/agentos/agentos.go: resolve, export, audit; denials are structured errors. Audit records gain circle and denied-crossing events.
Depends on: S583.1 decisions.
Acceptance: unit tests for resolution order and full Allows matrix (circle pair x op); legacy behaviour unchanged when no circles.yaml exists (everything = default circle); yoke and bashy GOWORK=off gates green.
