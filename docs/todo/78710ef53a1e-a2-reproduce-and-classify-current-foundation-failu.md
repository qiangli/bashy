---
id: 78710ef53a1e
kind: test
title: A2 Reproduce and classify current foundation failures
seq: 3035
status: todo
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T02:24:39.368634Z
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
---

Depends on A1. Run the agreed current-baseline suites under their supported containment and finite bounds. Distinguish product defects, setup failures, applicability and manual review. Retain raw verdicts, denominators and hashes. Consult 560/3059 and 3063 receipts only where relevant; do not import historical verdicts as current results. Licensed runs require the exact supported approval. Acceptance: all required inventory entries have an execution result or explicit blocker with cause and next action.

Execution plan: docs/plan-clean-baseline-campaign.md. KISS/MVP: reuse existing code and harnesses; smallest causal repair; keep findings grouped here unless independent ownership/acceptance requires a separate story. Old sprints 539/560 and migration 557 are reference material only, not a mandatory reconciliation queue or a waiver. Preserve old evidence and raw failures. Record exact source, dependency, build and artifact identities. Managers must verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
