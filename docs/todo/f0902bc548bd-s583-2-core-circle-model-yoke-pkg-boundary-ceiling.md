---
id: f0902bc548bd
kind: feature
title: 'S583.2 Core circle model: yoke/pkg/boundary, ceiling-locked BASHY_CIRCLE, scope resolution, bashy circle verb, boundaryGuard'
seq: 3133
status: todo
priority: p0
labels:
    - boundary
    - core
created: 2026-11-06T21:45:55.476745Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Core boundary model. New package yoke/pkg/boundary: Circle (built-in defaults: owner, family, friend, team, guest, public, plus optional team:<name>), Resolve(cwd, env) (Circle, Reason), Allows(dataCircle, target, op), minimal ~/.config/bashy/circles.yaml loader (0600, never committed: owner email, default circle, path/remote -> circle map, deny patterns), committed repo binding .bashy/circle. yoke/pkg/scope.Resolve maps active circle to directory partition ("" for default circle so legacy stores work in place, "owner" at 0700, "circles/<circle>" otherwise); yoke/pkg/principal resolves Owner from config/pairing oauth2_email instead of hardcoded "local". Ceiling-lock BASHY_CIRCLE in yoke/pkg/policy/coord/identity.go: spawned agents and non-TTY children inherit BASHY_CIRCLE and cannot switch or widen circles via --circle or env mutation without an interactive TTY human approve gate. New verb bashy circle (whoami, use, ls, show, check, share) with resolution reason; shown in bashy inspect context. boundaryGuard middleware after claimGuard in bashy/internal/agentos/agentos.go: resolve, enforce ceiling lock, export, audit; denials are structured errors.
Depends on: S583.1.
Acceptance: unit tests for resolution order, agent --circle ceiling-lock refusal, and full Allows matrix; legacy behaviour unchanged when no circles.yaml exists; yoke and bashy GOWORK=off gates green.
