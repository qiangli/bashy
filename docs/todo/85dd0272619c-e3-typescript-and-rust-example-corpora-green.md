---
id: 85dd0272619c
kind: test
title: E3 TypeScript and Rust example corpora green
seq: 3049
status: todo
priority: p1
labels:
    - clean-baseline
    - fence-corpora
created: 2026-11-04T03:51:00.918193Z
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
---

Depends on E1. TypeScript handbook + playground on the pinned default compiler (TypeScript 7 leg advisory only), diagnostic cases assert official error codes; Rust by Example + Rust Book listings against rustc from the same provisioned toolchain, mdBook attributes honoured. Implement missing Bash# features the examples expose (imports, class handles, JS/TSX fences, streaming and similar). Acceptance: every case PASSes on the step-1 platforms; counts committed to the ratchet.

Execution plan: docs/plan-s562-master-execution.md. Phase 1 of Sprint 562 (consolidates fence-corpora epic Sprints 391-396; their archived specs are reference only). Goal: bashy is all you need on every platform. Rollout step 1: Linux for every corpus, plus Windows for C# and PowerShell; later steps add Windows for the rest, then macOS and remaining arches. Every case must PASS: no xfail, no skip. A missing Bash# feature is implemented, never hidden or narrowed away; broad features are costed and reviewed with the operator first. Corpora are a permanent regression guard: fatal harness/run.sh section with committed per-mode/per-platform case and PASS counts. Run on claimed self-hosted test hosts, never the dev box. Record exact source, toolchain and binary identities. Delivery commits carry Sprint, Story and Story-ID trailers.
