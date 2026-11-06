---
id: feae8aa96a9a
kind: requirement
title: B1 Inventory promised fences and define the MVP corpus matrix
seq: 3038
status: todo
priority: p2
labels:
    - clean-baseline
created: 2026-11-04T02:24:39.956118Z
assignee: muse-conductor
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Enumerate supported fence types, language/runtime/toolchain providers, declared OS support, existing corpus cases and required versus optional capabilities. Record exact prerequisites and baseline identities; use existing suites. Separate language contracts in B from optional OCI/network/agent providers in C. Acceptance: finite required matrix with reproducible commands and explicit coverage gaps, no new framework.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: weave run 21.
