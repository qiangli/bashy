---
id: af5a35713e47
kind: test
title: B4 Verify complete fence coverage on a frozen candidate
seq: 3041
status: todo
priority: p2
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.394272Z
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Depends on B3 and consumed A/C fixes. Run the complete declared matrix, reconcile expected/executed counts and test prerequisite reproducibility from a clean environment. Record candidate/module/provider identities and per-platform receipts. Acceptance: every required case passes, no silent missing coverage or unexplained skips; optional C absence does not disable promised A/B behavior.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: see existing story details and S562 board; no verified run or branch reference recorded.
