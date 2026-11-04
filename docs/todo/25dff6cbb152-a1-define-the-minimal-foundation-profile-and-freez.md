---
id: 25dff6cbb152
kind: requirement
title: A1 Define the minimal foundation profile and freeze its baseline
seq: 3034
status: todo
priority: p0
labels:
    - clean-baseline
created: 2026-11-04T02:24:39.236765Z
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
---

Enumerate required shell/POSIX and promised GNU behavior, required core utilities, supported certification environment, authoritative suites and dependencies. Record current source/module/toolchain/provider/harness identities. Separate required acceptance from advisory probes. Adopt B -> A and C -> A/B with product-entrypoint composition; record actual violations, without broad refactoring. Acceptance: finite inventory and reproducible immutable baseline manifest; no legacy ticket reconciliation prerequisite.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
