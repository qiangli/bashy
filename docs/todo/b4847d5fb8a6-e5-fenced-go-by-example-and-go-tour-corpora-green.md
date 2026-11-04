---
id: b4847d5fb8a6
kind: test
title: E5 Fenced Go by Example and Go Tour corpora green
seq: 3051
status: assigned
priority: p1
labels:
    - clean-baseline
    - fence-corpora
created: 2026-11-04T04:13:00.396573Z
assignee: muse-spark1.2
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
---

Split from E2 for independent ownership (one owner per story). Depends on E1. Generated fenced Go by Example (85) + Go Tour (97) via a syntax-aware generator in tools/go-by-example; oracle reuses existing native oracle hashes and tests/tour/results.tsv; existing three-mode Go gate unchanged; build-only Tour rows stay build-only. Fix fence defects at the owning layer with focused regressions. Acceptance: every case PASSes in each required mode on the step-1 platforms (Linux); counts committed to the ratchet; regeneration byte-identical.

Execution plan: docs/plan-s562-master-execution.md. No xfail, no skip. Corpora are a permanent regression guard. Run on claimed self-hosted test hosts, never the dev box. Delivery commits carry Sprint, Story and Story-ID trailers.
