---
id: ee8e65e258a5
kind: test
title: E2 Python example corpus green
seq: 3048
status: done
priority: p1
labels:
    - clean-baseline
    - fence-corpora
created: 2026-11-04T03:51:00.761622Z
assignee: codex-gpt5.6-sol
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
closed: 2026-11-04T14:39:53.705365Z
closed_by: claude-opus5.5-s562
---

Depends on E1. Python tutorial cases + 00-boundary against native CPython; fenced Go by Example 85 + Tour 97 against the existing oracle hashes and tests/tour/results.tsv, existing three-mode Go gate unchanged. Fix fence defects at the owning layer (sh polyglot/interp/lower, bashsharp, bashy fence rows, yoke/external) with focused regressions. Acceptance: every case PASSes in each required mode on the step-1 platforms; counts committed to the ratchet.

Execution plan: docs/plan-s562-master-execution.md. Phase 1 of Sprint 562 (consolidates fence-corpora epic Sprints 391-396; their archived specs are reference only). Goal: bashy is all you need on every platform. Rollout step 1: Linux for every corpus, plus Windows for C# and PowerShell; later steps add Windows for the rest, then macOS and remaining arches. Every case must PASS: no xfail, no skip. A missing Bash# feature is implemented, never hidden or narrowed away; broad features are costed and reviewed with the operator first. Corpora are a permanent regression guard: fatal harness/run.sh section with committed per-mode/per-platform case and PASS counts. Run on claimed self-hosted test hosts, never the dev box. Record exact source, toolchain and binary identities. Delivery commits carry Sprint, Story and Story-ID trailers.
