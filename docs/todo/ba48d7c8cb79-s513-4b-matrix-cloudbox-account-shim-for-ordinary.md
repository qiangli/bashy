---
id: ba48d7c8cb79
kind: feature
title: S513.4b Matrix Cloudbox account shim for ordinary users
seq: 2740
status: todo
priority: p0
labels:
    - deferred
    - operator-stop
created: 2026-10-27T17:23:53.907255Z
weave: 301
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
---

Implement Bashy-side integration shim consumed by Cloudbox S513.4a commit 1b1af90. Existing Cloudbox handler expects POST <app>/_cloudbox/v1/account with ensure|reset, Bearer MATRIX_TOKEN, returns password once for an ordinary scoped user; coordinate exact request/response and reset semantics with Claude #171 before code. Use signed native Matrix/tuwunel service, stable mapping from Cloudbox subject, vault-managed credentials, independent Matrix lifecycle. No generic Authorization bypass or shared admin user. Scope Bashy examples/matrix-chat and existing extension/service mechanisms; no core command unless justified. Add automated live integration E2E on disposable host: three distinct Cloudbox users with chat:matrix scopes, each opt in/receive secret once; DM/group text+image through actual Matrix service and HTTPS route, uninvolved denied, revocation honored. Test-only synthetic secret adapter may cover protocol gate but production ordinary-user flow must use real vault on authorized host. If live Cloudbox HTTPS/test tokens unavailable, report blocked gate explicitly, commit bounded implementation with tests and no false PASS. No containers on Dragon/paid infra; coordinate host claim. Read CLAUDE.md; injected identity + Sprint513/Story trailers. Manager gates/merges.
