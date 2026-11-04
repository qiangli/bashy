---
id: 98e7589729ed
kind: feature
title: E1 Shared fence-corpus runner and generated inventories for all seven example sources
seq: 3047
status: assigned
priority: p0
labels:
    - clean-baseline
    - fence-corpora
created: 2026-11-04T03:51:00.602611Z
assignee: codex-gpt6-astra
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
---

One runner under bashsharp-tests/tools/fence-corpus reusing the tour cases.tsv/.expected conventions and existing Go-by-example supervision; interpreted mode for every case, compiled mode wherever lowering already supports the fence row (recorded). Generated inventories at pinned upstream commits for: Python Tutorial + 00-boundary; Go by Example (85) + Go Tour (97) generated fenced; TypeScript handbook + playground; C# docs; C and C++ references; Rust by Example + Rust Book; PowerShell docs reference/7.6. Every example gets a disposition; pin.tsv with provenance and licence attribution. Wire one fatal aggregate section with the count ratchet into harness/run.sh. Acceptance: runner and inventories committed; unaccounted examples, missing runners, manifests, mode records or toolchains fail closed.

Execution plan: docs/plan-s562-master-execution.md. Phase 1 of Sprint 562 (consolidates fence-corpora epic Sprints 391-396; their archived specs are reference only). Goal: bashy is all you need on every platform. Rollout step 1: Linux for every corpus, plus Windows for C# and PowerShell; later steps add Windows for the rest, then macOS and remaining arches. Every case must PASS: no xfail, no skip. A missing Bash# feature is implemented, never hidden or narrowed away; broad features are costed and reviewed with the operator first. Corpora are a permanent regression guard: fatal harness/run.sh section with committed per-mode/per-platform case and PASS counts. Run on claimed self-hosted test hosts, never the dev box. Record exact source, toolchain and binary identities. Delivery commits carry Sprint, Story and Story-ID trailers.
