---
id: 85dd0272619c
kind: test
title: E3 TypeScript and Rust example corpora green
seq: 3049
status: assigned
priority: p1
labels:
    - clean-baseline
    - fence-corpora
created: 2026-11-04T03:51:00.918193Z
assignee: agy-gemini3.8-flash
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Depends on E1. TypeScript handbook + playground on the pinned default compiler (TypeScript 7 leg advisory only), diagnostic cases assert official error codes; Rust by Example + Rust Book listings against rustc from the same provisioned toolchain, mdBook attributes honoured. Implement missing Bash# features the examples expose (imports, class handles, JS/TSX fences, streaming and similar). Acceptance: every case PASSes on the step-1 platforms; counts committed to the ratchet.

Execution plan: docs/plan-s562-master-execution.md. Phase 1 of Sprint 562 (consolidates fence-corpora epic Sprints 391-396; their archived specs are reference only). Goal: bashy is all you need on every platform. Rollout step 1: Linux for every corpus, plus Windows for C# and PowerShell; later steps add Windows for the rest, then macOS and remaining arches. Every case must PASS: no xfail, no skip. A missing Bash# feature is implemented, never hidden or narrowed away; broad features are costed and reviewed with the operator first. Corpora are a permanent regression guard: fatal harness/run.sh section with committed per-mode/per-platform case and PASS counts. Run on claimed self-hosted test hosts, never the dev box. Record exact source, toolchain and binary identities. Delivery commits carry Sprint, Story and Story-ID trailers.

Manager reconciliation (2026-11-06): actual current minted TypeScript439logical/878modes and Rust470logical/940modes passed on frozen306d product/engine8a1; initial Linux per-mode floors are committed separately with source-bound receipts. This does NOT close the all-official-examples parent acceptance: inventory still explicitly classifies78 TypeScript and77 Rust source rows as missing-feature. Some may be stale after delivered context/runtime work; reconcile each against exact authored source and minted cases before deciding. Preserve every original source disposition and count; implement remaining in-scope capabilities or obtain explicit operator scope decisions, never convert exclusions into PASS. Current successful minted-case evidence and unfinished official-source coverage must be reported separately. Completed narrow repair stories can close independently.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: see existing story details and S562 board; no verified run or branch reference recorded.
