---
id: df328510d106
kind: test
title: E4 C/C++, C# and PowerShell example corpora green
seq: 3050
status: todo
priority: p1
labels:
    - clean-baseline
    - fence-corpora
created: 2026-11-04T03:51:01.075638Z
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
---

Depends on E1. C17/C++20 reference examples against native clang; C# docs samples against native .NET; PowerShell docs examples against the pinned pwsh. C# and PowerShell run on Linux and Windows in step 1. Implement missing Bash# features the examples expose. Acceptance: every case PASSes on the step-1 platforms; counts committed to the ratchet.

Execution plan: docs/plan-s562-master-execution.md. Phase 1 of Sprint 562 (consolidates fence-corpora epic Sprints 391-396; their archived specs are reference only). Goal: bashy is all you need on every platform. Rollout step 1: Linux for every corpus, plus Windows for C# and PowerShell; later steps add Windows for the rest, then macOS and remaining arches. Every case must PASS: no xfail, no skip. A missing Bash# feature is implemented, never hidden or narrowed away; broad features are costed and reviewed with the operator first. Corpora are a permanent regression guard: fatal harness/run.sh section with committed per-mode/per-platform case and PASS counts. Run on claimed self-hosted test hosts, never the dev box. Record exact source, toolchain and binary identities. Delivery commits carry Sprint, Story and Story-ID trailers.
