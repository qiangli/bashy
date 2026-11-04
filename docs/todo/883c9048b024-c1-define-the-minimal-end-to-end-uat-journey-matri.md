---
id: 883c9048b024
kind: requirement
title: C1 Define the minimal end-to-end UAT journey matrix
seq: 3042
status: todo
priority: p0
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.665614Z
sprint: 563
sprint_id: 282b20a5-f15c-55eb-a314-eef9580e19d1
sprint_title: 'Product UAT: clean-install user journeys and defect closure'
---

Define concrete inputs and expected outcomes for clean install, Bashy-only dependency/toolchain provisioning, shell/coreutils use, representative fences, app/island execution, explicit sandbox, guard/child execution and promised Yoke/shaping flows. Enumerate supported platforms and required external prerequisites. Include named private integrations only if explicitly selected in this finite matrix. Acceptance: bounded user-facing checklist using existing harnesses, no unbounded all-features campaign.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
