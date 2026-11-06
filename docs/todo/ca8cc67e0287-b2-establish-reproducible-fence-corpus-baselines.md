---
id: ca8cc67e0287
kind: test
title: B2 Establish reproducible fence corpus baselines
seq: 3039
status: assigned
priority: p2
labels:
    - clean-baseline
created: 2026-11-04T02:24:40.087251Z
weave: 22
assignee: agy-gemini3.8-flash-w1
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Depends on B1. Provision pinned prerequisites through supported mechanisms; run existing corpus cases with controlled configuration, authenticated artifacts where needed, original assertions and bounded execution. Record missing prerequisites separately from product failures. Reference 539 F2/F3 and 560 environment evidence when useful, without assuming those failures persist. Acceptance: every required matrix cell has a valid measured result or explicit actionable blocker.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: weave run 22, S562 B2 scrub run s562-b2-scrub-base-1; see existing story details and S562 board.
