---
id: a555c551303e
kind: requirement
title: H1 One shaping model across web, CLI and service apps
seq: 3055
status: todo
priority: p1
labels:
    - shaping
created: 2026-11-04T06:51:46.948421Z
sprint: 564
sprint_id: 0f09b646-9acd-5bec-849d-2cf3b0e4edef
sprint_title: 'Software shaping for Bashy apps: any user shapes an app on their own host'
---

Define the single shaping model (principal version history; version produced by a human, genie or both; trusted verifier; scoped activation; offer/adopt sharing; submit upstream) and the three app kinds with activation scope and verifier checks: web (existing, per user, protected parts), CLI (bashy itself, per host side-slot binary, release kept, PATH unchanged), service (Klatch from Sprint 563 F14, per host image swap, data kept). Fix the CLI <-> /api/shape mapping (status, wish, show, diff, verify, approve, reject, preview, suggest, incident; history/undo/restore/release/offer/adopt delegating to bashy src; new submit) and the identity record (source commit, workspace/branch, artifact hash, verifier evidence, scope, actor, represented user). Acceptance: reviewed model and command/API contract; Sprint 526 web behavior explicitly unchanged.

Plan: docs/plan-s564-master-execution.md. Planning record; no implementation started. One shaping model, two front-ends: the existing web /shape/<app>/ page and /api/shape/{app}, and a new bashy shape CLI mirroring that API verb for verb; both use the shared yoke/pkg/shape engine; bashy src stays the version/source plumbing underneath. Same verbs and authorization for humans, agents and agent-assisted humans. Candidates always built in an isolated workspace, never a shared checkout. Sprint 526 web behavior unchanged. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
