---
id: 86c744779ff1
kind: test
title: C2 Smoke-run the invite/chat journeys and route every failure
seq: 3043
status: todo
priority: p0
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.799189Z
sprint: 563
sprint_id: 282b20a5-f15c-55eb-a314-eef9580e19d1
sprint_title: 'Product UAT: clean-install user journeys and defect closure'
---

Depends on C1. Run J1-J10 as far as they go on W, M and L from clean isolated installs with exact binary/image identities; start with P0 on elephant for the UAT cohort by 2026-11-07 (J4 publish survives, J6 invited member chat, J7 reboot, T3 many members behind one IP). Preserve reproduction, actual output and logs for every failure; unavailable services are blockers, not passes. Route each finding once: product/optional-provider/Klatch/outpost causes to C3, shell/coreutils to 561 A3, fence/language to 562 B3, satellite (proprietary cloudbox) gaps to support tickets. Current findings F1-F15 and tickets T1-T3 are tracked in the plan. Acceptance: every journey cell measured or blocked with evidence, every defect routed without duplicate implementation.

Execution plan: docs/plan-s563-master-execution.md (campaign: docs/plan-clean-baseline-campaign.md). KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
