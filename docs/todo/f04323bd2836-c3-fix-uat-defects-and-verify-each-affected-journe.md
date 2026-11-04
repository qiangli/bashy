---
id: f04323bd2836
kind: bug
title: C3 Fix UAT defects and verify each affected journey
seq: 3044
status: todo
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.953245Z
sprint: 563
sprint_id: 282b20a5-f15c-55eb-a314-eef9580e19d1
sprint_title: 'Product UAT: clean-install user journeys and defect closure'
---

Depends on C2 findings; continue through final UAT. Implement minimal C/product fixes; cross-link A/B causes to their repair stories and consume verified fixes. Cover explicit sandbox launch identity and fail-closed behavior, Bashy child guard inheritance, declared external effects and provisioning if reproduced. No implicit sandbox or permissive unknown-tool classification. Test Linux/native Windows where behavior is promised; WSL does not replace native Windows. Acceptance: every encountered in-scope defect fixed and its affected positive/negative journey rerun. Newly discovered final-UAT failures reopen this work.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
