---
id: 00480e8f2c3c
kind: test
title: C4 Run complete UAT against the integrated candidate
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

Depends on C3 and stable A/B acceptance candidates. Freeze integrated product/module/image identities and execute every required journey/platform cell from clean install. Recheck affected journeys after any new fix; keep coverage and failure history visible. Acceptance: complete agreed UAT passes, all in-scope issues closed by evidence and minimal user-facing limitations documented. This is candidate acceptance, not authorization to deploy, publish or replace a shared installation.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
