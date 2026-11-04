---
id: ca8cc67e0287
kind: test
title: B2 Establish reproducible fence corpus baselines
seq: 3039
status: todo
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.087251Z
sprint: 562
sprint_id: 2e8945a5-d199-539c-aaea-321a738ab9df
sprint_title: 'Fence corpora: complete required coverage and repair current failures'
---

Depends on B1. Provision pinned prerequisites through supported mechanisms; run existing corpus cases with controlled configuration, authenticated artifacts where needed, original assertions and bounded execution. Record missing prerequisites separately from product failures. Reference 539 F2/F3 and 560 environment evidence when useful, without assuming those failures persist. Acceptance: every required matrix cell has a valid measured result or explicit actionable blocker.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
