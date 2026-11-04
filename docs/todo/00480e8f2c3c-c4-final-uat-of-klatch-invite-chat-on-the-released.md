---
id: 00480e8f2c3c
kind: test
title: C4 Final UAT of Klatch invite/chat on the released product
seq: 3045
status: todo
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T02:24:41.105323Z
sprint: 563
sprint_id: 282b20a5-f15c-55eb-a314-eef9580e19d1
sprint_title: 'Product UAT: clean-install user journeys and defect closure'
---

Depends on C3 and stable 561/562 candidates. Freeze integrated Bashy, outpost and Klatch release identities and execute every required J1-J10 cell from clean install on W, M and L, natively, with single sign-on and team mode from the Klatch release (no host-scoped shaping); rerun the UAT cohort's journeys on that release and retire the elephant shaping branch and container. Recheck affected journeys after any new fix; keep coverage and failure history visible. Acceptance: complete agreed UAT passes, all in-scope issues closed by evidence, minimal user-facing limitations documented. Candidate acceptance only, not authorization to deploy, publish or replace a shared installation. Deferred: general software shaping (Sprint 564), satellite-less default outpost and multi-satellite switching (Sprint 565).

Execution plan: docs/plan-s563-master-execution.md (campaign: docs/plan-clean-baseline-campaign.md). KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
