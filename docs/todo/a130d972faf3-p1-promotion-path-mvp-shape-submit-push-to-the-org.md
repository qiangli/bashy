---
id: a130d972faf3
kind: feature
title: 'P1 Promotion path MVP: shape submit --push to the org remote and --gh upstream PR'
seq: 3147
status: todo
priority: p0
labels:
    - release
    - shaping
created: 2026-11-06T21:49:59.999939Z
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

Problem: rc.25 ships as a shapeable minimum; post-release features and fixes arrive as shaped changes promoted local user -> team/org -> upstream bashy. Today Service.Submit (yoke/pkg/shape/submit.go) opens only a LOCAL src PR plus a weave; nothing pushes a branch or opens an upstream PR, and src offer/adopt refuses CLI/service kinds.
MVP (KISS): (1) shape submit --push <remote>: push the shape-submit/<id> branch to the org remote configured by src upstream, reusing the go-git push in yoke/pkg/src/core_repair.go (release-core); ~100 LOC + unit test. (2) optional --gh: gh pr create against the bashy repo, body from the Submission record (Shape/Shaped-By trailers, verifier evidence path); ~80 LOC. (3) Teammate adopt for CLI kinds is a documented recipe, not new code: fetch the pushed branch, bashy shape wish bashy --from <checkout>, which re-verifies and rebuilds locally; document in examples/shaping/hello-world/README.md.
Order: lands in yoke first so the single pin-reconciliation bump (daf37e07c411) carries it. Cut line: if not merged by Saturday noon, freeze without it; it becomes the first shaped change after release and the known-issues ledger records it.
Deferred: kind-aware offer/adopt and cross-platform shaping matrix (Sprint 579 H5/H7).
Verification: unit tests for push/--gh argument building; one run pushing to a scratch org remote; GOWORK=off go test for yoke/pkg/shape and bashy internal/agentos shape tests.
