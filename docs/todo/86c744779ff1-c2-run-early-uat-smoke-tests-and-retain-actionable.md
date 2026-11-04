---
id: 86c744779ff1
kind: test
title: C2 Run early UAT smoke tests and retain actionable failures
seq: 3043
status: todo
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.799189Z
sprint: 563
sprint_id: 282b20a5-f15c-55eb-a314-eef9580e19d1
sprint_title: 'Product UAT: clean-install user journeys and defect closure'
---

Depends on C1; start early alongside A/B baselines. Exercise the shortest critical journeys in clean isolated homes with exact binary/image identities. Preserve reproduction, actual output and logs for every encountered in-scope bug; unavailable services are blockers, not passes. Use historical 539 F8/F6/F1 and 560 private evidence as references. Acceptance: early smoke matrix measured and each defect routed to A, B or C ownership without duplicate implementation.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
