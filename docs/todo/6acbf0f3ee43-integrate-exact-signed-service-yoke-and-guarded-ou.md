---
id: 6acbf0f3ee43
kind: chore
title: Integrate exact signed-service yoke and guarded outpost dependency pins
seq: 1905
status: assigned
priority: p1
created: 2026-10-13T08:30:13.206473Z
weave: 81
assignee: s434-bashy-pins
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
---

S434 final consumer dependency integration. Isolated bashy only, base main includes S433 OpenClaw af25a36e; preserve all commits. Exact yoke b0f5fd30710d7dc4df9ecce9e31cec029b9c2cee and outpost065b77d8db45ba7e2ec1989c5ba3f6d26d11394c (published by manager) must replace stale go.mod pins including yoke/external/otel and related yoke submodules if graph requires. Keep Go1.27.2, no broad upgrades, no runner release wiring changes (S436 owns that). Scope go.mod/go.sum plus own story. Check exact module graph and go mod verify with GOWORK=off; short relevant existing unit tests allowed, but NO go build ./cmd/bashy and NO full packages/build/suite on Dragon. Native build deferred to manager-held novidesign when available. Do not run tests that start services or containers. Commit exact dependency files with Sprint434 Story-ID; no main/push. Worker must run bashy sprint submit434 THIS story with exact commit and module checks, then message s434-codex-manager. Do not inspect any remote fixtures or credentials. Bashy main held by manager, S433/S436 ACK. Task repo story path is in current docs/todo; read that, no broad filesystem searches.
