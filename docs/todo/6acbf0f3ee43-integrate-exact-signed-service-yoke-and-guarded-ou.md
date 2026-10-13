---
id: 6acbf0f3ee43
kind: chore
title: Integrate exact signed-service yoke and guarded outpost dependency pins
seq: 1905
status: done
priority: p1
created: 2026-10-13T08:30:13.206473Z
weave: 81
assignee: s434-bashy-pins
sprint: 434
sprint_id: f0fadef8-daec-5fc2-9c78-40979f01a0a7
sprint_title: bashy verified builds for binmgr tools and services (fork, build, sign, distribute)
closed: 2026-10-13T09:31:02.905522Z
closed_by: s434-codex-manager
---

S434 final consumer dependency integration. Isolated bashy only, base main includes S433 OpenClaw af25a36e; preserve all commits. Exact yoke b0f5fd30710d7dc4df9ecce9e31cec029b9c2cee and outpost065b77d8db45ba7e2ec1989c5ba3f6d26d11394c (published by manager) must replace stale go.mod pins including yoke/external/otel and related yoke submodules if graph requires. Keep Go1.27.2, no broad upgrades, no runner release wiring changes (S436 owns that). Scope go.mod/go.sum plus own story. Check exact module graph and go mod verify with GOWORK=off; short relevant existing unit tests allowed, but NO go build ./cmd/bashy and NO full packages/build/suite on Dragon. Native build deferred to manager-held novidesign when available. Do not run tests that start services or containers. Commit exact dependency files with Sprint434 Story-ID; no main/push. Worker must run bashy sprint submit434 THIS story with exact commit and module checks, then message s434-codex-manager. Do not inspect any remote fixtures or credentials. Bashy main held by manager, S433/S436 ACK. Task repo story path is in current docs/todo; read that, no broad filesystem searches.

Final delivery supersedes the initial coordinates above. The isolated branch merges local shared Bashy `main` at reconciled commit `99bd7bdf5ab1880f5307e332d83bff13e13c9786`, preserving the earlier `bfe4d096` pin work and the full S433 OpenClaw/Go1.27.2/security history. GitHub `refs/heads/main` was independently verified at Yoke `43e1f4159979de01ecec57635085a1632cfba131` and Outpost `d5d078fbe1b0f11dc83d47b0a60e07d28710bf86`.

Final module coordinates:

- Yoke root, `external/otel`, `pkg/llmgw`, and `pkg/oci`: `v0.0.0-20261013085125-43e1f4159979`.
- Outpost: `v0.14.38-0.20261013085803-d5d078fbe1b0`.
- Reconciled Bashsharp: `v0.0.0-20261013005017-a85df1acfc75`, with required sums repaired.
- Graph-required coreutils: `v0.0.0-20261013004930-226a73d9a214`; gfy: `v0.0.0-20261013004914-30d38f086034`.
- Go language/toolchain remain `go 1.27` / `go1.27.2`. The exact final Yoke/Outpost pins retain the managed Ollama guard and Prima refusal; no source, runner, release, or source-admission behavior was edited in this story beyond the reconciled base merge.

Verification (all PASS, `GOWORK=off`): `go mod tidy -diff` (empty), `go mod verify` (`all modules verified`), exact `go list -m` and `go mod graph` coordinate checks, and focused non-service `go test ./internal/agentos -run '^(TestOutpostFrontDoorIsDiscoverable|TestSelfInstallSummaryKeepsOutpostDormantWithoutService)$'`. No build, broad suite, service, container, remote-host, or fixture traversal was run; native build remains deferred to the manager-qualified novidesign lane.
